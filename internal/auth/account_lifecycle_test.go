package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBanAccountRejectsAdminsAndTogglesBannedState(t *testing.T) {
	mutex.Lock()
	oldAccounts := accounts
	accounts = map[string]*Account{
		"admin": {ID: "admin", Admin: true},
		"user":  {ID: "user"},
	}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts = oldAccounts
		mutex.Unlock()
	})

	if err := BanAccount("admin"); err == nil {
		t.Fatal("BanAccount accepted an admin account")
	}
	if IsBanned("admin") {
		t.Fatal("admin account was marked banned")
	}

	if err := BanAccount("user"); err != nil {
		t.Fatalf("BanAccount(user) returned error: %v", err)
	}
	if !IsBanned("user") {
		t.Fatal("user account was not marked banned")
	}

	if err := UnbanAccount("user"); err != nil {
		t.Fatalf("UnbanAccount(user) returned error: %v", err)
	}
	if IsBanned("user") {
		t.Fatal("user account remained banned after UnbanAccount")
	}
}

func TestGetSessionInvalidatesDeletedCookieSession(t *testing.T) {
	mutex.Lock()
	oldAccounts := accounts
	oldSessions := sessions
	oldTokens := tokens
	accounts = map[string]*Account{}
	sessions = map[string]*Session{
		"11111111-1111-1111-1111-111111111111": {
			ID:      "11111111-1111-1111-1111-111111111111",
			Type:    "account",
			Token:   "MTExMTExMTEtMTExMS0xMTExLTExMTEtMTExMTExMTExMTEx",
			Account: "deleted-user",
			Created: time.Now(),
		},
	}
	tokens = map[string]*Token{}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts = oldAccounts
		sessions = oldSessions
		tokens = oldTokens
		mutex.Unlock()
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "MTExMTExMTEtMTExMS0xMTExLTExMTEtMTExMTExMTExMTEx"})

	if _, err := GetSession(req); err == nil {
		t.Fatal("GetSession succeeded for a deleted account")
	}
	mutex.Lock()
	_, stillPresent := sessions["11111111-1111-1111-1111-111111111111"]
	mutex.Unlock()
	if stillPresent {
		t.Fatal("stale session was not invalidated")
	}
}
