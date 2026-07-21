package data

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupPropagatesFileSyncFailureAndCleansTemporaryDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), "state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("sync failed")
	oldSync := syncFile
	syncFile = func(*os.File) error { return wantErr }
	t.Cleanup(func() { syncFile = oldSync })

	_, err := Backup(time.Date(2026, 7, 21, 12, 30, 0, 0, time.UTC))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Backup error = %v, want %v", err, wantErr)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(Dir()), "data-backup-20260721T123000Z.tmp")); !os.IsNotExist(statErr) {
		t.Fatalf("partial backup remains: %v", statErr)
	}
}

func TestBackupCopiesDataBesideSourceAndPreservesMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(Dir(), "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(Dir(), "nested", "secret.json")
	if err := os.WriteFile(source, []byte(`{"token":"kept"}`), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := Backup(time.Date(2026, 7, 21, 12, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	want := filepath.Join(filepath.Dir(Dir()), "data-backup-20260721T123000Z")
	if got != want {
		t.Fatalf("Backup path = %q, want %q", got, want)
	}
	b, err := os.ReadFile(filepath.Join(got, "nested", "secret.json"))
	if err != nil || string(b) != `{"token":"kept"}` {
		t.Fatalf("backup content = %q, %v", b, err)
	}
	info, err := os.Stat(filepath.Join(got, "nested", "secret.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode = %v, %v", info.Mode(), err)
	}
	if _, err := os.Stat(got + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary backup remains: %v", err)
	}
}

func TestBackupFailureLeavesNoPartialDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), "unreadable"), []byte("x"), 0000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read mode 000 files")
	}
	_, err := Backup(time.Date(2026, 7, 21, 12, 30, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("Backup succeeded with unreadable source")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(Dir()), "data-backup-20260721T123000Z.tmp")); !os.IsNotExist(statErr) {
		t.Fatalf("partial backup remains: %v", statErr)
	}
}
