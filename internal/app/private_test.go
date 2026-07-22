package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestPrivatePublicAllowlist(t *testing.T) {
	public := []string{
		"/setup", "/login", "/passkey/login/begin", "/passkey/login/finish",
		"/oauth2/google", "/oauth2/callback", "/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource", "/oauth/register", "/oauth/authorize",
		"/oauth/token", "/whatsapp/webhook", "/status", "/version", "/mu.css",
	}
	for _, path := range public {
		t.Run(path, func(t *testing.T) {
			hit := false
			h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return path == "/setup" })
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			if !hit {
				t.Fatalf("public path %s was blocked", path)
			}
		})
	}
}

func TestPrivateDeniesApplicationRoutesByDefault(t *testing.T) {
	denied := []string{"/home", "/news", "/docs", "/mcp", "/a2a", "/api", "/agent", "/images/file/private.png", "/apps/private.js", "/session", "/verify", "/wallet/stripe/webhook"}
	for _, path := range denied {
		t.Run(path, func(t *testing.T) {
			hit := false
			h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return false })
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if path == "/mcp" || path == "/a2a" || path == "/api" {
				req.Header.Set("Accept", "application/json")
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if hit {
				t.Fatalf("private path %s reached handler", path)
			}
			if WantsJSON(req) && rr.Code != http.StatusUnauthorized {
				t.Fatalf("%s = %d, want 401", path, rr.Code)
			}
			if !WantsJSON(req) && rr.Code != http.StatusSeeOther {
				t.Fatalf("%s = %d, want redirect", path, rr.Code)
			}
		})
	}
}

func TestPrivateRedirectsFreshBrowserRequestsToSetup(t *testing.T) {
	h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("fresh browser request reached private handler")
	}), func() bool { return true })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if location := rr.Header().Get("Location"); location != "/setup" {
		t.Fatalf("Location = %q, want /setup", location)
	}
}

func TestPrivateRedirectsConfiguredBrowserRequestsToLogin(t *testing.T) {
	h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("configured browser request reached private handler")
	}), func() bool { return false })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if location := rr.Header().Get("Location"); location != "/login?redirect=%2F" {
		t.Fatalf("Location = %q, want login redirect", location)
	}
}

func TestPrivateDeniesFreshMachineRequests(t *testing.T) {
	h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("fresh machine request reached private handler")
	}), func() bool { return true })
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestPrivateAllowsOwner(t *testing.T) {
	owner, err := auth.Owner()
	if errors.Is(err, auth.ErrNoOwner) {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}); err != nil {
			t.Fatal(err)
		}
		owner, err = auth.Owner()
	}
	if err != nil {
		t.Fatal(err)
	}
	sess, err := auth.Login(owner.ID, "owner-pass")
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	h := Private(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }), func() bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !hit {
		t.Fatal("owner request was blocked")
	}
}

func TestPublicStatusContainsNoOperationalDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	StatusHandler(rr, httptest.NewRequest(http.MethodGet, "/status", nil))
	if strings.TrimSpace(rr.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("status body = %s", rr.Body.String())
	}
}
