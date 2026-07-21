package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataPathRejectsNonTemporaryHomeInTests(t *testing.T) {
	t.Setenv("HOME", filepath.Clean(filepath.Join(os.TempDir(), "..", "mu-live-home")))
	if _, err := dataPath("accounts.json"); err == nil {
		t.Fatal("dataPath allowed a test to access a non-temporary home")
	}
}

func TestDataPathConfines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Keys that would escape the store must be rejected.
	escape := []string{
		"../escape.json",
		"a/../../escape.json",
		"apps/../../../etc/passwd",
		"..",
		"a/../..",
	}
	for _, k := range escape {
		if _, err := dataPath(k); err == nil {
			t.Errorf("dataPath(%q) allowed, want rejection", k)
		}
	}

	// Normal keys (including nested app/collection paths) must be allowed.
	ok := []string{
		"apps.json",
		"apps/notes/db/notes.json",
		"apps/notes/alice.json",
		"discord_links.json",
	}
	for _, k := range ok {
		if _, err := dataPath(k); err != nil {
			t.Errorf("dataPath(%q) rejected: %v", k, err)
		}
	}
}
