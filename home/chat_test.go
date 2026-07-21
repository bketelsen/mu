package home

import (
	"strings"
	"testing"
)

func TestGuestChatGuidesOwnerToLogin(t *testing.T) {
	html := chatComponent(true)

	if !strings.Contains(html, `href="/login?redirect=/agent"`) {
		t.Fatalf("guest chat HTML missing owner login link")
	}
	for _, forbidden := range []string{"No account needed", "Sign up", `href="/setup"`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("guest chat HTML contains %q", forbidden)
		}
	}
}
