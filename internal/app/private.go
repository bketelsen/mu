package app

import (
	"net/http"
	"strings"

	"mu/internal/auth"
)

var publicPrivateAssets = map[string]bool{
	"/mu.css":               true,
	"/mu.js":                true,
	"/qrcode.js":            true,
	"/sdk.css":              true,
	"/sdk.js":               true,
	"/manifest.webmanifest": true,
	"/favicon.ico":          true,
	"/account.png":          true,
	"/agent.svg":            true,
	"/apps.svg":             true,
	"/audio.png":            true,
	"/chat.png":             true,
	"/github.svg":           true,
	"/home.png":             true,
	"/icon-192.png":         true,
	"/icon-512.png":         true,
	"/images.svg":           true,
	"/logout.png":           true,
	"/mail.png":             true,
	"/membership.png":       true,
	"/mu.png":               true,
	"/news.png":             true,
	"/post.png":             true,
	"/saved.svg":            true,
	"/search.svg":           true,
	"/video.png":            true,
	"/weather.png":          true,
	"/weather.svg":          true,
}

func Private(next http.Handler, setupNeeded func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		needsSetup := setupNeeded()
		if publicPrivatePath(r.URL.Path, needsSetup) {
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
		if needsSetup {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		RedirectToLogin(w, r)
	})
}

func publicPrivatePath(path string, setupNeeded bool) bool {
	if publicPrivateAssets[path] {
		return true
	}
	switch path {
	case "/login", "/passkey/login/begin", "/passkey/login/finish",
		"/oauth2/google", "/oauth2/callback", "/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource", "/oauth/register", "/oauth/authorize",
		"/oauth/token", "/whatsapp/webhook", "/status", "/version":
		return true
	case "/setup":
		return setupNeeded
	default:
		return false
	}
}
