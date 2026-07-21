package chat

import (
	"strings"
	"testing"
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

func TestGuestChatAuthNoticeExplainsLoginAndAgentFallback(t *testing.T) {
	html := guestChatAuthNotice()

	for _, want := range []string{
		"Sign in to use saved chat.",
		"/agent",
		"Try Mu without an account",
		"/login?redirect=/chat",
		"/signup?redirect=/chat",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat auth notice missing %q in %s", want, html)
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
