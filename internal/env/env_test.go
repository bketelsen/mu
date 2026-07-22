package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsUnsetKeysOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nexport MU_TEST_ONE=one\nMU_TEST_TWO=\"two\"\nALREADY_SET=fromfile\n\nBAD LINE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY_SET", "fromenv")
	os.Unsetenv("MU_TEST_ONE")
	os.Unsetenv("MU_TEST_TWO")

	Load(path)

	if got := os.Getenv("MU_TEST_ONE"); got != "one" {
		t.Errorf("MU_TEST_ONE = %q, want one", got)
	}
	if got := os.Getenv("MU_TEST_TWO"); got != "two" {
		t.Errorf("MU_TEST_TWO = %q, want two (quotes stripped)", got)
	}
	if got := os.Getenv("ALREADY_SET"); got != "fromenv" {
		t.Errorf("ALREADY_SET = %q, want fromenv (env must win over file)", got)
	}
}

func TestLoadMissingFileIsNoop(t *testing.T) {
	Load(filepath.Join(t.TempDir(), "does-not-exist"))
}
