// Package topics manages the persisted configuration shared by news and chat.
package topics

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"mu/internal/data"
)

type Topic struct {
	Name    string `json:"name"`
	FeedURL string `json:"feed_url"`
	Prompt  string `json:"prompt"`
}

type Change struct {
	Added         []Topic
	Deleted       []Topic
	FeedChanged   []Topic
	PromptChanged []Topic
}

type Subscriber func(snapshot []Topic, change Change)

var defaultTopics = []Topic{
	{Name: "Crypto", FeedURL: "https://www.coindesk.com/arc/outboundfeeds/rss", Prompt: "Write exactly 2-3 concise sentences about the current state and key trends in cryptocurrency. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "Dev", FeedURL: "https://news.ycombinator.com/rss", Prompt: "Write exactly 2-3 concise sentences about current trends and hot topics in startups and development. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "Finance", FeedURL: "https://search.cnbc.com/rs/search/combinedcms/view.xml?partnerId=wrss01&id=100003114", Prompt: "Write exactly 2-3 concise sentences about the current state and key trends in global finance. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "Politics", FeedURL: "https://www.theguardian.com/politics/rss", Prompt: "Write exactly 2-3 concise sentences about current major political topics and trends. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "Tech", FeedURL: "https://techcrunch.com/feed/", Prompt: "Write exactly 2-3 concise sentences about current major trends and developments in technology. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "UK", FeedURL: "https://feeds.bbci.co.uk/news/rss.xml", Prompt: "Write exactly 2-3 concise sentences about current major topics and trends in UK news. Use plain text only, no bullets, asterisks, or special formatting."},
	{Name: "World", FeedURL: "https://www.aljazeera.com/xml/rss/all.xml", Prompt: "Write exactly 2-3 concise sentences about current major topics and trends around the world. Use plain text only, no bullets, asterisks, or special formatting."},
}

var (
	mu          sync.RWMutex
	records     []Topic
	subscribers = map[int]Subscriber{}
	nextID      int
	persist     = writeFile
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]*$`)

func Load() error {
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(data.Dir(), "topics.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		defaults := copyTopics(defaultTopics)
		normalizeTopics(defaults)
		sortTopics(defaults)
		if err := persist(defaults); err != nil {
			return err
		}
		records = defaults
		return nil
	}
	if err != nil {
		return err
	}
	var loaded []Topic
	if err := json.Unmarshal(b, &loaded); err != nil {
		return err
	}
	normalizeTopics(loaded)
	if err := validateTopics(loaded); err != nil {
		return err
	}
	sortTopics(loaded)
	records = loaded
	return nil
}

func Snapshot() []Topic {
	mu.RLock()
	snapshot := copyTopics(records)
	mu.RUnlock()
	sortTopics(snapshot)
	return snapshot
}

func Create(topic Topic) (Change, error) {
	topic = normalizeTopic(topic)
	mu.Lock()
	proposed := append(copyTopics(records), topic)
	if err := validateTopics(proposed); err != nil {
		mu.Unlock()
		return Change{}, err
	}
	sortTopics(proposed)
	if err := persist(proposed); err != nil {
		mu.Unlock()
		return Change{}, err
	}
	records = proposed
	change := Change{Added: []Topic{topic}}
	snapshot, callbacks := committedStateLocked()
	mu.Unlock()
	notify(callbacks, snapshot, change)
	return copyChange(change), nil
}

func Update(name string, replacement Topic) (Change, error) {
	replacement = normalizeTopic(replacement)
	mu.Lock()
	index := topicIndex(records, name)
	if index < 0 {
		mu.Unlock()
		return Change{}, fmt.Errorf("topic %q not found", name)
	}
	if replacement.Name != name {
		mu.Unlock()
		return Change{}, fmt.Errorf("topic name cannot be changed")
	}
	previous := records[index]
	proposed := copyTopics(records)
	proposed[index] = replacement
	if err := validateTopics(proposed); err != nil {
		mu.Unlock()
		return Change{}, err
	}
	sortTopics(proposed)
	if err := persist(proposed); err != nil {
		mu.Unlock()
		return Change{}, err
	}
	records = proposed
	change := Change{}
	if previous.FeedURL != replacement.FeedURL {
		change.FeedChanged = []Topic{replacement}
	}
	if previous.Prompt != replacement.Prompt {
		change.PromptChanged = []Topic{replacement}
	}
	snapshot, callbacks := committedStateLocked()
	mu.Unlock()
	notify(callbacks, snapshot, change)
	return copyChange(change), nil
}

func Delete(name string) (Change, error) {
	mu.Lock()
	index := topicIndex(records, name)
	if index < 0 {
		mu.Unlock()
		return Change{}, fmt.Errorf("topic %q not found", name)
	}
	deleted := records[index]
	proposed := make([]Topic, 0, len(records)-1)
	proposed = append(proposed, records[:index]...)
	proposed = append(proposed, records[index+1:]...)
	if err := persist(proposed); err != nil {
		mu.Unlock()
		return Change{}, err
	}
	records = proposed
	change := Change{Deleted: []Topic{deleted}}
	snapshot, callbacks := committedStateLocked()
	mu.Unlock()
	notify(callbacks, snapshot, change)
	return copyChange(change), nil
}

func Subscribe(subscriber Subscriber) func() {
	mu.Lock()
	id := nextID
	nextID++
	subscribers[id] = subscriber
	mu.Unlock()
	return func() {
		mu.Lock()
		delete(subscribers, id)
		mu.Unlock()
	}
}

func writeFile(records []Topic) error {
	if err := os.MkdirAll(data.Dir(), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(data.Dir(), ".topics-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(data.Dir(), "topics.json")); err != nil {
		return err
	}
	ok = true
	return nil
}

func validateTopics(topics []Topic) error {
	seen := make([]string, 0, len(topics))
	for _, topic := range topics {
		name := strings.TrimSpace(topic.Name)
		if name == "" || !safeName.MatchString(name) {
			return fmt.Errorf("invalid topic name %q", topic.Name)
		}
		if strings.TrimSpace(topic.Prompt) == "" {
			return fmt.Errorf("topic %q has an empty prompt", topic.Name)
		}
		u, err := url.ParseRequestURI(strings.TrimSpace(topic.FeedURL))
		if err != nil || !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("topic %q has an invalid feed URL", topic.Name)
		}
		for _, existing := range seen {
			if strings.EqualFold(existing, name) {
				return fmt.Errorf("duplicate topic name %q", topic.Name)
			}
		}
		seen = append(seen, name)
	}
	return nil
}

func normalizeTopic(topic Topic) Topic {
	topic.Name = strings.TrimSpace(topic.Name)
	topic.FeedURL = strings.TrimSpace(topic.FeedURL)
	topic.Prompt = strings.TrimSpace(topic.Prompt)
	return topic
}

func normalizeTopics(topics []Topic) {
	for i := range topics {
		topics[i] = normalizeTopic(topics[i])
	}
}

func copyTopics(topics []Topic) []Topic {
	return append([]Topic(nil), topics...)
}

func sortTopics(topics []Topic) {
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].Name < topics[j].Name
	})
}

func topicIndex(topics []Topic, name string) int {
	for i, topic := range topics {
		if topic.Name == name {
			return i
		}
	}
	return -1
}

func committedStateLocked() ([]Topic, []Subscriber) {
	snapshot := copyTopics(records)
	sortTopics(snapshot)
	callbacks := make([]Subscriber, 0, len(subscribers))
	for _, subscriber := range subscribers {
		callbacks = append(callbacks, subscriber)
	}
	return snapshot, callbacks
}

func notify(callbacks []Subscriber, snapshot []Topic, change Change) {
	for _, subscriber := range callbacks {
		subscriber(copyTopics(snapshot), copyChange(change))
	}
}

func copyChange(change Change) Change {
	change.Added = copyTopics(change.Added)
	change.Deleted = copyTopics(change.Deleted)
	change.FeedChanged = copyTopics(change.FeedChanged)
	change.PromptChanged = copyTopics(change.PromptChanged)
	sortTopics(change.Added)
	sortTopics(change.Deleted)
	sortTopics(change.FeedChanged)
	sortTopics(change.PromptChanged)
	return change
}
