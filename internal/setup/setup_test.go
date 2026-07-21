package setup

import (
	"testing"

	"mu/internal/settings"
)

// resetProviders sandboxes HOME and clears every provider credential from the
// environment and the (package-level) settings map, so each test starts from
// an unconfigured instance.
func resetProviders(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{
		"ANTHROPIC_API_KEY", "ATLAS_API_KEY",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"COPILOT_GITHUB_TOKEN",
	} {
		t.Setenv(k, "")
		settings.Set(k, "")
	}
}

func TestApplyProviderRequiresKeyWhenUnconfigured(t *testing.T) {
	resetProviders(t)

	if msg := applyProvider("copilot", "", ""); msg == "" {
		t.Fatal("copilot with no key and no stored token should error")
	}
	if msg := applyProvider("claude", "", ""); msg == "" {
		t.Fatal("claude with no key and no stored key should error")
	}
}

func TestApplyProviderAcceptsAlreadyConfiguredCopilot(t *testing.T) {
	resetProviders(t)
	settings.Set("COPILOT_GITHUB_TOKEN", "gho_existing")

	if msg := applyProvider("copilot", "", ""); msg != "" {
		t.Fatalf("copilot with stored token should succeed, got %q", msg)
	}
	if got := settings.Get("COPILOT_GITHUB_TOKEN"); got != "gho_existing" {
		t.Fatalf("stored token = %q, want gho_existing", got)
	}
}

func TestApplyProviderStoresProvidedKey(t *testing.T) {
	resetProviders(t)

	if msg := applyProvider("copilot", "gho_new", ""); msg != "" {
		t.Fatalf("copilot with key should succeed, got %q", msg)
	}
	if got := settings.Get("COPILOT_GITHUB_TOKEN"); got != "gho_new" {
		t.Fatalf("stored token = %q, want gho_new", got)
	}
}

func TestApplyProviderKeepsExistingOllamaBaseURL(t *testing.T) {
	resetProviders(t)
	settings.Set("OPENAI_BASE_URL", "http://gpu-box:8000/v1")

	if msg := applyProvider("ollama", "", ""); msg != "" {
		t.Fatalf("ollama should succeed, got %q", msg)
	}
	if got := settings.Get("OPENAI_BASE_URL"); got != "http://gpu-box:8000/v1" {
		t.Fatalf("base URL = %q, want existing http://gpu-box:8000/v1", got)
	}
}

func TestConfiguredProviderDetection(t *testing.T) {
	resetProviders(t)

	if v, _ := configuredProvider(); v != "" {
		t.Fatalf("configuredProvider on fresh instance = %q, want empty", v)
	}

	settings.Set("COPILOT_GITHUB_TOKEN", "gho_existing")
	v, label := configuredProvider()
	if v != "copilot" || label == "" {
		t.Fatalf("configuredProvider = %q/%q, want copilot with a label", v, label)
	}
}
