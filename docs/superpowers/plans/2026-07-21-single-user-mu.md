# Single-User Mu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert Mu into a private home server with exactly one login-capable owner, including a backed-up deterministic migration from legacy multi-account data.

**Architecture:** Keep account IDs as internal partition keys, but make `internal/auth` enforce one owner at every credential boundary. Run a synchronous, versioned migration before service/background startup, then wrap the HTTP stack in default-deny owner middleware and remove all account-provisioning and multi-user-only surfaces.

**Tech Stack:** Go 1.x standard library, `net/http`, JSON file storage under `~/.mu/data`, bcrypt sessions/PATs, WebAuthn, OAuth 2.1, existing Go test suite.

## Global Constraints

- Permit exactly one login-capable human account per Mu instance.
- First-run setup is the only supported account creation flow.
- Retain account IDs in domain records and service signatures as internal partition keys.
- Keep the non-login `micro` system identity.
- Require owner authentication for every application and service except setup/auth callbacks, validated webhooks, static assets, and health/version probes.
- Disable x402 as an authentication alternative while retaining the owner's wallet and outbound x402 client.
- Before destructive legacy migration, atomically create a complete timestamped sibling backup of the data directory and abort startup if backup fails.
- Keep the oldest admin by `Created`, tie-breaking by account ID; if no admin exists, delete all accounts and return to setup.
- Messaging integrations accept only a linked owner in direct messages and never auto-create accounts.
- Remove multi-user-only behavior rather than hiding it.
- Do not add a multi-user compatibility flag or owner self-delete endpoint.

---

### Task 1: Enforce The Owner Invariant

**Files:**
- Create: `internal/auth/owner_test.go`
- Modify: `internal/auth/auth.go:21-240,316-473,733-811`
- Modify: `internal/setup/setup.go:18-31,68-77`
- Delete: `internal/auth/admin_bootstrap_test.go`

**Interfaces:**
- Produces: `var ErrNoOwner error`, `var ErrOwnerExists error`, `var ErrMultipleAccounts error`
- Produces: `func Owner() (*Account, error)`
- Produces: `func OwnerExists() bool`
- Produces: `func IsOwner(accountID string) bool`
- Changes: `Create`, `Login`, `CreateSession`, and `ValidatePAT` accept only the sole owner identity.
- Consumed by: migration, HTTP middleware, OAuth, Google, setup, and channel tasks.

- [ ] **Step 1: Write owner-invariant tests**

Create `internal/auth/owner_test.go` with table/state tests that restore package globals during cleanup:

```go
package auth

import (
	"errors"
	"testing"
	"time"
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
```

Add this test helper in the same file:

```go
func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
```

Import `golang.org/x/crypto/bcrypt` in the test.

- [ ] **Step 2: Run the tests and verify the red state**

Run: `go test ./internal/auth -run 'Test(CreateEstablishesOnlyOwner|OwnerRejectsZeroAndMultipleAccounts|CredentialBoundariesRejectNonOwner)' -count=1`

Expected: FAIL because `Owner`, `OwnerExists`, `IsOwner`, and the owner errors do not exist and `Create` still accepts later accounts.

- [ ] **Step 3: Implement singleton owner operations and credential guards**

Add to `internal/auth/auth.go`:

```go
var (
	ErrNoOwner         = errors.New("owner is not configured")
	ErrOwnerExists     = errors.New("Mu already has an owner")
	ErrMultipleAccounts = errors.New("legacy accounts require single-owner migration")
)

func ownerLocked() (*Account, error) {
	if len(accounts) == 0 {
		return nil, ErrNoOwner
	}
	if len(accounts) != 1 {
		return nil, ErrMultipleAccounts
	}
	for _, acc := range accounts {
		return acc, nil
	}
	return nil, ErrNoOwner
}

func Owner() (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()
	return ownerLocked()
}

func OwnerExists() bool {
	_, err := Owner()
	return err == nil
}

func IsOwner(accountID string) bool {
	mutex.Lock()
	defer mutex.Unlock()
	owner, err := ownerLocked()
	return err == nil && owner.ID == accountID
}
```

At the top of `Create`, reject `len(accounts) != 0`, then unconditionally set `acc.Admin = true`, `acc.Approved = true`, and `acc.Banned = false` before saving. Delete `shouldBootstrapAdmin` and remove the now-unused `os` import.

In `Login`, `CreateSession`, and the successful branch of `ValidatePAT`, require `ownerLocked().ID` to equal the credential account before creating/returning access. Use the same check when accepting cookie/session tokens in `GetSession` so stale legacy sessions fail closed.

Change setup lifecycle checks to `auth.OwnerExists()` and leave explicit owner promotion out of `applySetup`, because `auth.Create` now establishes the invariant.

- [ ] **Step 4: Run focused auth and setup tests**

Run: `go test ./internal/auth ./internal/setup -count=1`

Expected: PASS. If tests that intentionally build multi-account maps fail, update those fixtures only where the test is not specifically exercising legacy migration.

- [ ] **Step 5: Commit the owner boundary**

```bash
git add internal/auth/auth.go internal/auth/owner_test.go internal/auth/admin_bootstrap_test.go internal/setup/setup.go
git commit -m "auth: enforce a single owner"
```

---

### Task 2: Add Atomic Data-Directory Backups

**Files:**
- Create: `internal/data/backup.go`
- Create: `internal/data/backup_test.go`
- Modify: `internal/data/data.go:54-72`

**Interfaces:**
- Produces: `func Dir() string`
- Produces: `func Backup(now time.Time) (string, error)`
- Consumed by: `auth.MigrateSingleOwner` in Task 3.

- [ ] **Step 1: Write backup behavior and failure tests**

Create `internal/data/backup_test.go`:

```go
package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupCopiesDataBesideSourceAndPreservesMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(Dir(), "nested"), 0700); err != nil { t.Fatal(err) }
	source := filepath.Join(Dir(), "nested", "secret.json")
	if err := os.WriteFile(source, []byte(`{"token":"kept"}`), 0600); err != nil { t.Fatal(err) }

	got, err := Backup(time.Date(2026, 7, 21, 12, 30, 0, 0, time.UTC))
	if err != nil { t.Fatalf("Backup: %v", err) }
	want := filepath.Join(filepath.Dir(Dir()), "data-backup-20260721T123000Z")
	if got != want { t.Fatalf("Backup path = %q, want %q", got, want) }
	b, err := os.ReadFile(filepath.Join(got, "nested", "secret.json"))
	if err != nil || string(b) != `{"token":"kept"}` { t.Fatalf("backup content = %q, %v", b, err) }
	info, err := os.Stat(filepath.Join(got, "nested", "secret.json"))
	if err != nil || info.Mode().Perm() != 0600 { t.Fatalf("backup mode = %v, %v", info.Mode(), err) }
	if _, err := os.Stat(got + ".tmp"); !os.IsNotExist(err) { t.Fatalf("temporary backup remains: %v", err) }
}

func TestBackupFailureLeavesNoPartialDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(Dir(), 0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(Dir(), "unreadable"), []byte("x"), 0000); err != nil { t.Fatal(err) }
	if os.Geteuid() == 0 { t.Skip("root can read mode 000 files") }
	_, err := Backup(time.Date(2026, 7, 21, 12, 30, 0, 0, time.UTC))
	if err == nil { t.Fatal("Backup succeeded with unreadable source") }
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(Dir()), "data-backup-20260721T123000Z.tmp")); !os.IsNotExist(statErr) {
		t.Fatalf("partial backup remains: %v", statErr)
	}
}
```

