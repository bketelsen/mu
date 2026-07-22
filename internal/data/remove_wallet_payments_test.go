package data

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLegacyPaymentFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("legacy-secret"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveWalletPaymentFilesDeletesAllTargetsAndMarksComplete(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".mu", "data")
	targets := []string{
		filepath.Join(dataDir, "wallets.json"),
		filepath.Join(dataDir, "transactions.json"),
		filepath.Join(dataDir, "daily_usage.json"),
		filepath.Join(dataDir, "trade_wallets.json"),
		filepath.Join(home, ".mu", "keys", "wallet.seed"),
	}
	for _, target := range targets {
		writeLegacyPaymentFile(t, target)
	}

	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("target %s remains; stat error = %v", target, err)
		}
	}
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if info, err := os.Stat(marker); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("marker %s missing or invalid: info=%v err=%v", marker, info, err)
	}
}

func TestRemoveWalletPaymentFilesAllowsMissingTargetsAndReruns(t *testing.T) {
	home := t.TempDir()
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

func TestRemoveWalletPaymentFilesDoesNotMarkPartialDeletion(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, ".mu", "data", "wallets.json")
	writeLegacyPaymentFile(t, filepath.Join(blocker, "child"))

	err := removeWalletPaymentFiles(home)
	if err == nil || !strings.Contains(err.Error(), blocker) {
		t.Fatalf("error = %v, want path %s", err, blocker)
	}
	marker := filepath.Join(home, ".mu", "data", removeWalletPaymentsMarker)
	if _, err := os.Lstat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("marker exists after failed deletion: %v", err)
	}

	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestRemoveWalletPaymentsFailsWhenHomeCannotResolve(t *testing.T) {
	want := errors.New("home unavailable")
	err := removeWalletPayments(func() (string, error) { return "", want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestRemoveWalletPaymentFilesRejectsNonRegularMarker(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, ".mu", "data", removeWalletPaymentsMarker)
	if err := os.MkdirAll(marker, 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("error = %v, want invalid marker path", err)
	}
}

func TestRemoveWalletPaymentFilesLeavesNoMarkerWhenMarkerWriteFails(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".mu", "data")
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if err := os.MkdirAll(marker+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("error = %v, want marker path", err)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("marker exists after marker-write failure: %v", err)
	}
}
