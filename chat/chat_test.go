package chat

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"mu/internal/data"
	topicstore "mu/topics"
)

func TestTopicConfigSnapshotCopiesAndReplacesTopics(t *testing.T) {
	applyTopicSnapshot([]topicstore.Topic{
		{Name: "World", Prompt: "world prompt"},
		{Name: "Dev", Prompt: "dev prompt"},
	})

	applyTopicSnapshot([]topicstore.Topic{{Name: "Crypto", Prompt: "crypto prompt"}, {Name: "Dev", Prompt: "new dev prompt"}})
	names, prompts := topicConfigSnapshot()
	if want := []string{"Crypto", "Dev"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
	if got := prompts["Dev"]; got != "new dev prompt" {
		t.Fatalf("Dev prompt = %q, want updated prompt", got)
	}
	if _, ok := prompts["World"]; ok {
		t.Fatal("replaced topic configuration retained World")
	}

	names[0] = "changed"
	prompts["Dev"] = "changed"
	copiedNames, copiedPrompts := topicConfigSnapshot()
	if want := []string{"Crypto", "Dev"}; !reflect.DeepEqual(copiedNames, want) {
		t.Fatalf("names copy changed runtime state: %#v", copiedNames)
	}
	if got := copiedPrompts["Dev"]; got != "new dev prompt" {
		t.Fatalf("prompts copy changed runtime state: %q", got)
	}
}

func TestSummaryTopicsIncludesAddedAndPromptChangedOnce(t *testing.T) {
	change := topicstore.Change{
		Added:         []topicstore.Topic{{Name: "Dev"}, {Name: "World"}},
		PromptChanged: []topicstore.Topic{{Name: "Dev"}},
		FeedChanged:   []topicstore.Topic{{Name: "Finance"}},
	}
	if got, want := summaryTopics(change), []string{"Dev", "World"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("summaryTopics() = %#v, want %#v", got, want)
	}
}

func TestTopicChangeDoesNotQueueFeedOnlyUpdates(t *testing.T) {
	if got := summaryTopics(topicstore.Change{FeedChanged: []topicstore.Topic{{Name: "Dev"}}}); len(got) != 0 {
		t.Fatalf("summaryTopics(feed change) = %#v, want none", got)
	}
}

func TestTopicDeletionPrunesSummaryWithoutRoomFiles(t *testing.T) {
	applyTopicSnapshot([]topicstore.Topic{{Name: "Dev", Prompt: "dev"}, {Name: "World", Prompt: "world"}})
	mutex.Lock()
	summaries = map[string]string{"Dev": "keep", "World": "remove"}
	summaryMeta = SummaryMetadata{Status: "fresh"}
	mutex.Unlock()
	if err := data.SaveFile("room_chat_World.json", "room messages"); err != nil {
		t.Fatal(err)
	}

	applyTopicChange([]topicstore.Topic{{Name: "Dev", Prompt: "dev"}}, topicstore.Change{Deleted: []topicstore.Topic{{Name: "World"}}})
	generateSummaryBatch(nil)

	mutex.RLock()
	_, exists := summaries["World"]
	mutex.RUnlock()
	if exists {
		t.Fatal("deleted topic summary remains in memory")
	}
	b, err := data.LoadFile("chat_summaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if got := saved["Dev"]; got != "keep" {
		t.Fatalf("saved Dev summary = %q, want keep", got)
	}
	if _, ok := saved["World"]; ok {
		t.Fatal("deleted topic summary remains on disk")
	}
	if b, err := data.LoadFile("room_chat_World.json"); err != nil || string(b) != "room messages" {
		t.Fatalf("room file changed: %q, %v", b, err)
	}
}

func TestSummaryBatchRetainsPriorSummaryAfterPartialFailure(t *testing.T) {
	applyTopicSnapshot([]topicstore.Topic{{Name: "Dev", Prompt: "dev"}, {Name: "World", Prompt: "world"}, {Name: "New", Prompt: "new"}})
	mutex.Lock()
	summaries = map[string]string{"Dev": "old dev", "World": "old world"}
	mutex.Unlock()
	original := generateTopicSummary
	generateTopicSummary = func(name, prompt string) (string, error) {
		if name == "World" || name == "New" {
			return "", errors.New("LLM unavailable")
		}
		return "new " + name, nil
	}
	defer func() { generateTopicSummary = original }()

	generateSummaryBatch([]string{"Dev", "World", "New"})

	mutex.RLock()
	defer mutex.RUnlock()
	if got := summaries["Dev"]; got != "new Dev" {
		t.Fatalf("successful summary = %q, want new Dev", got)
	}
	if got := summaries["World"]; got != "old world" {
		t.Fatalf("failed summary = %q, want retained old world", got)
	}
	if _, ok := summaries["New"]; ok {
		t.Fatal("failed new topic gained a summary")
	}
}

func TestHandlePatternMatchIgnoresUnsupportedPrompts(t *testing.T) {
	tests := []string{
		"",
		"tell me about bitcoin",
		"btc price",
		"price of gold",
		"price",
		"a price",
		"this symbol is too long price",
	}

	for _, content := range tests {
		t.Run(content, func(t *testing.T) {
			if got := handlePatternMatch(content, nil); got != "" {
				t.Fatalf("handlePatternMatch(%q) = %q, want empty string", content, got)
			}
		})
	}
}

func TestGuestChatAuthNoticeOffersOwnerLoginOnly(t *testing.T) {
	html := guestChatAuthNotice()

	for _, want := range []string{"Sign in to use your chat.", "/login?redirect=/chat"} {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat auth notice missing %q in %s", want, html)
		}
	}
	for _, forbidden := range []string{"/agent", "/setup", "without an account"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("guest chat auth notice contains %q in %s", forbidden, html)
		}
	}
}

func TestCurrentSummaryMetaDefaultsUnavailable(t *testing.T) {
	oldMeta := summaryMeta
	summaryMeta = SummaryMetadata{}
	defer func() { summaryMeta = oldMeta }()

	meta := currentSummaryMeta()
	if meta.Status != "unavailable" {
		t.Fatalf("Status = %q, want unavailable", meta.Status)
	}
	if meta.Source == "" {
		t.Fatalf("Source should explain summary provenance")
	}
}
