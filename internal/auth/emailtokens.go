// Email verification token storage. Sits in the auth package so handlers
// can call into a single place without import cycles.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"mu/internal/data"
)

// ============================================================
// Email verification tokens
// ============================================================

type emailToken struct {
	AccountID string
	Email     string
	ExpiresAt time.Time
}

var (
	emailTokenMu sync.Mutex
	emailTokens  = map[string]*emailToken{}
)

// CreateEmailVerificationToken issues a one-time verification token for
// the given account/email pair. Any prior token for the same account is
// invalidated so re-requesting cancels the old one.
func CreateEmailVerificationToken(accountID, email string) (string, error) {
	mutex.Lock()
	_, exists := accounts[accountID]
	mutex.Unlock()
	if !exists {
		return "", errors.New("account not found")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(tokenBytes)

	emailTokenMu.Lock()
	// Invalidate any previous tokens for this account.
	for k, v := range emailTokens {
		if v.AccountID == accountID {
			delete(emailTokens, k)
		}
	}
	emailTokens[tok] = &emailToken{
		AccountID: accountID,
		Email:     email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	emailTokenMu.Unlock()
	return tok, nil
}

// ConsumeEmailVerificationToken verifies a token and, on success, marks
// the account's email as verified. The token is single-use.
func ConsumeEmailVerificationToken(token string) (*Account, error) {
	emailTokenMu.Lock()
	t, ok := emailTokens[token]
	if ok {
		delete(emailTokens, token)
	}
	emailTokenMu.Unlock()
	if !ok {
		return nil, errors.New("invalid or expired verification link")
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, errors.New("verification link has expired — please request a new one")
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, exists := accounts[t.AccountID]
	if !exists {
		return nil, errors.New("account no longer exists")
	}
	acc.Email = t.Email
	acc.EmailVerified = true
	acc.EmailVerifiedAt = time.Now()
	if err := saveAccountsUnlocked(); err != nil {
		return nil, err
	}
	return acc, nil
}

// SetAccountEmail stores an unverified email on the account (called when
// the user submits the verification form so we remember the pending
// address even before they click the link).
func SetAccountEmail(accountID, email string) error {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return errors.New("account not found")
	}
	acc.Email = email
	acc.EmailVerified = false
	acc.EmailVerifiedAt = time.Time{}
	return saveAccountsUnlocked()
}

// saveAccountsUnlocked persists the accounts map. Caller must hold mutex.
func saveAccountsUnlocked() error {
	return data.SaveJSON("accounts.json", accounts)
}
