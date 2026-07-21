package auth

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func withAccounts(t *testing.T, value map[string]*Account) {
	t.Helper()
	mutex.Lock()
	oldAccounts, oldSessions, oldTokens := accounts, sessions, tokens
	accounts, sessions, tokens = value, map[string]*Session{}, map[string]*Token{}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts, sessions, tokens = oldAccounts, oldSessions, oldTokens
		mutex.Unlock()
	})
}

func TestCreateEstablishesOnlyOwner(t *testing.T) {
	withAccounts(t, map[string]*Account{})
	first := &Account{ID: "owner", Name: "Owner", Secret: "secret1", Created: time.Now()}
	if err := Create(first); err != nil {
		t.Fatalf("Create(first): %v", err)
	}
	if !first.Admin || !first.Approved || !IsOwner("owner") {
		t.Fatalf("first account = %#v, want owner/admin/approved", first)
	}
	if err := Create(&Account{ID: "second", Secret: "secret2"}); !errors.Is(err, ErrOwnerExists) {
		t.Fatalf("Create(second) = %v, want ErrOwnerExists", err)
	}
}

func TestCreateRejectsReservedMicroOwner(t *testing.T) {
	withAccounts(t, map[string]*Account{})
	if err := Create(&Account{ID: "micro", Name: "micro", Secret: "secret1"}); err == nil {
		t.Fatal("Create accepted the reserved micro owner")
	}
}

func TestOwnerRejectsZeroAndMultipleAccounts(t *testing.T) {
	withAccounts(t, map[string]*Account{})
	if _, err := Owner(); !errors.Is(err, ErrNoOwner) {
		t.Fatalf("Owner(empty) = %v, want ErrNoOwner", err)
	}
	accounts["a"] = &Account{ID: "a"}
	accounts["b"] = &Account{ID: "b"}
	if _, err := Owner(); !errors.Is(err, ErrMultipleAccounts) {
		t.Fatalf("Owner(multiple) = %v, want ErrMultipleAccounts", err)
	}
}

func TestRunForOwnerOnlyExecutesForCurrentOwner(t *testing.T) {
	t.Run("current owner executes", func(t *testing.T) {
		withAccounts(t, map[string]*Account{"owner": {ID: "owner"}})
		var executed string
		RunForOwner("owner", func(owner *Account) { executed = owner.ID })
		if executed != "owner" {
			t.Fatalf("executed as %q, want owner", executed)
		}
	})

	t.Run("no owner is a no-op", func(t *testing.T) {
		withAccounts(t, map[string]*Account{})
		executed := false
		RunForOwner("owner", func(*Account) { executed = true })
		if executed {
			t.Fatal("callback executed without an owner")
		}
	})

	t.Run("stale target is discarded", func(t *testing.T) {
		withAccounts(t, map[string]*Account{"owner": {ID: "owner"}})
		executed := false
		RunForOwner("legacy", func(*Account) { executed = true })
		if executed {
			t.Fatal("callback executed for stale target")
		}
	})
}

func TestCredentialBoundariesRejectNonOwner(t *testing.T) {
	withAccounts(t, map[string]*Account{
		"owner":  {ID: "owner", Secret: mustHash(t, "owner-pass")},
		"legacy": {ID: "legacy", Secret: mustHash(t, "legacy-pass")},
	})
	if _, err := Login("legacy", "legacy-pass"); err == nil {
		t.Fatal("Login accepted legacy non-owner")
	}
	if _, err := CreateSession("legacy"); err == nil {
		t.Fatal("CreateSession accepted legacy non-owner")
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
