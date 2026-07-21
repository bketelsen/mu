package telegram

import (
	"errors"
	"testing"
	"time"

	"mu/agent"
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

func TestLinkAccountRejectsNonOwner(t *testing.T) {
	ownerIDForTest(t)
	if err := linkAccount("telegram-test", "legacy"); err == nil {
		t.Fatal("linkAccount accepted a non-owner account")
	}
}

func TestGetHistoryReturnsCopy(t *testing.T) {
	telegramID := "history-copy-test"
	historyMu.Lock()
	histories[telegramID] = []agent.QueryMessage{{Role: "user", Text: "original"}}
	historyMu.Unlock()
	defer func() {
		historyMu.Lock()
		delete(histories, telegramID)
		historyMu.Unlock()
	}()

	got := getHistory(telegramID)
	got[0].Text = "mutated"

	again := getHistory(telegramID)
	if again[0].Text != "original" {
		t.Fatalf("getHistory returned mutable backing storage; got %q", again[0].Text)
	}
}