- [ ] **Step 2: Run the backup tests and verify failure**

Run: `go test ./internal/data -run TestBackup -count=1`

Expected: FAIL because `Dir` and `Backup` do not exist.

- [ ] **Step 3: Implement confined source lookup and atomic backup**

Move the base-directory expression into:

```go
func Dir() string {
	return filepath.Join(os.ExpandEnv("$HOME/.mu"), "data")
}
```

Use `Dir()` inside `dataPath`. In `backup.go`, implement `Backup(now)` with `filepath.WalkDir`: create `<parent>/data-backup-<UTC timestamp>.tmp` as `0700`, reject an existing final path, recreate directories with their source permissions, copy regular files with `io.Copy`, preserve regular-file permissions with `os.Chmod`, recreate symlinks with `os.Readlink`/`os.Symlink`, reject unsupported file types, close every destination before continuing, remove the temporary tree on any error, and finish with `os.Rename(temp, final)`. If the source directory does not exist, create an empty backup directory through the same temp/rename flow.

The public function must have this exact shape:

```go
func Backup(now time.Time) (backupPath string, err error) {
	source := Dir()
	name := "data-backup-" + now.UTC().Format("20060102T150405Z")
	final := filepath.Join(filepath.Dir(source), name)
	temp := final + ".tmp"
	if _, err := os.Stat(final); err == nil {
		return "", fmt.Errorf("backup already exists: %s", final)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	_ = os.RemoveAll(temp)
	defer func() {
		if err != nil { _ = os.RemoveAll(temp) }
	}()
	sourceInfo, statErr := os.Stat(source)
	if os.IsNotExist(statErr) {
		if err = os.MkdirAll(temp, 0700); err != nil { return "", err }
		if err = os.Rename(temp, final); err != nil { return "", err }
		return final, nil
	}
	if statErr != nil { return "", statErr }
	if err = os.MkdirAll(temp, sourceInfo.Mode().Perm()); err != nil { return "", err }
	if err = os.Chmod(temp, sourceInfo.Mode().Perm()); err != nil { return "", err }
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil { return relErr }
		if rel == "." { return nil }
		destination := filepath.Join(temp, rel)
		info, infoErr := entry.Info()
		if infoErr != nil { return infoErr }
		switch {
		case entry.IsDir():
			if err := os.Mkdir(destination, info.Mode().Perm()); err != nil { return err }
			return os.Chmod(destination, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil { return err }
			return os.Symlink(target, destination)
		case info.Mode().IsRegular():
			src, err := os.Open(path)
			if err != nil { return err }
			dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
			if err != nil { _ = src.Close(); return err }
			_, copyErr := io.Copy(dst, src)
			srcErr := src.Close()
			dstErr := dst.Close()
			if copyErr != nil { return copyErr }
			if srcErr != nil { return srcErr }
			if dstErr != nil { return dstErr }
			return os.Chmod(destination, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported data file type: %s", path)
		}
	})
	if err != nil { return "", err }
	if err = os.Rename(temp, final); err != nil { return "", err }
	return final, nil
}
```

Import `io`, `io/fs`, `fmt`, `os`, `path/filepath`, and `time`. Do not shell out to `cp` or follow symlinks.

- [ ] **Step 4: Run data tests**

Run: `go test ./internal/data -count=1`

Expected: PASS, including path confinement, JSON save, SQLite, and backup tests.

- [ ] **Step 5: Commit backup support**

```bash
git add internal/data/data.go internal/data/backup.go internal/data/backup_test.go
git commit -m "data: add atomic migration backups"
```

---

### Task 3: Migrate Legacy Accounts Synchronously

**Files:**
- Create: `internal/auth/migration.go`
- Create: `internal/auth/migration_test.go`
- Modify: `internal/auth/auth.go:273-314`
- Modify cleanup return signatures in: `blog/blog.go`, `social/social.go`, `apps/apps.go`, `stream/stream.go`, `user/user.go`, `mail/mail.go`, `wallet/wallet.go`, `wallet/basewallet.go`, `agent/micro/userstore.go`, `client/discord/discord.go`, `client/telegram/telegram.go`, `client/whatsapp/whatsapp.go`, `internal/app/prefs.go`, `internal/memory/memory.go`

**Interfaces:**
- Produces: `type AccountDeleteHook struct { Name string; Delete func(string) error }`
- Produces: `func RegisterAccountDeleteHook(name string, deleteFunc func(string) error)`
- Produces: `type MigrationResult struct { OwnerID, BackupPath string; Deleted int; Reset bool; Migrated bool }`
- Produces: `func MigrateSingleOwner(backup func() (string, error)) (MigrationResult, error)`
- Changes: account cleanup is synchronous, named, error-returning, and retry-safe.
- Consumed by: startup wiring in Task 4.

- [ ] **Step 1: Write deterministic selection, backup, cleanup, and retry tests**

Create `internal/auth/migration_test.go`. Use this helper to restore package state and isolate persisted files:

```go
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
	if err != nil { t.Fatal(err) }
	if !backupCalled || result.OwnerID != "old-admin" || result.Deleted != 2 || result.BackupPath != "/backup/data" {
		t.Fatalf("result = %#v, backup=%v", result, backupCalled)
	}
	owner, err := Owner()
	if err != nil || owner.ID != "old-admin" || !owner.Admin || !owner.Approved { t.Fatalf("owner = %#v, %v", owner, err) }
	if !reflect.DeepEqual(deleted, []string{"member", "new-admin"}) { t.Fatalf("deleted = %v", deleted) }
}

func TestMigrateNoAdminResetsAllAccounts(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"a": {ID: "a"}, "b": {ID: "b"}})
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil { t.Fatal(err) }
	if !result.Reset || result.OwnerID != "" || result.Deleted != 2 || len(accounts) != 0 { t.Fatalf("result = %#v", result) }
}

func TestMigrateBackupFailureDoesNotMutate(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"owner": {ID: "owner"}})
	wantErr := errors.New("disk full")
	_, err := MigrateSingleOwner(func() (string, error) { return "", wantErr })
	if !errors.Is(err, wantErr) { t.Fatalf("error = %v", err) }
	if accounts["owner"].Admin || accounts["owner"].Approved { t.Fatalf("account mutated: %#v", accounts["owner"]) }
}

func TestMigrateCleanupFailureIsRetryable(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"admin": {ID: "admin", Admin: true}, "other": {ID: "other"}})
	fail := true
	RegisterAccountDeleteHook("failing", func(string) error { if fail { return errors.New("write failed") }; return nil })
	if _, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil }); err == nil { t.Fatal("expected cleanup error") }
	if _, ok := accounts["other"]; !ok { t.Fatal("account removed before all cleanup hooks succeeded") }
	fail = false
	if _, err := MigrateSingleOwner(func() (string, error) { return "/backup/data-2", nil }); err != nil { t.Fatal(err) }
}

func TestMigrateNeverSelectsStoredMicroAccount(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"micro": {ID: "micro", Admin: true, Created: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		"owner": {ID: "owner", Admin: true, Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil { t.Fatal(err) }
	if result.OwnerID != "owner" { t.Fatalf("owner = %q, want owner", result.OwnerID) }
	if _, ok := accounts["micro"]; ok { t.Fatal("stored micro login account survived migration") }
}
```

