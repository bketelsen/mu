package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/topics"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-admin-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := topics.Load(); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func TestTopicsHandlerRequiresOwner(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/topics", nil)
	rr := httptest.NewRecorder()
	TopicsHandler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestTopicsHandlerRendersEscapedReadOnlyTopics(t *testing.T) {
	topic := topics.Topic{
		Name:    "Escaped",
		FeedURL: "https://example.com/feed.xml",
		Prompt:  `bad"><script>alert(1)</script>`,
	}
	if _, err := topics.Create(topic); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = topics.Delete(topic.Name) })

	req := ownerTopicsRequest(t, http.MethodGet, nil, false)
	rr := httptest.NewRecorder()
	TopicsHandler(rr, req)
	body := rr.Body.String()
	token := auth.CSRFToken(req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d", rr.Code)
	}
	if strings.Contains(body, `bad"><script>alert(1)</script>`) || !strings.Contains(body, `&lt;script&gt;alert(1)&lt;/script&gt;`) {
		t.Fatalf("prompt was not escaped: %s", body)
	}
	if !strings.Contains(body, `<strong>Escaped</strong>`) {
		t.Fatalf("topic name is not read-only: %s", body)
	}
	if strings.Contains(body, `type="text" name="name" value="Escaped"`) {
		t.Fatalf("topic edit form offers a rename field: %s", body)
	}
	for _, want := range []string{
		`name="feed_url"`, `name="prompt"`, `name="_csrf" value="` + token + `"`,
		`Deletion hides active content but retains history.`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if got := strings.Count(body, `name="_csrf" value="`+token+`"`); got < 3 {
		t.Fatalf("CSRF token appears in %d forms, want at least 3", got)
	}
}

func TestTopicsHandlerMutations(t *testing.T) {
	create := url.Values{"action": {"create"}, "name": {"Created"}, "feed_url": {"https://example.com/created.xml"}, "prompt": {"Created prompt"}}
	assertTopicsRedirect(t, create)
	t.Cleanup(func() { _, _ = topics.Delete("Created") })

	if !hasTopic("Created", "https://example.com/created.xml", "Created prompt") {
		t.Fatalf("create did not persist: %#v", topics.Snapshot())
	}

	seed := topics.Topic{Name: "Updated", FeedURL: "https://example.com/old.xml", Prompt: "Old prompt"}
	if _, err := topics.Create(seed); err != nil {
		t.Fatal(err)
	}
	update := url.Values{"action": {"update"}, "name": {"Updated"}, "feed_url": {"https://example.com/new.xml"}, "prompt": {"New prompt"}}
	assertTopicsRedirect(t, update)
	if !hasTopic("Updated", "https://example.com/new.xml", "New prompt") {
		t.Fatalf("update did not preserve the submitted name: %#v", topics.Snapshot())
	}

	delete := url.Values{"action": {"delete"}, "name": {"Updated"}}
	assertTopicsRedirect(t, delete)
	if hasTopic("Updated", "", "") {
		t.Fatalf("delete did not remove topic: %#v", topics.Snapshot())
	}
}

func TestTopicsHandlerRejectsInvalidMutations(t *testing.T) {
	before := topics.Snapshot()
	for _, form := range []url.Values{
		{"action": {"create"}, "name": {"Bad"}, "feed_url": {"not a URL"}, "prompt": {"Prompt"}},
		{"action": {"unknown"}},
	} {
		req := ownerTopicsRequest(t, http.MethodPost, form, true)
		rr := httptest.NewRecorder()
		TopicsHandler(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("form %v: got %d, want 400", form, rr.Code)
		}
		if form.Get("action") == "create" && !strings.Contains(rr.Body.String(), "invalid feed URL") {
			t.Errorf("invalid create did not render its error: %s", rr.Body.String())
		}
		if !reflect.DeepEqual(topics.Snapshot(), before) {
			t.Fatalf("form %v mutated topics: %#v", form, topics.Snapshot())
		}
	}

	for _, token := range []string{"", "invalid"} {
		form := url.Values{"action": {"create"}, "name": {"Rejected"}, "feed_url": {"https://example.com/rejected.xml"}, "prompt": {"Prompt"}}
		if token != "" {
			form.Set("_csrf", token)
		}
		req := ownerTopicsRequest(t, http.MethodPost, form, false)
		rr := httptest.NewRecorder()
		TopicsHandler(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("token %q: got %d, want 403", token, rr.Code)
		}
	}

	req := ownerTopicsRequest(t, http.MethodPut, nil, false)
	rr := httptest.NewRecorder()
	TopicsHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", rr.Code)
	}
}

func assertTopicsRedirect(t *testing.T, form url.Values) {
	t.Helper()
	req := ownerTopicsRequest(t, http.MethodPost, form, true)
	rr := httptest.NewRecorder()
	TopicsHandler(rr, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/admin/topics?saved=1" {
		t.Fatalf("got status %d location %q", rr.Code, rr.Header().Get("Location"))
	}
}

func ownerTopicsRequest(t *testing.T, method string, form url.Values, withCSRF bool) *http.Request {
	t.Helper()
	cookie := ownerSessionCookie(t)
	req := httptest.NewRequest(method, "/admin/topics", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	if withCSRF {
		form.Set("_csrf", auth.CSRFToken(req))
		req = httptest.NewRequest(method, "/admin/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
	}
	return req
}

func hasTopic(name, feedURL, prompt string) bool {
	for _, topic := range topics.Snapshot() {
		if topic.Name == name && (feedURL == "" || (topic.FeedURL == feedURL && topic.Prompt == prompt)) {
			return true
		}
	}
	return false
}
