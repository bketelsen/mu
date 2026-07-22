package data

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const removeWalletPaymentsMarker = "remove-wallet-payments-v1.done"

var removeWalletPaymentDataFiles = []string{
	"wallets.json",
	"transactions.json",
	"daily_usage.json",
	"trade_wallets.json",
}

// RemoveWalletPayments permanently removes legacy payment persistence once.
func RemoveWalletPayments() error {
	return removeWalletPayments(os.UserHomeDir)
}

func removeWalletPayments(homeDir func() (string, error)) error {
	home, err := homeDir()
	if err != nil {
		return fmt.Errorf("remove wallet payments migration: resolve home directory: %w", err)
	}
	if home == "" {
		return errors.New("remove wallet payments migration: resolve home directory: empty path")
	}
	return removeWalletPaymentFiles(home)
}

func removeWalletPaymentFiles(home string) error {
	dataDir := filepath.Join(home, ".mu", "data")
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if info, err := os.Lstat(marker); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("remove wallet payments migration: marker %s is not a regular file", marker)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove wallet payments migration: inspect marker %s: %w", marker, err)
	}

	targets := make([]string, 0, len(removeWalletPaymentDataFiles)+1)
	for _, name := range removeWalletPaymentDataFiles {
		targets = append(targets, filepath.Join(dataDir, name))
	}
	targets = append(targets, filepath.Join(home, ".mu", "keys", "wallet.seed"))
	for _, target := range targets {
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove wallet payments migration: delete %s: %w", target, err)
		}
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("remove wallet payments migration: target remains after deletion: %s", target)
			}
			return fmt.Errorf("remove wallet payments migration: verify deletion of %s: %w", target, err)
		}
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("remove wallet payments migration: create marker directory %s: %w", dataDir, err)
	}
	temp := marker + ".tmp"
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("remove wallet payments migration: create temporary marker %s: %w", temp, err)
	}
	if _, err = f.WriteString("completed\n"); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("remove wallet payments migration: write marker %s: %w", marker, err)
	}
	if err := os.Rename(temp, marker); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("remove wallet payments migration: commit marker %s: %w", marker, err)
	}
	if err := syncDirectory(dataDir); err != nil {
		_ = os.Remove(marker)
		return fmt.Errorf("remove wallet payments migration: sync marker directory %s: %w", dataDir, err)
	}
	return nil
}