Add these cases to the same file:

```go
func TestMigrateBreaksAdminTiesByIDAndRemovesCredentials(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"z-admin": {ID: "z-admin", Admin: true},
		"a-admin": {ID: "a-admin", Admin: true},
	})
	sessions["s"] = &Session{ID: "s", Account: "z-admin"}
	tokens["t"] = &Token{ID: "t", Account: "z-admin"}
	passkeys["p"] = &Passkey{ID: "p", Account: "z-admin"}
	result, err := MigrateSingleOwner(func() (string, error) { return "/backup/data", nil })
	if err != nil { t.Fatal(err) }
	if result.OwnerID != "a-admin" { t.Fatalf("owner = %q", result.OwnerID) }
	if len(sessions) != 0 || len(tokens) != 0 || len(passkeys) != 0 {
		t.Fatalf("legacy credentials remain: sessions=%d tokens=%d passkeys=%d", len(sessions), len(tokens), len(passkeys))
	}
}

func TestMigrateCompletedMarkerMakesRetryNoOp(t *testing.T) {
	resetMigrationState(t, map[string]*Account{"owner": {ID: "owner"}})
	backups := 0
	backup := func() (string, error) { backups++; return fmt.Sprintf("/backup/%d", backups), nil }
	if _, err := MigrateSingleOwner(backup); err != nil { t.Fatal(err) }
	result, err := MigrateSingleOwner(backup)
	if err != nil { t.Fatal(err) }
	if result.Migrated || backups != 1 { t.Fatalf("result=%#v backups=%d", result, backups) }
}

func TestMigrateZeroAccountsDoesNotBackup(t *testing.T) {
	resetMigrationState(t, map[string]*Account{})
	called := false
	result, err := MigrateSingleOwner(func() (string, error) { called = true; return "/backup/data", nil })
	if err != nil { t.Fatal(err) }
	if called || result.Migrated { t.Fatalf("empty migration = %#v backup=%v", result, called) }
}
```

Import `errors`, `fmt`, `reflect`, `testing`, and `time`.

- [ ] **Step 2: Run migration tests and verify failure**

Run: `go test ./internal/auth -run TestMigrate -count=1`

Expected: FAIL because the migration API and named hooks do not exist.

- [ ] **Step 3: Implement migration selection and named synchronous hooks**

In `migration.go`, define `singleOwnerMigrationVersion = 1`, persist `single_owner_migration.json`, always classify account ID `micro` as a non-surviving legacy login record, sort the remaining candidate admins by `Created` ascending then `ID`, and sort deletion IDs before invoking hooks. If `micro` is the only stored account, back up and delete it, then return to setup. Use this sequence:

```go
backupPath, err := backup()
if err != nil { return MigrationResult{}, fmt.Errorf("backup data: %w", err) }
for _, id := range deleteIDs {
	for _, hook := range accountDeleteHooks {
		if err := hook.Delete(id); err != nil {
			return MigrationResult{}, fmt.Errorf("cleanup %s for %s: %w", hook.Name, id, err)
		}
	}
	removeAccountCredentials(id)
}
if survivor != nil {
	survivor.Admin = true
	survivor.Approved = true
	survivor.Banned = false
}
if err := persistAuthState(); err != nil { return MigrationResult{}, err }
if err := data.SaveJSON("single_owner_migration.json", map[string]int{"version": singleOwnerMigrationVersion}); err != nil { return MigrationResult{}, err }
```

`removeAccountCredentials` must remove the account, all sessions, PATs, and passkeys for that ID while holding `mutex`. `persistAuthState` must return errors from all four `SaveJSON` calls instead of discarding them. Remove the goroutine-based `DeleteAccount`; no public owner deletion API remains.

After all account hooks succeed and before writing the migration marker, remove legacy `invites.json` and `invite_requests.json` with `data.DeleteFile`. Treat `os.IsNotExist` as success and return any other error. Account moderation state is normalized by deleting non-survivors and setting the survivor to `Admin: true`, `Approved: true`, and `Banned: false`.

Change each registered cleanup function listed in **Files** to `func(string) error`. Return the relevant `data.SaveJSON`/`data.DeleteFile` error, treating `os.IsNotExist` as success. Where one function writes multiple stores, return the first error. Update direct call sites and tests to check or explicitly discard the error.

- [ ] **Step 4: Run migration and affected package tests**

Run: `go test ./internal/auth ./blog ./social ./apps ./stream ./user ./mail ./wallet ./agent/micro ./client/discord ./client/telegram ./client/whatsapp ./internal/app ./internal/memory -count=1`

Expected: PASS with cleanup failures propagated and migration tests green.

- [ ] **Step 5: Commit the migration**

```bash
git add internal/auth/migration.go internal/auth/migration_test.go internal/auth/auth.go blog/blog.go social/social.go apps/apps.go stream/stream.go user/user.go mail/mail.go wallet/wallet.go wallet/basewallet.go agent/micro/userstore.go client/discord/discord.go client/telegram/telegram.go client/whatsapp/whatsapp.go internal/app/prefs.go internal/memory/memory.go
git commit -m "auth: migrate legacy accounts to one owner"
```

---

### Task 4: Run Migration Before Services And Background Work

**Files:**
- Modify: `main.go:98-182,355-389`
- Modify: `main_test.go`

**Interfaces:**
- Consumes: `data.Backup`, `auth.RegisterAccountDeleteHook`, `auth.MigrateSingleOwner`.
- Produces: `func registerAccountCleanup()` and `func migrateSingleOwner() error` for startup and focused testing.

- [ ] **Step 1: Add startup-order tests**

Add to `main_test.go` a test that injects a backup function through a package variable and records hook execution:

