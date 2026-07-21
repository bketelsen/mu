package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func ownerIDForTest(t *testing.T) string {
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
	return owner.ID
}

func TestClassifyMessage(t *testing.T) {
	ownerID := ownerIDForTest(t)
	tests := []struct {
		name   string
		direct bool
		linked string
		want   messageAccess
	}{
		{"shared owner", false, ownerID, accessIgnore},
		{"unlinked DM", true, "", accessNeedsLink},
		{"stale legacy DM", true, "legacy", accessNeedsLink},
		{"owner DM", true, ownerID, accessOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMessage(tt.direct, tt.linked); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkAccountRejectsNonOwner(t *testing.T) {
	ownerIDForTest(t)
	if err := linkAccount("whatsapp-test", "legacy"); err == nil {
		t.Fatal("linkAccount accepted a non-owner account")
	}
}

func TestWebhookRejectsPOSTWithoutAppSecret(t *testing.T) {
	t.Setenv("WHATSAPP_APP_SECRET", "")
	req := httptest.NewRequest(http.MethodPost, "/whatsapp/webhook", strings.NewReader(`{"entry":[]}`))
	res := httptest.NewRecorder()

	Handler(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"entry":[]}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      body,
			signature: validSignature,
			secret:    secret,
			want:      true,
		},
		{
			name:      "tampered body",
			body:      []byte(`{"entry":[{"changed":true}]}`),
			signature: validSignature,
			secret:    secret,
			want:      false,
		},
		{
			name:      "wrong secret",
			body:      body,
			signature: validSignature,
			secret:    "different-secret",
			want:      false,
		},
		{
			name:      "missing sha256 prefix",
			body:      body,
			signature: hex.EncodeToString(mac.Sum(nil)),
			secret:    secret,
			want:      false,
		},
		{
			name:      "invalid hex",
			body:      body,
			signature: "sha256=not-hex",
			secret:    secret,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verifySignature(tt.body, tt.signature, tt.secret); got != tt.want {
				t.Fatalf("verifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}
