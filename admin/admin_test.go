package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func ownerSessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
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
	sess, err := auth.CreateSession(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func TestAdminDashboardContainsOnlyOperationalLinks(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(ownerSessionCookie(t))
	rr := httptest.NewRecorder()
	AdminHandler(rr, req)
	body := rr.Body.String()
	for _, want := range []string{"/admin/env", "/admin/server", "/admin/log"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %s", want)
		}
	}
	for _, forbidden := range []string{"Users", "Invites", "Moderation", "Blocklist", "/admin/users", "/admin/invite"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("dashboard contains %q", forbidden)
		}
	}
}

func TestBlocklistRowsEscapeValues(t *testing.T) {
	tests := []struct {
		name string
		html string
	}{
		{name: "email", html: blocklistEmailRow(`bad"><script>alert(1)</script>@example.com`)},
		{name: "ip", html: blocklistIPRow(`127.0.0.1"><script>alert(1)</script>`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.html, `<script>`) || strings.Contains(tt.html, `"><script>`) {
				t.Fatalf("blocklist row contains unescaped value: %s", tt.html)
			}
			if !strings.Contains(tt.html, `&lt;script&gt;alert(1)&lt;/script&gt;`) {
				t.Fatalf("blocklist row does not include escaped script text: %s", tt.html)
			}
			if !strings.Contains(tt.html, `&#34;&gt;`) {
				t.Fatalf("blocklist row does not escape attribute-breaking quote: %s", tt.html)
			}
		})
	}
}

func TestUserActionsDoNotRenderDeleteForm(t *testing.T) {
	actions := userActions(&auth.Account{ID: "member"}, "owner", "all")
	if strings.Contains(actions, `name="action" value="delete"`) {
		t.Fatalf("rendered obsolete delete action: %s", actions)
	}
}
