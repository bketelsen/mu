// Package testutil contains process-level isolation helpers for tests.
package testutil

import (
	"os"
	"testing"
)

// RunWithTempHome runs a package's tests with all home-directory persistence
// redirected to a temporary directory.
func RunWithTempHome(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-test-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", home); err != nil {
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