```go
func TestMigrateSingleOwnerUsesDataBackup(t *testing.T) {
	oldBackup := backupData
	oldRun := runOwnerMigration
	called := false
	backupData = func() (string, error) { called = true; return "/tmp/mu-backup", nil }
	runOwnerMigration = func(backup func() (string, error)) (auth.MigrationResult, error) {
		path, err := backup()
		return auth.MigrationResult{Migrated: true, BackupPath: path, OwnerID: "owner"}, err
	}
	t.Cleanup(func() { backupData, runOwnerMigration = oldBackup, oldRun })
	if err := migrateSingleOwner(); err != nil { t.Fatal(err) }
	if !called { t.Fatal("startup migration did not invoke data backup") }
}
```

To avoid exposing production mutation helpers solely for this test, make `migrateSingleOwner` accept the migration function as a package variable:

```go
var backupData = func() (string, error) { return data.Backup(time.Now()) }
var runOwnerMigration = auth.MigrateSingleOwner
```

The test can replace `runOwnerMigration` with a closure that calls and verifies the supplied backup function, avoiding changes to auth internals.

- [ ] **Step 2: Run the focused startup test and verify failure**

Run: `go test . -run TestMigrateSingleOwnerUsesDataBackup -count=1`

Expected: FAIL because `backupData`, `runOwnerMigration`, and `migrateSingleOwner` do not exist.

- [ ] **Step 3: Register cleanup and migrate immediately after settings load**

Add:

```go
func migrateSingleOwner() error {
	result, err := runOwnerMigration(backupData)
	if err != nil { return err }
	if result.Migrated {
		app.Log("auth", "single-owner migration complete owner=%s deleted=%d reset=%v backup=%s", result.OwnerID, result.Deleted, result.Reset, result.BackupPath)
	}
	return nil
}
```

Create `registerAccountCleanup()` using one named registration per package. Call it after `settings.Load()` and before `data.Load()`, then call `migrateSingleOwner()`. On error, log once and `os.Exit(1)` before any `Load`, goroutine, listener, SMTP server, or messaging client starts.

Remove the old anonymous `AccountDeleteHooks` append at lines 359-375. Move `news.StartSentimentLoop`, MCP gateway startup, `data.StartIndexing`, `search.StartTopics`, `blog.StartOpinion`, `blog.StartNotes`, and channel `Load` calls to positions after successful migration. Keep package data initialization needed by cleanup before migration; package `init` functions already run before `main`.

- [ ] **Step 4: Run startup and migration tests**

Run: `go test . ./internal/auth ./internal/data -count=1`

Expected: PASS. Inspect `main.go` to confirm the migration call textually precedes every `go ` startup and every `Start...` call.

- [ ] **Step 5: Commit startup integration**

```bash
git add main.go main_test.go
git commit -m "main: migrate owner before starting services"
```

---

### Task 5: Make HTTP And Protocol Access Private By Default

**Files:**
- Create: `internal/app/private.go`
- Create: `internal/app/private_test.go`
- Modify: `internal/app/status.go`
- Modify: `main.go:1165-1232,1489-1633`
- Modify: `internal/api/mcp.go:515-560`
- Modify: `internal/api/mcp_test.go`

**Interfaces:**
- Produces: `func Private(next http.Handler, setupNeeded func() bool) http.Handler`
- Consumes: `auth.RequireSession`, `WantsJSON`, `SendsJSON`.
- Behavior: exact unauthenticated allowlist; every other route requires the owner.

- [ ] **Step 1: Write middleware allowlist and denial tests**

Create `internal/app/private_test.go` with this table and denial test:

```go
func TestPrivatePublicAllowlist(t *testing.T) {
	public := []string{
		"/setup", "/login", "/passkey/login/begin", "/passkey/login/finish",
		"/oauth2/google", "/oauth2/callback", "/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource", "/oauth/register", "/oauth/authorize",
		"/oauth/token", "/whatsapp/webhook", "/wallet/stripe/webhook", "/status", "/version", "/mu.css",
	}
	for _, path := range public {
		t.Run(path, func(t *testing.T) {
			hit := false
			h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return path == "/setup" })
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			if !hit { t.Fatalf("public path %s was blocked", path) }
		})
	}
}

func TestPrivateDeniesApplicationRoutesByDefault(t *testing.T) {
	denied := []string{"/home", "/news", "/docs", "/mcp", "/a2a", "/api", "/agent", "/images/file/private.png", "/apps/private.js", "/session", "/verify"}
	for _, path := range denied {
		t.Run(path, func(t *testing.T) {
			hit := false
			h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return false })
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if path == "/mcp" || path == "/a2a" || path == "/api" { req.Header.Set("Accept", "application/json") }
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if hit { t.Fatalf("private path %s reached handler", path) }
			if WantsJSON(req) && rr.Code != http.StatusUnauthorized { t.Fatalf("%s = %d, want 401", path, rr.Code) }
			if !WantsJSON(req) && rr.Code != http.StatusFound { t.Fatalf("%s = %d, want redirect", path, rr.Code) }
		})
	}
}
```

Add an authenticated-owner case:

```go
func TestPrivateAllowsOwner(t *testing.T) {
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil { t.Fatal(err) }
		owner, err = auth.Owner()
	}
	if err != nil { t.Fatal(err) }
	sess, err := auth.Login(owner.ID, "owner-pass")
	if err != nil { t.Fatal(err) }
	hit := false
	h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !hit { t.Fatal("owner request was blocked") }
}
```

Import `errors` and `time`.

Add a minimal-health response test in `internal/app/private_test.go` and a version-shape test in `main_test.go`:

```go
func TestPublicStatusContainsNoOperationalDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	StatusHandler(rr, httptest.NewRequest(http.MethodGet, "/status", nil))
	if strings.TrimSpace(rr.Body.String()) != `{"status":"ok"}` { t.Fatalf("status body = %s", rr.Body.String()) }
}

func TestVersionInfoDoesNotExposeServiceTopology(t *testing.T) {
	info := versionInfo()
	if _, ok := info["services"]; ok { t.Fatalf("public version exposes services: %#v", info) }
}
```

- [ ] **Step 2: Run private middleware tests and verify failure**

Run: `go test ./internal/app -run TestPrivate -count=1`

Expected: FAIL because `Private` does not exist.

- [ ] **Step 3: Implement exact lifecycle-aware allowlisting**

Implement `Private` with exact path matching for auth endpoints and embedded assets. The function must:

```go
func Private(next http.Handler, setupNeeded func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPrivatePath(r.URL.Path, setupNeeded()) {
			next.ServeHTTP(w, r)
			return
		}
		if _, _, err := auth.RequireSession(r); err == nil {
			next.ServeHTTP(w, r)
			return
		}
		if WantsJSON(r) || SendsJSON(r) || r.URL.Path == "/mcp" || r.URL.Path == "/a2a" || strings.HasPrefix(r.URL.Path, "/api/") {
			RespondError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		RedirectToLogin(w, r)
	})
}
```

