package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOwnerCanAlwaysWrite(t *testing.T) {
	mutex.Lock()
	oldAccounts := accounts
	accounts = map[string]*Account{
		"owner": {ID: "owner", Admin: true, Approved: true, Created: time.Now()},
	}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts = oldAccounts
		mutex.Unlock()
	})

	if !CanWrite("owner") {
		t.Fatal("owner should be allowed to write")
	}
	if CanWrite("missing") {
		t.Fatal("missing account should not be allowed to write")
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
