package agent

import (
	"errors"
	"testing"
	"time"

	"mu/internal/ai"
	"mu/internal/auth"
)

func TestExtractMemoryOnlyRunsForCurrentOwner(t *testing.T) {
	for _, tt := range []struct {
		name        string
		target      string
		removeOwner bool
		wantCalls   int
		wantWrites  int
	}{
		{name: "current owner", target: "owner", wantCalls: 1, wantWrites: 1},
		{name: "stale target", target: "legacy", wantCalls: 0, wantWrites: 0},
		{name: "no owner", target: "owner", removeOwner: true, wantCalls: 0, wantWrites: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			owner := ensureMemoryOwner(t)
			if tt.removeOwner {
				if err := auth.Delete(owner); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := auth.Create(owner); err != nil {
						t.Fatal(err)
					}
				})
			}

			oldAsk, oldSet := askBackground, setMemory
			calls, writes := 0, 0
			askBackground = func(*ai.Prompt) (string, error) {
				calls++
				return `{"key":"preference","value":"tea"}`, nil
			}
			setMemory = func(string, string, string) { writes++ }
			t.Cleanup(func() { askBackground, setMemory = oldAsk, oldSet })

			extractMemory(tt.target, "remember I like tea")
			if calls != tt.wantCalls || writes != tt.wantWrites {
				t.Fatalf("model calls/writes = %d/%d, want %d/%d", calls, writes, tt.wantCalls, tt.wantWrites)
			}
		})
	}
}

func ensureMemoryOwner(t *testing.T) *auth.Account {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		owner = &auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := auth.Delete(owner); err != nil {
				t.Fatal(err)
			}
		})
		return owner
	}
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
