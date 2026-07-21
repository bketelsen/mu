package ai

import "testing"

// clearProviderEnv blanks every provider credential so each test controls
// exactly which providers appear configured, regardless of the host env.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"COPILOT_GITHUB_TOKEN", "COPILOT_CHAT_MODEL", "COPILOT_BACKGROUND_MODEL", "COPILOT_PREMIUM_MODEL",
		"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_PREMIUM_MODEL",
		"ATLAS_API_KEY", "OPENAI_API_KEY", "OPENAI_BASE_URL",
	} {
		t.Setenv(k, "")
	}
}

func TestResolveProviderCopilot(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_test")
	// Anthropic key also present: Copilot still wins — it is the gateway for
	// both model families when configured.
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	for _, model := range []string{"claude-sonnet-4.5", "gpt-4.1"} {
		provider, key, baseURL, err := resolveProvider(model)
		if err != nil {
			t.Fatalf("resolveProvider(%s): %v", model, err)
		}
		if provider != "copilot" || key != "gho_test" || baseURL != "" {
			t.Fatalf("resolveProvider(%s) = %s/%s/%s, want copilot/gho_test/\"\"", model, provider, key, baseURL)
		}
	}
}

func TestResolveProviderAtlasModelsStillRouteToAtlas(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_test")
	t.Setenv("ATLAS_API_KEY", "atlas_test")

	provider, _, _, err := resolveProvider(ModelDeepSeekFlash)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "atlascloud" {
		t.Fatalf("deepseek model routed to %s, want atlascloud", provider)
	}
}

func TestResolveProviderAnthropicWithoutCopilot(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	provider, _, _, err := resolveProvider("claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if provider != "anthropic" {
		t.Fatalf("provider = %s, want anthropic", provider)
	}
}

func TestModelDefaultsUnderCopilot(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_test")

	if got := DefaultModel(); got != defaultCopilotChatModel {
		t.Errorf("DefaultModel = %q, want %q", got, defaultCopilotChatModel)
	}
	if got := BackgroundModel(); got != defaultCopilotBackgroundModel {
		t.Errorf("BackgroundModel = %q, want %q", got, defaultCopilotBackgroundModel)
	}
	if got := PremiumModel(); got != defaultCopilotChatModel {
		t.Errorf("PremiumModel = %q, want chat model %q", got, defaultCopilotChatModel)
	}

	t.Setenv("COPILOT_CHAT_MODEL", "gpt-5")
	t.Setenv("COPILOT_BACKGROUND_MODEL", "gpt-4o-mini")
	t.Setenv("COPILOT_PREMIUM_MODEL", "claude-opus-4.1")
	if got := DefaultModel(); got != "gpt-5" {
		t.Errorf("DefaultModel override = %q", got)
	}
	if got := BackgroundModel(); got != "gpt-4o-mini" {
		t.Errorf("BackgroundModel override = %q", got)
	}
	if got := PremiumModel(); got != "claude-opus-4.1" {
		t.Errorf("PremiumModel override = %q", got)
	}
}

// The pre-existing trap: OPENAI_API_KEY doubles as an Atlas key via
// getAtlasAPIKey, flipping BackgroundModel to DeepSeek even for local-only
// setups. With Copilot configured that fallback must never win.
func TestBackgroundModelCopilotBeatsAtlasFallback(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_test")
	t.Setenv("OPENAI_API_KEY", "ollama")

	if got := BackgroundModel(); got != defaultCopilotBackgroundModel {
		t.Errorf("BackgroundModel = %q, want %q (copilot must outrank atlas fallback)", got, defaultCopilotBackgroundModel)
	}
}

func TestConfiguredWithCopilotOnly(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_test")
	if !Configured() {
		t.Error("Configured() = false with Copilot token set")
	}
	if !CopilotConfigured() {
		t.Error("CopilotConfigured() = false with token set")
	}
}
