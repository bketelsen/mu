package discord

import (
	"errors"
	"testing"
	"time"

	"mu/internal/auth"
)

func ownerIDForTest(t *testing.T) string {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil {
			t.Fatal(err)
		}
		owner, err = auth.Owner()
	}
	if err != nil {
		t.Fatal(err)
	}
	return owner.ID
}

func TestGenerateLinkCodeReplacesExistingAccountCode(t *testing.T) {
	codeMu.Lock()
	codes = map[string]*linkCode{}
	codeMu.Unlock()

	ownerID := ownerIDForTest(t)
	first, err := GenerateLinkCode(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateLinkCode(ownerID)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatalf("expected replacement code to differ from first code")
	}
	if account, ok := redeemCode(first); ok || account != "" {
		t.Fatalf("old code redeemed as account %q, ok=%v; want expired replacement", account, ok)
	}
	if account, ok := redeemCode(second); !ok || account != ownerID {
		t.Fatalf("new code redeemed as account %q, ok=%v; want %s, true", account, ok, ownerID)
	}
	if account, ok := redeemCode(second); ok || account != "" {
		t.Fatalf("code redeemed twice as account %q, ok=%v; want consumed", account, ok)
	}
}

func TestGenerateLinkCodeRejectsNonOwner(t *testing.T) {
	ownerIDForTest(t)
	if code, err := GenerateLinkCode("legacy"); err == nil || code != "" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestClassifyMessage(t *testing.T) {
	ownerID := ownerIDForTest(t)
	tests := []struct {
		name   string
		direct bool
		linked string
		want   messageAccess
	}{
		{"shared owner", false, ownerID, accessIgnore},
		{"unlinked DM", true, "", accessNeedsLink},
		{"stale legacy DM", true, "legacy", accessNeedsLink},
		{"owner DM", true, ownerID, accessOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMessage(tt.direct, tt.linked); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmptyMessageReplyRequiresOwnerLink(t *testing.T) {
	if got := emptyMessageReply(""); got != "Link this bot to your Mu owner account before using it." {
		t.Fatalf("unlinked reply = %q", got)
	}
	if got := emptyMessageReply(ownerIDForTest(t)); got != "Ask me anything — I'm Micro, your agent across news, mail, markets, weather, search and more." {
		t.Fatalf("owner reply = %q", got)
	}
}

func TestRedeemCodeRejectsExpiredCode(t *testing.T) {
	codeMu.Lock()
	codes = map[string]*linkCode{
		"expired": {Account: "alice", ExpiresAt: time.Now().Add(-time.Minute)},
	}
	codeMu.Unlock()

	if account, ok := redeemCode("expired"); ok || account != "" {
		t.Fatalf("expired code redeemed as account %q, ok=%v; want rejected", account, ok)
	}
	codeMu.Lock()
	_, stillPresent := codes["expired"]
	codeMu.Unlock()
	if stillPresent {
		t.Fatal("expired code was not removed")
	}
}
