package auth

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func resetMigrationState(t *testing.T, value map[string]*Account) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	mutex.Lock()
	oldAccounts, oldSessions, oldTokens, oldPasskeys := accounts, sessions, tokens, passkeys
	accounts = value
	sessions = map[string]*Session{}
	tokens = map[string]*Token{}
	passkeys = map[string]*Passkey{}
	mutex.Unlock()
	oldHooks := accountDeleteHooks
	accountDeleteHooks = nil
	t.Cleanup(func() {
		mutex.Lock()
		accounts, sessions, tokens, passkeys = oldAccounts, oldSessions, oldTokens, oldPasskeys
		mutex.Unlock()
		accountDeleteHooks = oldHooks
	})
}

func TestMigrateKeepsOldestAdminAfterBackup(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"new-admin": {ID: "new-admin", Admin: true, Created: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		"old-admin": {ID: "old-admin", Admin: true, Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		"member":    {ID: "member", Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
	var deleted []string
	RegisterAccountDeleteHook("record", func(id string) error { deleted = append(deleted, id); return nil })
	backupCalled := false
	result, err := MigrateSingleOwner(func() (string, error) { backupCalled = true; return "/backup/data", nil })
	if err != nil {
		t.Fatal(err)
	}
	if !backupCalled || result.OwnerID != "old-admin" || result.Deleted != 2 || result.BackupPath != "/backup/data" {
		t.Fatalf("result = %#v, backup=%v", result, backupCalled)
	}
	owner, err := Owner()
	if err != nil || owner.ID != "old-admin" || !owner.Admin || !owner.Approved {
		t.Fatalf("owner = %#v, %v", owner, err)
	}
	if !reflect.DeepEqual(deleted, []string{"member", "new-admin"}) {
		t.Fatalf("deleted = %v", deleted)
	}
}

func TestMigrateNoAdminResetsAllAccounts(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"a": {ID: "a"}, "b": {ID: "b"}})
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reset || result.OwnerID != "" || result.Deleted != 2 || len(accounts) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestMigrateBackupFailureDoesNotMutate(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"owner": {ID: "owner"}})
	wantErr := errors.New("disk full")
	_, err := MigrateSingleOwner(func() (string, error) { return "", wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	if accounts["owner"].Admin || accounts["owner"].Approved {
		t.Fatalf("account mutated: %#v", accounts["owner"])
	}
}

func TestMigrateCleanupFailureIsRetryable(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"admin": {ID: "admin", Admin: true}, "other": {ID: "other"}})
	fail := true
	RegisterAccountDeleteHook("failing", func(string) error {
		if fail {
			return errors.New("write failed")
		}
		return nil
	})
	if _, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil }); err == nil {
		t.Fatal("expected cleanup error")
	}
	if _, ok := accounts["other"]; !ok {
		t.Fatal("account removed before all cleanup hooks succeeded")
	}
	fail = false
	if _, err := MigrateSingleOwner(func() (string, error) { return "/backup/data-2", nil }); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateNeverSelectsStoredMicroAccount(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"micro": {ID: "micro", Admin: true, Created: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		"owner": {ID: "owner", Admin: true, Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.OwnerID != "owner" {
		t.Fatalf("owner = %q, want owner", result.OwnerID)
	}
	if _, ok := accounts["micro"]; ok {
		t.Fatal("stored micro login account survived migration")
	}
}

func TestMigrateBreaksAdminTiesByIDAndRemovesCredentials(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"z-admin": {ID: "z-admin", Admin: true},
		"a-admin": {ID: "a-admin", Admin: true},
	})
	sessions["s"] = &Session{ID: "s", Account: "z-admin"}
	tokens["t"] = &Token{ID: "t", Account: "z-admin"}
	passkeys["p"] = &Passkey{ID: "p", Account: "z-admin"}
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.OwnerID != "a-admin" {
		t.Fatalf("owner = %q", result.OwnerID)
	}
	if len(sessions) != 0 || len(tokens) != 0 || len(passkeys) != 0 {
		t.Fatalf("legacy credentials remain: sessions=%d tokens=%d passkeys=%d", len(sessions), len(tokens), len(passkeys))
	}
}

func TestMigrateCompletedMarkerMakesRetryNoOp(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"owner": {ID: "owner"}})
	backups := 0
	backup := func() (string, error) { backups++; return fmt.Sprintf("/backup/%d", backups), nil }
	if _, err := MigrateSingleOwner(backup); err != nil {
		t.Fatal(err)
	}
	result, err := MigrateSingleOwner(backup)
	if err != nil {
		t.Fatal(err)
	}
	if result.Migrated || backups != 1 {
		t.Fatalf("result=%#v backups=%d", result, backups)
	}
}

func TestMigrateZeroAccountsDoesNotBackup(t *testing.T) {
	resetMigrationState(t, map[string]*Account{})
	called := false
	result, err := MigrateSingleOwner(func() (string, error) { called = true; return "/backup/data", nil })
	if err != nil {
		t.Fatal(err)
	}
	if called || result.Migrated {
		t.Fatalf("empty migration = %#v backup=%v", result, called)
	}
}
