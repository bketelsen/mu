package app

import (
	"errors"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
)

func setGoogleOwner(t *testing.T, email string) *auth.Account {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	data.Load()
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
	oldEmail, oldVerified := owner.Email, owner.EmailVerified
	owner.Email, owner.EmailVerified = email, true
	if err := auth.UpdateAccount(owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		owner.Email, owner.EmailVerified = oldEmail, oldVerified
		_ = auth.UpdateAccount(owner)
	})
	return owner
}

func TestResolveGoogleOwnerNeverCreatesAccount(t *testing.T) {
	owner := setGoogleOwner(t, "owner@example.com")
	if acc, err := resolveGoogleOwner(&googleUser{Email: "other@example.com", EmailVerified: true}); err == nil || acc != nil {
		t.Fatalf("unlinked Google identity resolved to %#v, %v", acc, err)
	}
	if current, err := auth.Owner(); err != nil || current.ID != owner.ID {
		t.Fatalf("owner changed to %#v, %v", current, err)
	}
}

func TestResolveGoogleOwnerAcceptsLinkedEmail(t *testing.T) {
	setGoogleOwner(t, "owner@example.com")
	acc, err := resolveGoogleOwner(&googleUser{Email: "OWNER@example.com", EmailVerified: true})
	if err != nil || acc.ID != "owner" {
		t.Fatalf("owner = %#v, %v", acc, err)
	}
}
