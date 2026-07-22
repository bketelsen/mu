package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

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
