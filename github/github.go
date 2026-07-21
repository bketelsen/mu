package github

import (
	"net/http"
	"time"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/settings"
)

var defaultServer = NewServer(NewClient(
	&http.Client{Timeout: 15 * time.Second},
	"https://api.github.com",
	func() string { return settings.Get("GITHUB_TOKEN") },
))

// DefaultServer returns the production GitHub service handler.
func DefaultServer() *Server { return defaultServer }

// Load registers the production GitHub service.
func Load() {
	if err := service.Register("github", defaultServer); err != nil {
		app.Log("github", "service register failed: %v", err)
	}
}
