package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"mu/internal/service"
)

func TestServerRepositoriesReturnsStructuredAndModelReadyResults(t *testing.T) {
	s := newServiceTestServer(t)
	defer s.Close()

	var rsp RepositoriesResponse
	err := NewServer(NewClient(s.Client(), s.URL, testToken)).Repositories(context.Background(), &RepositoriesRequest{Query: "mu"}, &rsp)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsp.Repositories) != 1 || rsp.Repositories[0].FullName != "micro/mu" || rsp.Page.Page != 1 {
		t.Fatalf("response = %#v", rsp)
	}
	for _, want := range []string{"micro/mu", "Repository", "https://github.com/micro/mu"} {
		if !strings.Contains(rsp.Text, want) {
			t.Fatalf("text = %q, missing %q", rsp.Text, want)
		}
	}
}

func TestServerRepositorySelectsRequestedResource(t *testing.T) {
	s := newServiceTestServer(t)
	defer s.Close()
	server := NewServer(NewClient(s.Client(), s.URL, testToken))

	for _, tc := range []struct {
		resource string
		issues   int
		pulls    int
	}{
		{"", 0, 0},
		{"metadata", 0, 0},
		{"issues", 1, 0},
		{"pulls", 0, 1},
	} {
		var rsp RepositoryResponse
		err := server.Repository(context.Background(), &RepositoryRequest{Owner: "micro", Repo: "mu", Resource: tc.resource}, &rsp)
		if err != nil {
			t.Fatalf("resource %q: %v", tc.resource, err)
		}
		if rsp.Repository.FullName != "micro/mu" || len(rsp.Issues) != tc.issues || len(rsp.PullRequests) != tc.pulls {
			t.Fatalf("resource %q response = %#v", tc.resource, rsp)
		}
	}

	for _, resource := range []string{"unknown"} {
		err := server.Repository(context.Background(), &RepositoryRequest{Owner: "micro", Repo: "mu", Resource: resource}, &RepositoryResponse{})
		assertServerInvalid(t, err)
	}
}

func TestServerSearchMapsResourceAndRequiresQuery(t *testing.T) {
	s := newServiceTestServer(t)
	defer s.Close()
	server := NewServer(NewClient(s.Client(), s.URL, testToken))

	for _, resource := range []string{"issues", "pulls", "all"} {
		var rsp SearchResponse
		err := server.Search(context.Background(), &SearchRequest{Query: "leak", Owner: "micro", Repo: "mu", Resource: resource, State: "open"}, &rsp)
		if err != nil || len(rsp.Items) != 1 {
			t.Fatalf("resource %q: response %#v, error %v", resource, rsp, err)
		}
		for _, want := range []string{"Issue #42", "open", "bug", "Fix leak", "https://github.com/micro/mu/issues/42"} {
			if !strings.Contains(rsp.Text, want) {
				t.Fatalf("resource %q text = %q, missing %q", resource, rsp.Text, want)
			}
		}
	}

	assertServerInvalid(t, server.Search(context.Background(), &SearchRequest{}, &SearchResponse{}))
	assertServerInvalid(t, server.Search(context.Background(), &SearchRequest{Query: "leak", Resource: "bad"}, &SearchResponse{}))
}

func TestServerIssueFormatsThreadAndSafelyTruncatesText(t *testing.T) {
	s := newServiceTestServer(t)
	defer s.Close()

	var rsp IssueResponse
	err := NewServer(NewClient(s.Client(), s.URL, testToken)).Issue(context.Background(), &IssueRequest{Owner: "micro", Repo: "mu", Number: 42}, &rsp)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.Thread.Issue.Number != 42 || len(rsp.Thread.Comments) != 2 {
		t.Fatalf("response = %#v", rsp)
	}
	for _, want := range []string{"micro/mu", "Issue #42", "open", "bug", "Issue body", "first comment", "second comment", "https://github.com/micro/mu/issues/42"} {
		if !strings.Contains(rsp.Text, want) {
			t.Fatalf("text = %q, missing %q", rsp.Text, want)
		}
	}

	long := Thread{Issue: Issue{Number: 1, Title: "large", Body: strings.Repeat("界", 12000)}}
	text := threadText(long)
	if len(text) > 32<<10 || !utf8.ValidString(text) {
		t.Fatalf("length = %d, valid UTF-8 = %t", len(text), utf8.ValidString(text))
	}
}

func TestServerRejectsNilRequests(t *testing.T) {
	server := NewServer(nil)
	assertServerInvalid(t, server.Repositories(context.Background(), nil, &RepositoriesResponse{}))
	assertServerInvalid(t, server.Repository(context.Background(), nil, &RepositoryResponse{}))
	assertServerInvalid(t, server.Search(context.Background(), nil, &SearchResponse{}))
	assertServerInvalid(t, server.Issue(context.Background(), nil, &IssueResponse{}))
}

func TestServerMeshRepositories(t *testing.T) {
	s := newServiceTestServer(t)
	defer s.Close()

	const name = "github-task4-service-test"
	if err := service.Register(name, NewServer(NewClient(s.Client(), s.URL, testToken))); err != nil {
		t.Fatal(err)
	}
	var rsp RepositoriesResponse
	if err := service.Call(context.Background(), name, "Server.Repositories", &RepositoriesRequest{Query: "mu"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if len(rsp.Repositories) != 1 || rsp.Repositories[0].FullName != "micro/mu" {
		t.Fatalf("response = %#v", rsp)
	}
}

func newServiceTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/repositories":
			_, _ = io.WriteString(w, `{"items":[{"id":1,"full_name":"micro/mu","description":"Mu home server","html_url":"https://github.com/micro/mu"}]}`)
		case "/repos/micro/mu":
			_, _ = io.WriteString(w, `{"id":1,"full_name":"micro/mu","description":"Mu home server","html_url":"https://github.com/micro/mu"}`)
		case "/repos/micro/mu/pulls":
			_, _ = io.WriteString(w, `[{"number":7,"title":"Improve docs","body":"Pull body","state":"open","html_url":"https://github.com/micro/mu/pull/7","labels":[{"name":"docs"}]}]`)
		case "/search/issues":
			_, _ = io.WriteString(w, `{"items":[{"number":42,"title":"Fix leak","body":"Issue body","state":"open","html_url":"https://github.com/micro/mu/issues/42","labels":[{"name":"bug"}]}]}`)
		case "/repos/micro/mu/issues/42":
			_, _ = io.WriteString(w, `{"number":42,"title":"Fix leak","body":"Issue body","state":"open","html_url":"https://github.com/micro/mu/issues/42","labels":[{"name":"bug"}]}`)
		case "/repos/micro/mu/issues/42/comments":
			_, _ = io.WriteString(w, `[{"body":"second comment","created_at":"2025-02-01T00:00:00Z"},{"body":"first comment","created_at":"2025-01-01T00:00:00Z"}]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
}

func testToken() string { return "test-token" }

func assertServerInvalid(t *testing.T, err error) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorInvalid {
		t.Fatalf("error = %v, want invalid APIError", err)
	}
}