Allow `/setup` only while `setupNeeded()` is true. Allow `/login`, the two passkey login endpoints, Google login/callback, OAuth discovery/register/authorize/token, validated webhook endpoints, `/status`, `/version`, and only exact embedded root assets needed by login/setup: `/mu.css`, `/mu.js`, `/qrcode.js`, `/sdk.css`, `/sdk.js`, `/manifest.webmanifest`, `/favicon.ico`, and the fixed icon/image basenames under `internal/app/html`. Do not allow arbitrary file extensions: `/images/file/private.png` and `/apps/private.js` still require owner authentication. Do not allow passkey registration/deletion, `/session`, `/verify`, or any application prefix.

Reduce the unauthenticated `/status` response to `{"status":"ok"}` with no account count, online count, provider configuration, wallet configuration, or service details. Keep detailed operational diagnostics behind owner-authenticated `/admin/diagnostics`. Keep `/version` limited to build/runtime identifiers and remove the in-process service list.

Wrap the existing top-level server handler with `app.Private(..., setup.Needed)`. Delete the `authenticated` map and its token/x402 alternative branch. Simplify `/` to redirect an authenticated owner to `/home`; setup redirection remains the middleware/setup handler responsibility. Remove logged-out guest home rendering.

Delete unauthenticated `GuestNewsSearch` branches from `ExecuteToolAs` and `ExecuteTool`; all MCP execution now requires owner context.

- [ ] **Step 4: Verify private access and no x402 bypass**

Run: `go test ./internal/app ./internal/api . -count=1`

Expected: PASS. Add this `internal/api/mcp_test.go` regression:

```go
func TestExecuteToolDoesNotUseGuestNewsSearch(t *testing.T) {
	old := GuestNewsSearch
	GuestNewsSearch = func(string) (string, error) { return "guest result", nil }
	t.Cleanup(func() { GuestNewsSearch = old })
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	text, isErr, err := ExecuteTool(req, "news_search", map[string]any{"query": "private"})
	if err == nil || !isErr || !strings.Contains(strings.ToLower(text), "authentication") {
		t.Fatalf("ExecuteTool = %q, %v, %v", text, isErr, err)
	}
}
```

- [ ] **Step 5: Commit private-by-default middleware**

```bash
git add internal/app/private.go internal/app/private_test.go internal/app/status.go internal/api/mcp.go internal/api/mcp_test.go main.go
git commit -m "http: require owner access by default"
```

---

### Task 6: Remove Every Account-Provisioning Path

**Files:**
- Delete: `internal/auth/invite.go`, `internal/auth/invite_test.go`, `internal/app/captcha.go`
- Modify: `internal/app/app.go:21-70,390-654,780-909,1080-1130`
- Modify: `internal/app/google_oauth.go:51-52,97-158,213-281,339-347`
- Create: `internal/app/google_oauth_test.go`
- Modify: `internal/auth/oauth.go:261-369`
- Modify: `internal/auth/oauth_test.go`
- Modify: `internal/setup/setup_test.go`
- Modify: `internal/api/mcp_page.go`, `internal/api/api.go`, `internal/api/mcp_micro.go`
- Modify: `main.go:489-535,1411-1434,1691-1713`

**Interfaces:**
- Consumes: `auth.Owner`, `auth.IsOwner`.
- Behavior: setup alone calls `auth.Create`; Google, OAuth, web, and MCP never provision.

- [ ] **Step 1: Write Google and OAuth owner-only tests**

Create `internal/app/google_oauth_test.go` with a test seam `var fetchGoogleUser = googleUserInfo` and this reusable owner setup:

```go
func setGoogleOwner(t *testing.T, email string) *auth.Account {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil {
			t.Fatal(err)
		}
		owner, err = auth.Owner()
	}
	if err != nil { t.Fatal(err) }
	oldEmail, oldVerified := owner.Email, owner.EmailVerified
	owner.Email, owner.EmailVerified = email, true
	if err := auth.UpdateAccount(owner); err != nil { t.Fatal(err) }
	t.Cleanup(func() {
		owner.Email, owner.EmailVerified = oldEmail, oldVerified
		_ = auth.UpdateAccount(owner)
	})
	return owner
}
```

Import `errors` and `time`, then test `resolveGoogleOwner` directly:

```go
func TestResolveGoogleOwnerNeverCreatesAccount(t *testing.T) {
	owner := setGoogleOwner(t, "owner@example.com")
	if acc, err := resolveGoogleOwner(&googleUser{Email: "other@example.com", EmailVerified: true}); err == nil || acc != nil {
		t.Fatalf("unlinked Google identity resolved to %#v, %v", acc, err)
	}
	if current, err := auth.Owner(); err != nil || current.ID != owner.ID { t.Fatalf("owner changed to %#v, %v", current, err) }
}

func TestResolveGoogleOwnerAcceptsLinkedEmail(t *testing.T) {
	setGoogleOwner(t, "owner@example.com")
	acc, err := resolveGoogleOwner(&googleUser{Email: "OWNER@example.com", EmailVerified: true})
	if err != nil || acc.ID != "owner" { t.Fatalf("owner = %#v, %v", acc, err) }
}
```

Add OAuth seams and a POST authorization regression in `internal/auth/oauth_test.go`:

```go
func TestOAuthAuthorizeUsesAuthenticatedOwnerID(t *testing.T) {
	oldLogin, oldCreate := loginForOAuth, createOAuthCode
	oauthMu.Lock()
	oldClients := oauthClients
	oauthClients = map[string]*OAuthClient{"client": {ClientID: "client", RedirectURIs: []string{"https://client.example/callback"}}}
	oauthMu.Unlock()
	loginForOAuth = func(string, string) (*Session, error) { return &Session{Account: "canonical-owner"}, nil }
	gotAccount := ""
	createOAuthCode = func(clientID, accountID, redirectURI, challenge, method string) string {
		gotAccount = accountID
		return "code"
	}
	t.Cleanup(func() {
		loginForOAuth, createOAuthCode = oldLogin, oldCreate
		oauthMu.Lock()
		oauthClients = oldClients
		oauthMu.Unlock()
	})

	form := url.Values{"client_id": {"client"}, "redirect_uri": {"https://client.example/callback"}, "username": {"submitted-alias"}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	OAuthAuthorizePostHandler(rr, req)
	if gotAccount != "canonical-owner" { t.Fatalf("authorization account = %q", gotAccount) }
}
```

Define `var loginForOAuth = Login` and `var createOAuthCode = CreateAuthorizationCode` in `oauth.go`, and use them in the POST handler.

- [ ] **Step 2: Run provisioning tests and verify red state**

Run: `go test ./internal/app ./internal/auth -run 'TestResolveGoogleOwner|TestOAuthAuthorize' -count=1`

