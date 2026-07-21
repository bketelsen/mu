package agent

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"mu/internal/ai"
	"mu/internal/auth"
)

func TestExtractMemoryOnlyRunsForCurrentOwner(t *testing.T) {
	if os.Getenv("MU_TEST_NO_OWNER_MEMORY") == "1" {
		if _, err := auth.Owner(); !errors.Is(err, auth.ErrNoOwner) {
			t.Fatalf("Owner() = %v, want no owner", err)
		}
		oldAsk, oldSet := askBackground, setMemory
		calls, writes := 0, 0
		askBackground = func(*ai.Prompt) (string, error) { calls++; return `{"key":"preference","value":"tea"}`, nil }
		setMemory = func(string, string, string) { writes++ }
		t.Cleanup(func() { askBackground, setMemory = oldAsk, oldSet })
		extractMemory("owner", "remember I like tea")
		if calls != 0 || writes != 0 {
			t.Fatalf("model calls/writes = %d/%d, want 0/0", calls, writes)
		}
		return
	}
	for _, tt := range []struct {
		name       string
		target     string
		wantCalls  int
		wantWrites int
	}{
		{name: "current owner", target: "owner", wantCalls: 1, wantWrites: 1},
		{name: "stale target", target: "legacy", wantCalls: 0, wantWrites: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ensureMemoryOwner(t)

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
	t.Run("no owner", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		cmd := exec.Command(os.Args[0], "-test.run=^TestExtractMemoryOnlyRunsForCurrentOwner$")
		cmd.Env = append(os.Environ(), "MU_TEST_NO_OWNER_MEMORY=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("no-owner subprocess: %v\n%s", err, output)
		}
	})
}

func ensureMemoryOwner(t *testing.T) *auth.Account {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		owner = &auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
