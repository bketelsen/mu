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
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer ts.Close()
	w := httptest.NewRecorder()
	newHandler(NewServer(NewClient(ts.Client(), ts.URL, testToken)), allowAdmin).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github?tab=bad&state=nope&page=-1&number=-5", nil))
	body := w.Body.String()
	for _, want := range []string{`name="tab" value="issues"`, `name="state" value="open"`, `name="page" value="1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("normalized state missing %q: %s", want, body)
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want repository list only", requests)
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
		case "/search/repositories":
			_, _ = io.WriteString(w, `{"items":[{"id":1,"full_name":"micro/mu","description":`+jsonString(text)+`,"html_url":`+jsonString(url)+`}]}`)
		case "/repos/micro/mu":
			_, _ = io.WriteString(w, `{"id":1,"full_name":"micro/mu","description":`+jsonString(text)+`,"html_url":`+jsonString(url)+`}`)
		case "/search/issues":
			_, _ = io.WriteString(w, `{"items":[{"number":42,"title":`+jsonString(text)+`,"body":`+jsonString(text)+`,"state":"open","html_url":`+jsonString(url)+`,"labels":[{"name":`+jsonString(text)+`}]}]}`)
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