Expected: FAIL because `resolveGoogleOwner` does not exist and current Google flow provisions unknown emails.

- [ ] **Step 3: Delete signup/invite code and make Google resolve only the owner**

Delete signup rate-limit state, signup/invite templates and handlers, captcha code, blocked-user account-page link, and invite auth storage. Remove `/signup`, `/request-invite`, `/invite`, and `/admin/invite` registration plus CSRF exceptions. Remove the MCP `signup` tool registration entirely.

Replace `findOrCreateGoogleAccount` and `uniqueUsernameFromEmail` with:

```go
func resolveGoogleOwner(info *googleUser) (*auth.Account, error) {
	owner, err := auth.Owner()
	if err != nil { return nil, err }
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if email == "" || !info.EmailVerified || strings.ToLower(owner.Email) != email || !owner.EmailVerified {
		return nil, errors.New("Google account is not linked to this Mu owner")
	}
	return owner, nil
}
```

The callback returns `403` for an unlinked Google identity and never calls `auth.Create`. Keep connect mode, but permit it only from an existing owner session.

In OAuth authorization POST, retain the session returned by `auth.Login` and pass `sess.Account` to `CreateAuthorizationCode`. Validate the requested client and redirect URI before login or redirect in both GET and POST handlers.

Change login copy from “Sign up” to “Run first-time setup if this is a new server.” Remove Google injection from any deleted signup template. Update MCP/API generated copy to describe owner PAT/login only and remove signup/tool references.

- [ ] **Step 4: Verify no provisioning references remain in executable Go**

Run: `go test ./internal/auth ./internal/app ./internal/setup ./internal/api . -count=1`

Expected: PASS.

Run: `rg -n 'auth\.Create\(|Name:\s*"signup"|/signup|request-invite|CreateInvite|findOrCreateGoogleAccount|auto-create account' --glob '*.go'`

Expected at the Task 6 boundary: `internal/setup/setup.go` and the three channel `autoCreateAccount` helpers scheduled for removal in Task 7 may still contain production `auth.Create` calls; test fixtures may contain direct calls. No web, MCP, invite, or Google production provisioning route/tool/helper remains. Task 7's final channel scan removes the remaining channel callers.

- [ ] **Step 5: Commit provisioning removal**

```bash
git add -A internal/auth internal/app internal/setup internal/api main.go
git commit -m "auth: remove account provisioning paths"
```

---

### Task 7: Restrict Messaging Channels To Linked Owner DMs

**Files:**
- Modify: `client/discord/discord.go:1-148,292-380`
- Modify: `client/discord/interactions.go:70-150`
- Modify: `client/discord/link.go`
- Modify: `client/discord/link_test.go`
- Modify: `client/telegram/telegram.go:1-13,180-266,354-446`
- Modify: `client/telegram/telegram_test.go`
- Modify: `client/whatsapp/whatsapp.go:1-14,165-247,325-380`
- Modify: `client/whatsapp/whatsapp_test.go`

**Interfaces:**
- Consumes: `auth.Owner`, `auth.IsOwner`, `auth.Login`.
- Produces per channel: owner-only `linkAccount`/`LinkAccount` returning `error`.
- Behavior: shared contexts return before link, history, tool, or agent processing.

- [ ] **Step 1: Write owner-link and shared-context tests**

For each channel package, extract a pure authorization helper and test it. Use the same contract with package-local names:

```go
type messageAccess int

const (
	accessIgnore messageAccess = iota
	accessNeedsLink
	accessOwner
)

func classifyMessage(isDirect bool, linkedAccount string) messageAccess {
	if !isDirect { return accessIgnore }
	if linkedAccount == "" || !auth.IsOwner(linkedAccount) { return accessNeedsLink }
	return accessOwner
}
```

Add this table in each channel package, replacing `ownerIDForTest(t)` with a package test helper that calls `auth.Owner()` or creates `owner/owner-pass` when absent:

```go
func TestClassifyMessage(t *testing.T) {
	ownerID := ownerIDForTest(t)
	tests := []struct {
		name string
		direct bool
		linked string
		want messageAccess
	}{
		{"shared owner", false, ownerID, accessIgnore},
		{"unlinked DM", true, "", accessNeedsLink},
		{"stale legacy DM", true, "legacy", accessNeedsLink},
		{"owner DM", true, ownerID, accessOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMessage(tt.direct, tt.linked); got != tt.want { t.Fatalf("got %v, want %v", got, tt.want) }
		})
	}
}
```

Implement the helper in each test file:

```go
func ownerIDForTest(t *testing.T) string {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil { t.Fatal(err) }
		owner, err = auth.Owner()
	}
	if err != nil { t.Fatal(err) }
	return owner.ID
}
```

In `client/discord/link_test.go`, add:

```go
func TestGenerateLinkCodeRejectsNonOwner(t *testing.T) {
	ownerIDForTest(t)
	if code, err := GenerateLinkCode("legacy"); err == nil || code != "" { t.Fatalf("code=%q err=%v", code, err) }
}
```

Extend `client/whatsapp/whatsapp_test.go` with a POST-handler test that clears `WHATSAPP_APP_SECRET`, sends a webhook body, and expects `503 Service Unavailable` without dispatching a message. A publicly exempt webhook must never accept an unsigned POST because the validation secret is absent.

- [ ] **Step 2: Run channel tests and verify red state**

Run: `go test ./client/discord ./client/telegram ./client/whatsapp -run 'Test(ClassifyMessage|OwnerLink)' -count=1`

Expected: FAIL because the access classifiers do not exist and current handlers auto-create accounts/respond in groups.

- [ ] **Step 3: Remove auto-provisioning and guard handlers before work**

Delete all three `autoCreateAccount` functions and their username/password-generation helpers. In each inbound handler:

1. Return immediately for a group, guild, server channel, or non-DM interaction before sending a response.
2. Process `link` only in a DM.
3. After a successful password login or code redemption, verify `auth.IsOwner(accountID)` before persisting the link.
4. For an unlinked or stale link, send `Link this bot to your Mu owner account before using it.` and return.
5. Call `agent.QueryWithOpts` with the owner ID and `Public: false`.

Require `WHATSAPP_APP_SECRET` for every webhook POST before dispatching messages; return `503` when absent and `401` when `X-Hub-Signature-256` is invalid. Keep GET verification gated by exact `WHATSAPP_VERIFY_TOKEN` equality.

Change link persistence functions to reject non-owner IDs:

```go
func linkAccount(externalID, accountID string) error {
	if !auth.IsOwner(accountID) { return errors.New("only the Mu owner can be linked") }
	linkMu.Lock()
	defer linkMu.Unlock()
	links[externalID] = accountID
	return data.SaveJSON("telegram_links.json", links)
}
```

Apply the same logic and each package's file name to Discord and WhatsApp. Make Discord `GenerateLinkCode` return `(string, error)` and reject non-owner account IDs; update `app.DiscordLinkCodeFunc` and account-page handling accordingly.

