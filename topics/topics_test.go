package topics

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"mu/internal/data"
)

func useTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	resetForTest()
	t.Cleanup(resetForTest)
}

func resetForTest() {
	mu.Lock()
	records = nil
	subscribers = map[int]Subscriber{}
	nextID = 0
	persist = writeFile
	mu.Unlock()
}

func TestLoadSeedsDefaultsOnce(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	got := Snapshot()
	if len(got) != 7 {
		t.Fatalf("got %d topics, want 7", len(got))
	}
	if got[0].Name != "Crypto" || got[6].Name != "World" {
		t.Fatalf("unexpected sorted defaults: %#v", got)
	}
	b, err := os.ReadFile(filepath.Join(data.Dir(), "topics.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"feed_url"`)) || !bytes.Contains(b, []byte(`"prompt"`)) {
		t.Fatalf("persisted defaults are incomplete: %s", b)
	}

	resetForTest()
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Snapshot(); len(got) != 7 || got[0].Name != "Crypto" || got[6].Name != "World" {
		t.Fatalf("second load returned %#v", got)
	}
}

func TestLoadPreservesExistingEmptyCollection(t *testing.T) {
	useTempHome(t)
	if err := os.MkdirAll(data.Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data.Dir(), "topics.json")
	if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Snapshot(); len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}
}

func TestLoadRejectsInvalidExistingFileWithoutReplacingIt(t *testing.T) {
	useTempHome(t)
	if err := os.MkdirAll(data.Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data.Dir(), "topics.json")
	want := []byte("not json")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(); err == nil {
		t.Fatal("Load succeeded, want error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file changed to %q", got)
	}
}

func TestCreateValidatesAndSortsTopics(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	valid := Topic{Name: "Science", FeedURL: "https://example.com/science.xml", Prompt: "Summarize science."}
	invalid := []Topic{
		{},
		{Name: "Unsafe/Name", FeedURL: valid.FeedURL, Prompt: valid.Prompt},
		{Name: valid.Name, FeedURL: "ftp://example.com/feed.xml", Prompt: valid.Prompt},
		{Name: valid.Name, FeedURL: "/feed.xml", Prompt: valid.Prompt},
		{Name: valid.Name, FeedURL: valid.FeedURL, Prompt: " \t "},
	}
	for _, topic := range invalid {
		if _, err := Create(topic); err == nil {
			t.Fatalf("Create(%#v) succeeded", topic)
		}
	}
	change, err := Create(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(change.Added, []Topic{valid}) {
		t.Fatalf("added %#v, want %#v", change.Added, []Topic{valid})
	}
	if _, err := Create(Topic{Name: "science", FeedURL: valid.FeedURL, Prompt: valid.Prompt}); err == nil {
		t.Fatal("case-folded duplicate succeeded")
	}
	if got := Snapshot(); got[4].Name != "Science" {
		t.Fatalf("topics are not sorted: %#v", got)
	}
}

func TestUpdateClassifiesChangesAndRejectsRename(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	tech := topicNamed(t, Snapshot(), "Tech")
	renamed := tech
	renamed.Name = "Technology"
	if _, err := Update("Tech", renamed); err == nil {
		t.Fatal("rename succeeded")
	}
	changedFeed := tech
	changedFeed.FeedURL = "https://example.com/tech.xml"
	change, err := Update("Tech", changedFeed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(change.FeedChanged, []Topic{changedFeed}) || len(change.PromptChanged) != 0 {
		t.Fatalf("unexpected feed change: %#v", change)
	}
	changedPrompt := changedFeed
	changedPrompt.Prompt = "New technology summary."
	change, err = Update("Tech", changedPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(change.PromptChanged, []Topic{changedPrompt}) || len(change.FeedChanged) != 0 {
		t.Fatalf("unexpected prompt change: %#v", change)
	}
	change, err = Delete("Tech")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(change.Deleted, []Topic{changedPrompt}) {
		t.Fatalf("unexpected delete: %#v", change)
	}
}

func TestSnapshotsAndSubscriberArgumentsAreCopied(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot()
	snapshot[0].Name = "Changed"
	if got := Snapshot()[0].Name; got != "Crypto" {
		t.Fatalf("snapshot mutation changed store to %q", got)
	}
	unsubscribe := Subscribe(func(snapshot []Topic, change Change) {
		snapshot[0].Name = "Changed"
		change.Added[0].Name = "Changed"
	})
	defer unsubscribe()
	if _, err := Create(Topic{Name: "Science", FeedURL: "https://example.com/science.xml", Prompt: "Summarize science."}); err != nil {
		t.Fatal(err)
	}
	if got := topicNamed(t, Snapshot(), "Science").Name; got != "Science" {
		t.Fatalf("subscriber mutation changed store to %q", got)
	}
}

func TestSubscriberRunsAfterCommitAndOutsideLock(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	called := false
	unsubscribe := Subscribe(func(snapshot []Topic, change Change) {
		called = true
		if got := Snapshot(); len(got) != len(snapshot) {
			t.Fatalf("snapshot length %d, want %d", len(got), len(snapshot))
		}
		b, err := os.ReadFile(filepath.Join(data.Dir(), "topics.json"))
		if err != nil {
			t.Fatal(err)
		}
		var persisted []Topic
		if err := json.Unmarshal(b, &persisted); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(persisted, snapshot) {
			t.Fatalf("disk contents %#v, want %#v", persisted, snapshot)
		}
		if len(change.Added) != 1 || change.Added[0].Name != "Science" {
			t.Fatalf("unexpected change: %#v", change)
		}
	})
	defer unsubscribe()
	if _, err := Create(Topic{Name: "Science", FeedURL: "https://example.com/science.xml", Prompt: "Summarize science."}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("subscriber not called")
	}
}

func TestUpdatePersistenceFailureDoesNotCommit(t *testing.T) {
	useTempHome(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	before := Snapshot()
	path := filepath.Join(data.Dir(), "topics.json")
	diskBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	originalPersist := persist
	persist = func([]Topic) error { return errors.New("disk full") }
	t.Cleanup(func() { persist = originalPersist })
	tech := topicNamed(t, before, "Tech")
	tech.Prompt = "changed"
	if _, err := Update("Tech", tech); err == nil {
		t.Fatal("update succeeded")
	}
	if !reflect.DeepEqual(Snapshot(), before) {
		t.Fatal("failed write changed memory")
	}
	diskAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(diskAfter, diskBefore) {
		t.Fatal("failed write changed disk")
	}
}

func topicNamed(t *testing.T, topics []Topic, name string) Topic {
	t.Helper()
	for _, topic := range topics {
		if topic.Name == name {
			return topic
		}
	}
	t.Fatalf("topic %q not found in %#v", name, topics)
	return Topic{}
}
