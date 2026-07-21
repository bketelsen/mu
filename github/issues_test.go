package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssuesSearchesOpenRepositoryIssues(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		for _, want := range []string{"repo:micro/mu", "is:issue", "is:open", "memory leak"} {
			if !strings.Contains(r.URL.Query().Get("q"), want) {
				t.Fatalf("query = %q, missing %q", r.URL.Query().Get("q"), want)
			}
		}
		_, _ = io.WriteString(w, `{"items":[{"number":42,"title":"Fix leak"}]}`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	issues, _, err := c.Issues(context.Background(), ItemOptions{Owner: "micro", Repo: "mu", Query: "memory leak", State: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 42 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestPullRequestsListsRepositoryPullRequestsWithoutText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/micro/mu/pulls" || r.URL.RawQuery != "direction=desc&page=1&per_page=30&sort=updated&state=all" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `[{"number":7,"title":"Improve docs"}]`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	pulls, _, err := c.PullRequests(context.Background(), ItemOptions{Owner: "micro", Repo: "mu", State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].Number != 7 {
		t.Fatalf("pulls = %#v", pulls)
	}
}

func TestPullRequestsSearchesRepositoryTextWithoutAllQualifier(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query().Get("q")
		for _, want := range []string{"repo:micro/mu", "is:pr", "documentation"} {
			if !strings.Contains(query, want) {
				t.Fatalf("query = %q, missing %q", query, want)
			}
		}
		if strings.Contains(query, "is:all") {
			t.Fatalf("query = %q, contains is:all", query)
		}
		_, _ = io.WriteString(w, `{"items":[{"number":7,"title":"Improve docs"}]}`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	pulls, _, err := c.PullRequests(context.Background(), ItemOptions{Owner: "micro", Repo: "mu", Query: "documentation", State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].Number != 7 {
		t.Fatalf("pulls = %#v", pulls)
	}
}

func TestSearchValidatesAndQualifiesType(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/search/issues" || !strings.Contains(r.URL.Query().Get("q"), "is:pr") {
			t.Fatalf("request = %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"items":[{"number":3}]}`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })

	got, _, err := c.Search(context.Background(), ItemOptions{Query: "docs", Type: "pulls"})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %#v, err %v", got, err)
	}
	for _, opts := range []ItemOptions{{}, {Query: "x", Type: "invalid"}, {Owner: "micro", Repo: ""}} {
		_, _, err := c.Search(context.Background(), opts)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Kind != ErrorInvalid {
			t.Fatalf("options %#v error = %v, want invalid APIError", opts, err)
		}
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d", calls)
	}
}

func TestSearchOmitsAllStateQualifier(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if strings.Contains(query, "is:all") {
			t.Fatalf("query = %q, contains is:all", query)
		}
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	if _, _, err := c.Search(context.Background(), ItemOptions{Query: "docs", State: "all"}); err != nil {
		t.Fatal(err)
	}
}

func TestThreadFetchesSortedCommentsAndPullRequest(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		switch r.URL.Path {
		case "/repos/micro/mu/issues/42":
			_, _ = io.WriteString(w, `{"number":42,"pull_request":{"url":"x"}}`)
		case "/repos/micro/mu/issues/42/comments":
			if r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("request = %s", r.URL.String())
			}
			_, _ = io.WriteString(w, `[{"id":2,"created_at":"2025-02-01T00:00:00Z"},{"id":1,"created_at":"2025-01-01T00:00:00Z"}]`)
		case "/repos/micro/mu/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"title":"PR"}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	thread, err := c.Thread(context.Background(), "micro", "mu", 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(thread.Comments) != 2 || thread.Comments[0].ID != 1 || thread.PullRequest == nil || thread.PullRequest.Number != 42 {
		t.Fatalf("thread = %#v", thread)
	}
	if len(paths) != 3 {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestThreadDoesNotFetchPullRequestForIssue(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/repos/micro/mu/issues/42":
			_, _ = io.WriteString(w, `{"number":42}`)
		case "/repos/micro/mu/issues/42/comments":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })

	thread, err := c.Thread(context.Background(), "micro", "mu", 42)
	if err != nil || thread.PullRequest != nil || calls != 2 {
		t.Fatalf("thread %#v, calls %d, err %v", thread, calls, err)
	}
}

func TestItemOperationsRejectInvalidInputBeforeHTTP(t *testing.T) {
	calls := 0
	c := NewClient(&http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected HTTP request")
	})}, "https://api.github.com", func() string { return "test-token" })

	for _, opts := range []ItemOptions{{State: "pending"}, {Type: "invalid"}, {Owner: "bad/name", Repo: "mu"}, {Query: strings.Repeat("x", 257)}} {
		_, _, err := c.Issues(context.Background(), opts)
		assertInvalid(t, err)
	}
	_, err := c.Thread(context.Background(), "micro", "mu", 0)
	assertInvalid(t, err)
	_, _, err = c.PullRequests(context.Background(), ItemOptions{})
	assertInvalid(t, err)
	if calls != 0 {
		t.Fatalf("upstream calls = %d", calls)
	}
}

func assertInvalid(t *testing.T, err error) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorInvalid {
		t.Fatalf("error = %v, want invalid APIError", err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