- [ ] **Step 4: Run all channel tests**

Run: `go test ./client/discord ./client/telegram ./client/whatsapp ./internal/app . -count=1`

Expected: PASS with no account creation and no shared-context agent calls.

Run: `rg -n 'autoCreateAccount|Auto-created account|created your account|Public:\s*!isDM|Public:\s*isGroup' client`

Expected: no matches.

- [ ] **Step 5: Commit owner-only messaging**

```bash
git add client/discord client/telegram client/whatsapp internal/app/app.go main.go
git commit -m "channels: allow linked owner DMs only"
```

---

### Task 8: Remove Multi-User Administration And Domain Behavior

**Files:**
- Modify: `internal/auth/auth.go:30-45,218-240,550-727`
- Delete: `internal/auth/post_rules_test.go`
- Modify: `internal/auth/account_lifecycle_test.go`
- Modify: `internal/app/controls.go:100-130,220-252`
- Modify: `internal/app/prefs.go:11-16,98-143`
- Modify: `internal/api/mcp.go:390-405,468-485`
- Modify: `admin/admin.go`
- Modify: `admin/console.go`
- Modify: `admin/flag.go`
- Modify: `main.go:284-338,1273-1315,1388-1389,1453-1472,1636-1689,1715-1750`
- Modify: `blog/blog.go`, `social/social.go`, `apps/apps.go`, `stream/stream.go`, `user/user.go`, `mail/mail.go`
- Modify: `wallet/wallet.go`, `wallet/handlers.go`, `wallet/wallet_test.go`
- Delete: `blog/activitypub.go`, `blog/activitypub_test.go`

**Interfaces:**
- Removes: bans, approval APIs, post-age gates, presence lists, blocking, transfers, user administration, ActivityPub/WebFinger.
- Retains: `Account.Admin` for owner/admin operational pages plus `Account.Approved` and `Account.Banned` only to deserialize and normalize legacy account JSON; no runtime feature reads or mutates moderation state after migration.

- [ ] **Step 1: Write absence and owner-write regression tests**

Add tests that capture the resulting contract:

```go
func TestOwnerCanAlwaysWrite(t *testing.T) {
	withAccounts(t, map[string]*Account{"owner": {ID: "owner", Admin: true, Approved: true, Created: time.Now()}})
	if !CanWrite("owner") { t.Fatal("owner should be allowed to write") }
	if CanWrite("missing") { t.Fatal("missing account should not be allowed to write") }
}
```

Replace `CanPost` with the simpler `CanWrite(accountID string) bool { return IsOwner(accountID) }` so central charged-write middleware still has an explicit identity assertion.

Add this `admin/admin_test.go` case, using a local helper that creates/loads the owner and returns an `auth.CreateSession(owner.ID)` cookie:

```go
func TestAdminDashboardContainsOnlyOperationalLinks(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(ownerSessionCookie(t))
	rr := httptest.NewRecorder()
	AdminHandler(rr, req)
	body := rr.Body.String()
	for _, want := range []string{"/admin/env", "/admin/server", "/admin/log"} {
		if !strings.Contains(body, want) { t.Errorf("dashboard missing %s", want) }
	}
	for _, forbidden := range []string{"Users", "Invites", "Moderation", "Blocklist", "/admin/users", "/admin/invite"} {
		if strings.Contains(body, forbidden) { t.Errorf("dashboard contains %q", forbidden) }
	}
}
```

Add to `wallet/wallet_test.go`:

```go
func TestWalletTransferRemoved(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/wallet/transfer", nil)
	req.AddCookie(ownerSessionCookie(t))
	rr := httptest.NewRecorder()
	Handler(rr, req)
	if rr.Code != http.StatusNotFound { t.Fatalf("GET /wallet/transfer = %d, want 404", rr.Code) }

	req = httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.AddCookie(ownerSessionCookie(t))
	rr = httptest.NewRecorder()
	Handler(rr, req)
	if strings.Contains(rr.Body.String(), "/wallet/transfer") { t.Fatal("wallet page still links to transfers") }
}
```

Implement this helper independently in each package test file:

```go
func ownerSessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil { t.Fatal(err) }
		owner, err = auth.Owner()
	}
	if err != nil { t.Fatal(err) }
	sess, err := auth.CreateSession(owner.ID)
	if err != nil { t.Fatal(err) }
	return &http.Cookie{Name: "session", Value: sess.Token}
}
```

- [ ] **Step 2: Run focused tests and verify red state**

Run: `go test ./internal/auth ./admin ./wallet -run 'Test(OwnerCanAlwaysWrite|AdminDashboard|WalletTransferRemoved)' -count=1`

Expected: FAIL because `CanWrite` does not exist and multi-user UI/routes remain.

- [ ] **Step 3: Remove auth and application multi-user primitives**

Remove `Banned` behavior, `GetAllAccounts` from production callers, presence maps/functions, `VerificationRequired`, `CanPost`, `PostBlockReason`, `IsNewAccount`, `IsBanned`, `BanAccount`, `UnbanAccount`, and `ApproveAccount`. Keep `Admin`, `Approved`, and `Banned` fields for persisted migration input; owner creation/migration always sets admin/approved true and banned false.

Delete block/unblock/blocked actions from `ControlsHandler`, remove the `Blocked` preference map and its helpers, and remove MCP tools `wallet_transfer`, `block_user`, and `unblock_user`.

Replace charged-write checks in `main.go` with:

```go
if !auth.CanWrite(sess.Account) {
	http.Error(w, "owner authentication required", http.StatusForbidden)
	return
}
```

Remove post-age and ban filtering from blog/social/apps/stream. Remove owner auto-ban/moderation branches in apps and user status; retain content validation that rejects unsafe generated output without mutating account state. Remove online-user WebSocket/ping/status fields and public profile routing (`/@...`). Mail recipient lookup uses `auth.Owner()` and accepts only that local mailbox.

Replace every background-job account enumeration or persisted arbitrary user target with `auth.Owner()` at execution time. If no owner exists, the job returns without work; if a persisted target differs from `owner.ID`, discard that target. Add focused tests around any changed scheduler helper to prove it never runs as a deleted legacy account.

- [ ] **Step 4: Remove user administration, transfer, and federation routes/code**

Reduce `admin.AdminHandler` to operational links only. Delete `UsersHandler`, `ModerateHandler`, account/invite/ban/approve console commands, and account-specific flag actions. Keep server configuration, logs, diagnostics, mail spam controls, API usage, and generic content deletion.

Delete transfer constants/functions/UI/tests: `DailyTransferCap`, `OpTransfer`, `TxTransfer` handling, `DailyTransferTotal`, `TransferCredits`, `/wallet/transfer`, `handleTransferPage`, `handleTransfer`, and `respondTransferError`. Preserve historical transaction rendering as generic debit/credit text if old `type:"transfer"` records are encountered; do not expose a new transfer action.

