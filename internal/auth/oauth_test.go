package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func resetOAuthTestState(t *testing.T) {
	t.Helper()

	oauthMu.Lock()
	oldClients := oauthClients
	oldCodes := oauthCodes
	oauthClients = map[string]*OAuthClient{}
	oauthCodes = map[string]*OAuthCode{}
	oauthMu.Unlock()

	mutex.Lock()
	oldAccounts := accounts
	oldSessions := sessions
	accounts = map[string]*Account{}
	sessions = map[string]*Session{}
	mutex.Unlock()

	t.Cleanup(func() {
		oauthMu.Lock()
		oauthClients = oldClients
		oauthCodes = oldCodes
		oauthMu.Unlock()

		mutex.Lock()
		accounts = oldAccounts
		sessions = oldSessions
		mutex.Unlock()
	})
}

func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func TestExchangeAuthorizationCodeRequiresMatchingPKCE(t *testing.T) {
	resetOAuthTestState(t)

	accountID := "oauth-user"
	mutex.Lock()
	accounts[accountID] = &Account{ID: accountID, Name: "OAuth User", Created: time.Now()}
	mutex.Unlock()

	verifier := "correct horse battery staple verifier"
	code := CreateAuthorizationCode("client-1", accountID, "https://app.example/callback", s256Challenge(verifier), "S256")

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "wrong verifier"); err == nil {
		t.Fatal("expected invalid code_verifier error")
	}

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", verifier); err == nil {
		t.Fatal("expected authorization code to be single-use after a failed exchange")
	}
}

func TestExchangeAuthorizationCodeCreatesSessionForPlainPKCE(t *testing.T) {
	resetOAuthTestState(t)

	accountID := "oauth-user"
	mutex.Lock()
	accounts[accountID] = &Account{ID: accountID, Name: "OAuth User", Created: time.Now()}
	mutex.Unlock()

	code := CreateAuthorizationCode("client-1", accountID, "https://app.example/callback", "plain-verifier", "plain")
	token, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "plain-verifier")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "plain-verifier"); err == nil {
		t.Fatal("expected authorization code to be single-use")
	}
}

func TestOAuthAuthorizeUsesAuthenticatedOwnerID(t *testing.T) {
	oldLogin, oldCreate := loginForOAuth, createOAuthCode
	oauthMu.Lock()
	oldClients := oauthClients
	oauthClients = map[string]*OAuthClient{"client": {ClientID: "client", RedirectURIs: []string{"https://client.example/callback"}}}
	oauthMu.Unlock()
	loginForOAuth = func(string, string) (*Session, error) { return &Session{Account: "canonical-owner"}, nil }
	gotAccount := ""
	createOAuthCode = func(clientID, accountID, redirectURI, challenge, method string) string {
		gotAccount = accountID
		return "code"
	}
	t.Cleanup(func() {
		loginForOAuth, createOAuthCode = oldLogin, oldCreate
		oauthMu.Lock()
		oauthClients = oldClients
		oauthMu.Unlock()
	})

	form := url.Values{"client_id": {"client"}, "redirect_uri": {"https://client.example/callback"}, "username": {"submitted-alias"}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	OAuthAuthorizePostHandler(rr, req)
	if gotAccount != "canonical-owner" {
		t.Fatalf("authorization account = %q", gotAccount)
	}
}

func TestOAuthAuthorizeRejectsInvalidRedirectBeforeLogin(t *testing.T) {
	oldLogin := loginForOAuth
	oauthMu.Lock()
	oldClients := oauthClients
	oauthClients = map[string]*OAuthClient{"client": {ClientID: "client", RedirectURIs: []string{"https://client.example/callback"}}}
	oauthMu.Unlock()
	called := false
	loginForOAuth = func(string, string) (*Session, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() {
		loginForOAuth = oldLogin
		oauthMu.Lock()
		oauthClients = oldClients
		oauthMu.Unlock()
	})

	form := url.Values{"client_id": {"client"}, "redirect_uri": {"https://attacker.example/callback"}, "username": {"owner"}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	OAuthAuthorizePostHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("Login was called for an invalid redirect URI")
	}
}
