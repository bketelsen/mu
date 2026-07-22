package chat

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"

	"mu/internal/data"
	"mu/internal/testutil"
	topicstore "mu/topics"
)

func TestMain(m *testing.M) {
	testutil.RunWithTempHome(m)
}

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
	originalConfigured, original := summaryAIConfigured, generateTopicSummary
	summaryAIConfigured = func() bool { return true }
	generateTopicSummary = func(name, prompt string) (string, error) {
		if name == "World" || name == "New" {
			return "", errors.New("LLM unavailable")
		}
		return "new " + name, nil
	}
	defer func() {
		summaryAIConfigured = originalConfigured
		generateTopicSummary = original
	}()

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

func TestSummaryBatchSkipsUnconfiguredAIOnce(t *testing.T) {
	applyTopicSnapshot([]topicstore.Topic{{Name: "Dev", Prompt: "dev"}, {Name: "World", Prompt: "world"}})
	originalConfigured, originalGenerate := summaryAIConfigured, generateTopicSummary
	summaryAIConfigured = func() bool { return false }
	called := 0
	generateTopicSummary = func(string, string) (string, error) {
		called++
		return "", nil
	}
	t.Cleanup(func() {
		summaryAIConfigured = originalConfigured
		generateTopicSummary = originalGenerate
	})

	generateSummaryBatch([]string{"Dev", "World"})
	if called != 0 {
		t.Fatalf("generated %d summaries while AI was unconfigured", called)
	}
}

func TestSummaryBatchDoesNotRestoreTopicDeletedDuringGeneration(t *testing.T) {
	applyTopicSnapshot([]topicstore.Topic{{Name: "World", Prompt: "world"}})
	mutex.Lock()
	summaries = map[string]string{}
	mutex.Unlock()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	originalConfigured, original := summaryAIConfigured, generateTopicSummary
	summaryAIConfigured = func() bool { return true }
	generateTopicSummary = func(name, prompt string) (string, error) {
		close(started)
		<-release
		return "stale World summary", nil
	}
	defer func() {
		summaryAIConfigured = originalConfigured
		generateTopicSummary = original
	}()

	go func() {
		generateSummaryBatch([]string{"World"})
		close(finished)
	}()
	<-started
	applyTopicChange(nil, topicstore.Change{Deleted: []topicstore.Topic{{Name: "World"}}})
	close(release)
	<-finished

	mutex.RLock()
	_, exists := summaries["World"]
	mutex.RUnlock()
	if exists {
		t.Fatal("deleted topic summary was restored in memory")
	}
	b, err := data.LoadFile("chat_summaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if _, exists := saved["World"]; exists {
		t.Fatal("deleted topic summary was restored on disk")
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

func TestOwnerChatRequestIsNotPaymentGated(t *testing.T) {
	rec := httptest.NewRecorder()
	req := ownerChatRequest(t, http.MethodPost, "/chat", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	handlePostChat(rec, req)
	assertNoChatPaymentGate(t, rec)
}

func ownerChatRequest(t *testing.T, method, target string, body *strings.Reader) *http.Request {
	t.Helper()
	owner, err := auth.Owner()
	if err != nil {
		owner = &auth.Account{ID: "chatowner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return req
}

func assertNoChatPaymentGate(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code == http.StatusPaymentRequired {
		t.Fatalf("request was payment-gated: %s", recorder.Body.String())
	}
	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"insufficient credits", "top up", "/wallet"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response contains removed payment copy %q: %s", forbidden, recorder.Body.String())
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