Delete ActivityPub implementation/tests and remove WebFinger, actor, inbox, outbox, and content-negotiation routing from `main.go` and blog handlers. These removed paths fall through to authenticated `404` after login and never publish owner content.

- [ ] **Step 5: Run affected tests and static absence checks**

Run: `go test ./internal/auth ./internal/app ./internal/api ./admin ./wallet ./blog ./social ./apps ./stream ./user ./mail . -count=1`

Expected: PASS.

Run: `rg -n 'GetAllAccounts|IsBanned|BanAccount|UnbanAccount|ApproveAccount|IsNewAccount|PostBlockReason|GetOnlineUsers|wallet_transfer|block_user|unblock_user|/wallet/transfer|WebFingerHandler|ActorHandler|OutboxHandler|InboxHandler' --glob '*.go'`

Expected: no production matches. Migration tests may construct multiple account values directly but must not expose runtime multi-user APIs.

- [ ] **Step 6: Commit multi-user feature removal**

```bash
git add -A internal/auth internal/app internal/api admin wallet blog social apps stream user mail main.go
git commit -m "product: remove multi-user features"
```

---

### Task 9: Update Product Copy And Documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/ABOUT.md`, `docs/ARCHITECTURE.md`, `docs/CLI.md`, `docs/DISCORD.md`, `docs/ENVIRONMENT_VARIABLES.md`, `docs/INSTALLATION.md`, `docs/MCP.md`, `docs/MESSAGING_SYSTEM.md`, `docs/SECURITY.md`, `docs/SYSTEM_DESIGN.md`, `docs/TELEGRAM.md`, `docs/WALLET_AND_CREDITS.md`, `docs/WHITEPAPER.md`, `docs/X402.md`
- Delete: `docs/ACTIVITYPUB.md`
- Modify generated UI copy in: `chat/chat.go`, `places/places.go`, `wallet/handlers.go`, `home/*`, `agents/*`, `internal/api/*`, `internal/app/*`
- Modify: `docs/docs_test.go`, `chat/chat_test.go`

**Interfaces:**
- Documents: private single-owner setup, owner authentication, PAT/CLI/MCP, linked-owner DMs, migration selection/backup, and outbound-only x402.
- Removes: hosted/public/multi-user/signup/invite/transfer/federation claims.

- [ ] **Step 1: Add documentation and rendered-copy regression tests**

Extend `docs/docs_test.go` to walk current embedded Markdown and fail on product claims that are no longer valid:

```go
var forbiddenSingleOwnerCopy = []string{
	"sign up", "signup tool", "invite-only", "transfer credits to other users",
	"auto-created accounts", "activitypub federation", "pay per request without an account",
}
```

Compare lowercase content, and maintain a small explicit exclusion for historical design/spec files under `docs/superpowers/`; do not exclude current user documentation. Update `chat/chat_test.go` to expect only a `/login` link in logged-out copy.

- [ ] **Step 2: Run docs/copy tests and verify red state**

Run: `go test ./docs ./chat -count=1`

Expected: FAIL and list current stale signup/public/multi-user claims.

- [ ] **Step 3: Rewrite current product documentation and copy**

Make README lead with “A private, single-owner home server.” Document:

- First-run setup creates the owner and no later accounts can be added.
- Every web/service/API surface is private after setup.
- Password, passkey, linked Google, PAT, OAuth, CLI, API, MCP, and A2A access all resolve to the owner.
- Discord, Telegram, and WhatsApp work only in DMs after linking the owner.
- Legacy migration backs up the complete data directory, keeps the oldest admin, or resets admin-less instances.
- x402 remains available only for owner-initiated outbound calls; incoming payment does not bypass authentication.

Delete public hosting, signup/invite, account-free API, local-user moderation, transfer, public social/profile, and ActivityPub instructions. Update architecture tables and `CLAUDE.md` conventions so future code does not reintroduce auto-provisioning.

Remove stale “Try without an account”, signup, invite, public pricing, multi-user counts, and account-free x402 strings from rendered Go templates. Logged-out pages present only owner login or first-run setup.

- [ ] **Step 4: Run copy scans and docs tests**

Run: `go test ./docs ./chat ./internal/app ./internal/api -count=1`

Expected: PASS.

Run: `rg -ni 'sign.?up|invite|auto.?creat.*account|transfer credits|activitypub|without an account|public version|public viewing' README.md CLAUDE.md docs --glob '!docs/superpowers/**'`

Expected: no stale product claims. Matches in descriptions of removed behavior must be rewritten as explicit migration history only if operationally necessary.

- [ ] **Step 5: Commit documentation cleanup**

```bash
git add -A README.md CLAUDE.md docs chat places wallet home agents internal/api internal/app
git commit -m "docs: describe private single-owner Mu"
```

---

### Task 10: Verify The Complete Single-Owner System

**Files:**
- Modify only files required to fix failures found by this verification.

**Interfaces:**
- Verifies all interfaces and constraints produced by Tasks 1-9.

- [ ] **Step 1: Run formatting and inspect changes**

Run: `gofmt -w $(git diff --name-only --diff-filter=ACM HEAD~9 -- '*.go')`

Expected: command exits `0`. Then run `git diff --check`; expected: no output.

- [ ] **Step 2: Run the complete short test suite**

Run: `go test ./... -short -count=1`

Expected: PASS with zero failing packages.

- [ ] **Step 3: Run static analysis**

Run: `go vet ./...`

Expected: exit `0` with no diagnostics.

- [ ] **Step 4: Build every package**

Run: `go build ./...`

Expected: exit `0`.

- [ ] **Step 5: Run final invariant scans**

Run:

```bash
rg -n 'Name:\s*"signup"|/signup|request-invite|CreateInvite|autoCreateAccount|findOrCreateGoogleAccount|GetAllAccounts|BanAccount|ApproveAccount|wallet_transfer|block_user|/wallet/transfer|WebFingerHandler|ActorHandler' --glob '*.go'
rg -ni 'sign.?up|invite-only|auto.?created accounts|transfer credits to other users|without an account' README.md CLAUDE.md docs --glob '!docs/superpowers/**'
```

Expected: no production-code or current-documentation matches. Test names documenting rejected legacy behavior are acceptable only when they assert the feature is absent.

- [ ] **Step 6: Inspect migration and route ordering manually**

Confirm in `main.go` that `registerAccountCleanup()` and `migrateSingleOwner()` run before every service `Load`, listener, channel startup, indexing worker, and background loop. Confirm `app.Private` wraps the complete default mux and that no inner branch accepts x402 in place of owner authentication.

- [ ] **Step 7: Commit verification fixes, if any**

If formatting or verification required tracked changes:

```bash
git add -A
git commit -m "test: verify single-owner system"
```

If the worktree is already clean, do not create an empty commit.
