// Package setup provides the first-run configuration flow for a self-hosted
// instance: a guided web page (and a companion `mu setup` CLI wizard) that
// creates the admin account and selects an AI provider, so a fresh `mu --serve`
// goes from "boots" to "works" without a treasure hunt through /admin/env.
package setup

import (
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// Needed reports whether the instance still needs first-run setup — i.e. no
// owner account exists yet. Once an owner exists the flow closes and routing
// stops sending people here.
func Needed() bool {
	return !auth.OwnerExists()
}

// Handler serves GET /setup (the form) and POST /setup (apply). It is only open
// while no owner exists; afterwards it redirects to /login so it can't be used
// to mint a second account.
func Handler(w http.ResponseWriter, r *http.Request) {
	if auth.OwnerExists() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Pick up anything `mu setup` wrote after this server started, so an
	// already-configured provider is recognised without a restart.
	settings.Load()

	if r.Method == http.MethodPost {
		applySetup(w, r)
		return
	}

	w.Write([]byte(render("")))
}

func applySetup(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id := strings.TrimSpace(r.FormValue("username"))
	secret := r.FormValue("password")
	provider := r.FormValue("provider")
	key := strings.TrimSpace(r.FormValue("key"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))

	if id == "" {
		w.Write([]byte(render("Choose a username for your admin account.")))
		return
	}
	if msg := auth.ValidateUsername(id); msg != "" {
		w.Write([]byte(render(msg)))
		return
	}
	if len(secret) < 6 {
		w.Write([]byte(render("Password must be at least 6 characters.")))
		return
	}

	if msg := applyProvider(provider, key, baseURL); msg != "" {
		w.Write([]byte(render(msg)))
		return
	}

	// Create the owner account. auth.Create establishes its owner privileges.
	if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: secret, Created: time.Now()}); err != nil {
		w.Write([]byte(render(err.Error())))
		return
	}

	sess, err := auth.Login(id, secret)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	secure := r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: sess.Token, Path: "/", MaxAge: 2592000,
		Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

// applyProvider resolves the chosen AI provider into the settings keys the
// runtime reads. An empty key is accepted when the provider already has
// credentials (from env or a previous `mu setup`), so the web wizard never
// demands a token that is already stored. Returns a user-facing error message,
// or "" on success.
func applyProvider(provider, key, baseURL string) string {
	switch provider {
	case "claude":
		if key != "" {
			settings.Set("ANTHROPIC_API_KEY", key)
		} else if !settings.IsSet("ANTHROPIC_API_KEY") {
			return "Enter your Anthropic API key, or pick another provider."
		}
	case "atlas":
		if key != "" {
			settings.Set("ATLAS_API_KEY", key)
		} else if !settings.IsSet("ATLAS_API_KEY") {
			return "Enter your Atlas Cloud API key, or pick another provider."
		}
	case "ollama":
		if baseURL != "" {
			settings.Set("OPENAI_BASE_URL", baseURL)
		} else if !settings.IsSet("OPENAI_BASE_URL") {
			settings.Set("OPENAI_BASE_URL", "http://localhost:11434/v1")
		}
		if !settings.IsSet("OPENAI_API_KEY") {
			settings.Set("OPENAI_API_KEY", "ollama")
		}
	case "copilot":
		if key != "" {
			settings.Set("COPILOT_GITHUB_TOKEN", key)
		} else if !settings.IsSet("COPILOT_GITHUB_TOKEN") {
			return "Enter your GitHub OAuth token for Copilot (run `mu setup` in a terminal for guided device-flow sign-in), or pick another provider."
		}
	default:
		return "Pick an AI provider."
	}
	return ""
}

// configuredProvider reports which AI provider already has credentials, if
// any, so the form can say so instead of asking for a key again.
func configuredProvider() (value, label string) {
	switch {
	case settings.IsSet("COPILOT_GITHUB_TOKEN"):
		return "copilot", "GitHub Copilot"
	case settings.IsSet("ANTHROPIC_API_KEY"):
		return "claude", "Anthropic Claude"
	case settings.IsSet("ATLAS_API_KEY"):
		return "atlas", "Atlas Cloud / DeepSeek"
	case settings.IsSet("OPENAI_BASE_URL"):
		return "ollama", "Ollama / OpenAI-compatible"
	}
	return "", ""
}

func render(errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<p style="color:#c00;margin:0 0 12px">` + html.EscapeString(errMsg) + `</p>`
	}
	sel, selLabel := configuredProvider()
	def := sel
	if def == "" {
		def = "claude"
	}
	check := func(p string) string {
		if p == def {
			return " checked"
		}
		return ""
	}
	providerNote := ""
	if sel != "" {
		providerNote = `<p style="color:#27ae60;font-size:13px;margin:0 0 8px">` + selLabel + ` is already configured (e.g. via <code>mu setup</code>) — leave the key blank to keep it.</p>
    `
	}
	body := `<div class="card" style="max-width:520px;margin:0 auto">
  <h1 style="margin:0 0 6px">Welcome to Mu</h1>
  <p style="color:#666;margin:0 0 20px">Two quick things and you're running your own instance.</p>
  ` + errHTML + `
  <form method="POST" action="/setup">
    <h3 style="margin:0 0 8px;font-size:1em">1 · Admin account</h3>
    <input name="username" placeholder="username" autocomplete="username" required
      style="width:100%;padding:10px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:15px">
    <input name="password" type="password" placeholder="password (min 6 chars)" autocomplete="new-password" required
      style="width:100%;padding:10px;margin:0 0 20px;border:1px solid #ddd;border-radius:6px;font-size:15px">

    <h3 style="margin:0 0 8px;font-size:1em">2 · AI provider</h3>
    ` + providerNote + `<label style="display:block;margin:0 0 6px"><input type="radio" name="provider" value="claude"` + check("claude") + `> Anthropic Claude</label>
    <label style="display:block;margin:0 0 6px"><input type="radio" name="provider" value="atlas"` + check("atlas") + `> Atlas Cloud / DeepSeek</label>
    <label style="display:block;margin:0 0 6px"><input type="radio" name="provider" value="copilot"` + check("copilot") + `> GitHub Copilot (subscription)</label>
    <label style="display:block;margin:0 0 12px"><input type="radio" name="provider" value="ollama"` + check("ollama") + `> Ollama / OpenAI-compatible (local)</label>
    <input name="key" placeholder="API key (Claude/Atlas) or GitHub OAuth token (Copilot)"
      style="width:100%;padding:10px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:15px">
    <input name="base_url" placeholder="Ollama base URL (default http://localhost:11434/v1)"
      style="width:100%;padding:10px;margin:0 0 20px;border:1px solid #ddd;border-radius:6px;font-size:15px">

    <button type="submit" style="width:100%;padding:12px;background:#111;color:#fff;border:none;border-radius:6px;font-size:15px;cursor:pointer">Start Mu</button>
  </form>
  <p style="color:#888;font-size:13px;margin:16px 0 0">You can change any of this later at <code>/admin/env</code>. Prefer the terminal? Run <code>mu setup</code>.</p>
</div>`
	return app.RenderHTML("Setup", "Set up your Mu instance", body)
}
