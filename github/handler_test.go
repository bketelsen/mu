package github

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func denyAdmin(*http.Request) (*auth.Session, *auth.Account, error) {
	return nil, nil, errors.New("admin access required")
}

func allowAdmin(*http.Request) (*auth.Session, *auth.Account, error) {
	return &auth.Session{Account: "admin"}, &auth.Account{ID: "admin", Admin: true}, nil
}

func TestHandlerRequiresAdminBeforeGitHubCall(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer ts.Close()
	h := newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), denyAdmin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d", calls)
	}
}

func TestHandlerShowsMissingTokenSetup(t *testing.T) {
	h := newHandler(NewServer(NewClient(http.DefaultClient, "https://api.github.invalid", func() string { return "" })), allowAdmin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github", nil))
	if body := w.Body.String(); !strings.Contains(body, "GITHUB_TOKEN") || !strings.Contains(body, `/admin/env`) {
		t.Fatalf("missing setup state: %s", body)
	}
}

func TestHandlerRendersRepositoryWorkspace(t *testing.T) {
	ts := workspaceServer(t, false)
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu&tab=issues&state=open&number=42", nil))
	body := w.Body.String()
	for _, want := range []string{"github-layout", "github-repos", "github-content", "micro/mu", "Issues", "Pull requests", "Fix leak", "Previous", "Next", `target="_blank" rel="noopener noreferrer"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("workspace missing %q: %s", want, body)
		}
	}
}

func TestHandlerEscapesGitHubContent(t *testing.T) {
	ts := workspaceServer(t, true)
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu&number=42", nil))
	body := w.Body.String()
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("unescaped GitHub content: %s", body)
	}
	if strings.Contains(body, `href="javascript:alert(1)"`) {
		t.Fatalf("unsafe GitHub URL rendered as link: %s", body)
	}
}

func TestHandlerParsesBoundedQueryState(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/user/repos" {
			t.Fatalf("unexpected upstream request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?tab=bad&state=nope&page=-1&number=-5", nil))
	body := w.Body.String()
	if strings.Contains(body, "GitHub workspace unavailable") {
		t.Fatalf("repository response did not decode successfully: %s", body)
	}
	for _, want := range []string{`name="tab" value="issues"`, `name="state" value="open"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("normalized state missing %q: %s", want, body)
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want repository list only", requests)
	}
}

func TestHandlerScopesQueryToSelectedRepository(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/repos":
			if got := r.URL.Query().Get("q"); got != "" {
				t.Fatalf("repository query = %q, want empty", got)
			}
			_, _ = io.WriteString(w, `[{"full_name":"micro/mu"}]`)
		case "/repos/micro/mu":
			_, _ = io.WriteString(w, `{"full_name":"micro/mu"}`)
		case "/search/issues":
			if got := r.URL.Query().Get("q"); !strings.Contains(got, "memory leak") {
				t.Fatalf("item query = %q, want search text", got)
			}
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.String())
		}
	}))
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu&q=memory+leak", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHandlerSearchFormPreservesRepositoryStateAndResetsDetail(t *testing.T) {
	ts := workspaceServer(t, false)
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu&tab=pulls&state=closed&page=3&number=42", nil))
	body := w.Body.String()
	for _, want := range []string{`placeholder="Search pull requests"`, `name="owner" value="micro"`, `name="repo" value="mu"`, `name="tab" value="pulls"`, `name="state" value="closed"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("search form missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `name="page"`) || strings.Contains(body, `name="number"`) {
		t.Fatalf("search form retained page or detail state: %s", body)
	}
}

func TestHandlerRetainsRepositoryRailOnRepositoryError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/repos":
			_, _ = io.WriteString(w, `[{"full_name":"micro/mu"}]`)
		case "/repos/micro/mu":
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.String())
		}
	}))
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu", nil))
	body := w.Body.String()
	for _, want := range []string{`class="github-repos"`, "micro/mu", `class="github-content"`, "GitHub workspace unavailable"} {
		if !strings.Contains(body, want) {
			t.Fatalf("error workspace missing %q: %s", want, body)
		}
	}
}

func TestHandlerPaginatesRepositoryRailIndependently(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/repos":
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Fatalf("repository page = %q, want 2", got)
			}
			w.Header().Set("Link", `<https://api.github.com/user/repos?page=1>; rel="prev", <https://api.github.com/user/repos?page=3>; rel="next"`)
			_, _ = io.WriteString(w, `[{"full_name":"micro/mu"}]`)
		case "/repos/micro/mu":
			_, _ = io.WriteString(w, `{"full_name":"micro/mu"}`)
		case "/search/issues":
			if got := r.URL.Query().Get("page"); got != "5" {
				t.Fatalf("item page = %q, want 5", got)
			}
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.String())
		}
	}))
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?owner=micro&repo=mu&q=a%26b&page=5&repo_page=2", nil))
	body := w.Body.String()
	for _, want := range []string{"repo_page=1", "repo_page=3", "page=5", "q=a%26b"} {
		if !strings.Contains(body, want) {
			t.Fatalf("repository pagination missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "q=a&b") {
		t.Fatalf("raw query rendered in pagination link: %s", body)
	}
}

func workspaceServer(t *testing.T, malicious bool) *httptest.Server {
	t.Helper()
	text, url := "Fix leak", "https://github.com/micro/mu"
	if malicious {
		text, url = "<script>alert(1)</script>", "javascript:alert(1)"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<https://api.github.com?page=1>; rel="prev", <https://api.github.com?page=3>; rel="next"`)
		switch r.URL.Path {
		case "/user/repos":
			_, _ = io.WriteString(w, `[{"id":1,"full_name":"micro/mu","description":`+jsonString(text)+`,"html_url":`+jsonString(url)+`}]`)
		case "/repos/micro/mu":
			_, _ = io.WriteString(w, `{"id":1,"full_name":"micro/mu","description":`+jsonString(text)+`,"html_url":`+jsonString(url)+`}`)
		case "/search/issues":
			_, _ = io.WriteString(w, `{"items":[{"number":42,"title":`+jsonString(text)+`,"body":`+jsonString(text)+`,"state":"open","html_url":`+jsonString(url)+`,"labels":[{"name":`+jsonString(text)+`}]}]}`)
		case "/repos/micro/mu/pulls":
			_, _ = io.WriteString(w, `[]`)
		case "/repos/micro/mu/issues/42":
			_, _ = io.WriteString(w, `{"number":42,"title":`+jsonString(text)+`,"body":`+jsonString(text)+`,"state":"open","html_url":`+jsonString(url)+`}`)
		case "/repos/micro/mu/issues/42/comments":
			_, _ = io.WriteString(w, `[{"body":`+jsonString(text)+`}]`)
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.String())
		}
	}))
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
