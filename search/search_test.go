package search

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"no tags here", "no tags here"},
		{"<strong>bold</strong> text", "bold text"},
		{"<em>italic</em> and <b>bold</b>", "italic and bold"},
		{"result with &amp; entity", "result with & entity"},
		{"<b>hello</b> &lt;world&gt;", "hello <world>"},
		{"", ""},
	}
	for _, tc := range tests {
		got := stripHTML(tc.input)
		if got != tc.want {
			t.Errorf("stripHTML(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestOwnerWebSearchIsNotPaymentGated(t *testing.T) {
	rec := httptest.NewRecorder()
	WebHandler(rec, ownerSearchRequest(t, "/web?q=mu"))
	assertNoSearchPaymentGate(t, rec)
}

func ownerSearchRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	owner, err := auth.Owner()
	if err != nil {
		owner = &auth.Account{ID: "searchowner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return req
}

func assertNoSearchPaymentGate(t *testing.T, recorder *httptest.ResponseRecorder) {
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

func TestRecentSearchesScriptEscapesLabelsWithoutManglingSpaces(t *testing.T) {
	if !strings.Contains(webRecentSearchesScript, "div.textContent = String(text);") {
		t.Fatal("recent search labels should escape via textContent")
	}
	if strings.Contains(webRecentSearchesScript, ".replace(/ /g, '&gt;')") || strings.Contains(webRecentSearchesScript, ".replace(/\\s/g, '&gt;')") {
		t.Fatal("recent search escaping must not convert spaces to HTML entities")
	}
	if !strings.Contains(webRecentSearchesScript, "encodeURIComponent(search)") {
		t.Fatal("recent search data attributes should encode raw queries without changing label text")
	}
	if !strings.Contains(webRecentSearchesScript, "decodeURIComponent(item.getAttribute('data-query') || '')") {
		t.Fatal("recent search click/remove handlers should decode stored queries")
	}
}
