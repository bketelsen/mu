package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"mu/internal/ai/copilot"
	"mu/internal/settings"
)

// runSetup is the `mu setup` wizard: a headless-friendly companion to the web
// /setup page. It writes the AI provider into the shared settings store so the
// next `mu --serve` comes up configured. Admin creation happens on first web
// signup (or via the ADMIN env var), which it explains at the end.
func runSetup(_ []string) int {
	settings.Load() // don't clobber existing settings

	in := bufio.NewReader(os.Stdin)
	fmt.Println("Mu setup — configure an AI provider.")
	fmt.Println()
	fmt.Println("  1) Anthropic Claude")
	fmt.Println("  2) Atlas Cloud / DeepSeek")
	fmt.Println("  3) Ollama / OpenAI-compatible (local)")
	fmt.Println("  4) GitHub Copilot (use your Copilot subscription)")
	choice := prompt(in, "Provider [1-4]: ")

	switch choice {
	case "1":
		key := prompt(in, "Anthropic API key: ")
		if key == "" {
			return setupErr("no key entered")
		}
		settings.Set("ANTHROPIC_API_KEY", key)
	case "2":
		key := prompt(in, "Atlas Cloud API key: ")
		if key == "" {
			return setupErr("no key entered")
		}
		settings.Set("ATLAS_API_KEY", key)
	case "3":
		url := prompt(in, "Base URL [http://localhost:11434/v1]: ")
		if url == "" {
			url = "http://localhost:11434/v1"
		}
		settings.Set("OPENAI_BASE_URL", url)
		settings.Set("OPENAI_API_KEY", "ollama")
	case "4":
		if err := setupCopilot(in); err != nil {
			return setupErr(err.Error())
		}
	default:
		return setupErr("pick 1, 2, 3 or 4")
	}

	fmt.Println()
	fmt.Println("✓ AI provider saved.")
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mu --serve            # start the server")
	fmt.Println("  open http://localhost:8080")
	fmt.Println()
	fmt.Println("The first account you create becomes admin")
	fmt.Println("(or set ADMIN=you@example.com before starting the server).")
	return 0
}

// setupCopilot signs the user in to GitHub Copilot via the OAuth device flow
// (a token can also be pasted directly), then lists the models the
// subscription offers so chat and background models can be chosen.
func setupCopilot(in *bufio.Reader) error {
	fmt.Println()
	fmt.Println("GitHub Copilot sign-in. Press Enter to start the device flow,")
	fmt.Println("or paste an existing GitHub OAuth token (gho_...).")
	token := prompt(in, "Token [Enter = device flow]: ")

	if token == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		dc, err := copilot.StartDeviceFlow(ctx)
		if err != nil {
			return err
		}
		fmt.Println()
		fmt.Printf("  1. Open  %s\n", dc.VerificationURI)
		fmt.Printf("  2. Enter code  %s\n", dc.UserCode)
		fmt.Println()
		fmt.Println("Waiting for approval...")
		token, err = copilot.WaitForDeviceToken(ctx, dc)
		if err != nil {
			return err
		}
		fmt.Println("✓ Signed in.")
	}

	// Verify the token and show what the subscription offers.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	models, err := copilot.ListModels(ctx, token)
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}
	settings.Set("COPILOT_GITHUB_TOKEN", token)

	fmt.Println()
	fmt.Println("Models available on your subscription:")
	for _, m := range models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		fmt.Printf("  %-28s %s\n", m.ID, name)
	}
	fmt.Println()
	fmt.Println("Chat model: used for interactive queries (claude-* models consume")
	fmt.Println("premium requests). Background model: used for high-volume summaries")
	fmt.Println("and planning — pick an included 0x model like gpt-4.1.")
	if chat := prompt(in, "Chat model [claude-sonnet-4.5]: "); chat != "" {
		settings.Set("COPILOT_CHAT_MODEL", chat)
	}
	if bg := prompt(in, "Background model [gpt-4.1]: "); bg != "" {
		settings.Set("COPILOT_BACKGROUND_MODEL", bg)
	}
	return nil
}

func prompt(in *bufio.Reader, label string) string {
	fmt.Print(label)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

func setupErr(msg string) int {
	fmt.Fprintln(os.Stderr, "setup:", msg)
	return 2
}
