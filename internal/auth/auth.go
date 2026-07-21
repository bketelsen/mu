package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/data"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var mutex sync.Mutex
var accounts = map[string]*Account{}
var sessions = map[string]*Session{}
var tokens = map[string]*Token{} // PAT tokens: tokenID -> Token

var (
	ErrNoOwner          = errors.New("owner is not configured")
	ErrOwnerExists      = errors.New("Mu already has an owner")
	ErrMultipleAccounts = errors.New("legacy accounts require single-owner migration")
)

type Account struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Secret          string    `json:"secret"`
	Created         time.Time `json:"created"`
	Admin           bool      `json:"admin"`
	Language        string    `json:"language"`
	Widgets         []string  `json:"widgets,omitempty"`         // App IDs to show as home widgets
	HomeCards       []string  `json:"home_cards,omitempty"`      // Card IDs the user has chosen to show (empty = all defaults)
	HomeCardsSeen   []string  `json:"home_cards_seen,omitempty"` // Card IDs the customise panel has offered this user; anything newer defaults to visible
	Approved        bool      `json:"approved,omitempty"`        // Legacy JSON compatibility only.
	Email           string    `json:"email,omitempty"`
	EmailVerified   bool      `json:"email_verified,omitempty"`
	EmailVerifiedAt time.Time `json:"email_verified_at,omitempty"`
	Banned          bool      `json:"banned,omitempty"` // Legacy JSON compatibility only.
}

// preHomeCardsSeen is the set of home cards that existed before per-user
// "seen" tracking was added. Accounts saved earlier have an empty
// HomeCardsSeen; we treat them as having been offered exactly these, so any
// card introduced afterwards (images, and future cards) defaults to visible
// instead of being silently hidden by the HomeCards allowlist.
var preHomeCardsSeen = map[string]bool{
	"blog": true, "news": true, "markets": true,
	"social": true, "video": true, "mail": true, "web": true,
}

// ShowHomeCard reports whether a default home card (one defined in cards.json)
// should render for this account. A card the user explicitly selected shows; a
// card they deselected (present in their seen set but not their allowlist)
// hides; a card newer than anything they've been offered defaults to visible.
func (a *Account) ShowHomeCard(id string) bool {
	if len(a.HomeCards) == 0 {
		return true // no customization yet → all defaults show
	}
	for _, c := range a.HomeCards {
		if c == id {
			return true
		}
	}
	seen := a.HomeCardsSeen
	if len(seen) == 0 {
		return !preHomeCardsSeen[id] // legacy account → only genuinely new cards
	}
	for _, c := range seen {
		if c == id {
			return false // offered before and not selected → deliberately hidden
		}
	}
	return true // never offered → new card, default on
}

// HomeCardActive reports whether an opt-in card (mail, web) is explicitly
// enabled. Unlike default cards these are off unless the user turns them on.
func (a *Account) HomeCardActive(id string) bool {
	for _, c := range a.HomeCards {
		if c == id {
			return true
		}
	}
	return false
}

type Session struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Token   string    `json:"token"`
	Account string    `json:"account"`
	Created time.Time `json:"created"`
}

// Token represents a Personal Access Token (PAT) for API automation
type Token struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`    // User-friendly name for the token
	Token       string    `json:"token"`   // The actual token value (hashed in storage)
	Account     string    `json:"account"` // Account ID this token belongs to
	Created     time.Time `json:"created"`
	LastUsed    time.Time `json:"last_used"`
	ExpiresAt   time.Time `json:"expires_at"`  // Optional expiration
	Permissions []string  `json:"permissions"` // e.g., "read", "write", "admin"
}

func init() {
	b, _ := data.LoadFile("accounts.json")
	json.Unmarshal(b, &accounts)
	b, _ = data.LoadFile("sessions.json")
	json.Unmarshal(b, &sessions)
	b, _ = data.LoadFile("tokens.json")
	json.Unmarshal(b, &tokens)
}

func ownerLocked() (*Account, error) {
	if len(accounts) == 0 {
		return nil, ErrNoOwner
	}
	if len(accounts) != 1 {
		return nil, ErrMultipleAccounts
	}
	for _, acc := range accounts {
		return acc, nil
	}
	return nil, ErrNoOwner
}

func Owner() (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()
	return ownerLocked()
}

// RunForOwner executes fn only when targetID still identifies the sole owner.
// Deferred work must not act on a deleted legacy account.
func RunForOwner(targetID string, fn func(owner *Account)) {
	owner, err := Owner()
	if err != nil || owner.ID != targetID {
		return
	}
	fn(owner)
}

func OwnerExists() bool {
	_, err := Owner()
	return err == nil
}

func IsOwner(accountID string) bool {
	mutex.Lock()
	defer mutex.Unlock()
	owner, err := ownerLocked()
	return err == nil && owner.ID == accountID
}

// CanWrite is the explicit identity check used by charged write middleware.
func CanWrite(accountID string) bool {
	return IsOwner(accountID)
}

func Create(acc *Account) error {
	mutex.Lock()
	defer mutex.Unlock()

	if len(accounts) != 0 {
		return ErrOwnerExists
	}

	// hash the secret
	hash, err := bcrypt.GenerateFromPassword([]byte(acc.Secret), 10)
	if err != nil {
		return err
	}

	acc.Secret = string(hash)
	acc.Admin = true
	acc.Approved = true
	acc.Banned = false
	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	return nil
}

func Delete(acc *Account) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[acc.ID]; !ok {
		return errors.New("account does not exist")
	}

	delete(accounts, acc.ID)
	data.SaveJSON("accounts.json", accounts)

	return nil
}

func GetAccount(id string) (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}

	return acc, nil
}

func UpdateAccount(acc *Account) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[acc.ID]; !ok {
		return errors.New("account does not exist")
	}

	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	return nil
}

// GetAccountByEmail finds an account by email (case-insensitive). Used for
// OAuth sign-in, where the email is the stable identity across providers.
func GetAccountByEmail(email string) (*Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("email required")
	}
	mutex.Lock()
	defer mutex.Unlock()
	for _, acc := range accounts {
		if strings.ToLower(acc.Email) == email {
			return acc, nil
		}
	}
	return nil, errors.New("account not found")
}

// GetAccountByName finds an account by username (case-insensitive)
func GetAccountByName(name string) (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()

	nameLower := strings.ToLower(name)
	for _, acc := range accounts {
		if strings.ToLower(acc.Name) == nameLower || strings.ToLower(acc.ID) == nameLower {
			return acc, nil
		}
	}
	return nil, errors.New("account not found")
}

func Login(id, secret string) (*Session, error) {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}

	err := bcrypt.CompareHashAndPassword([]byte(acc.Secret), []byte(secret))
	if err != nil {
		return nil, errors.New("invalid account secret")
	}
	owner, err := ownerLocked()
	if err != nil {
		return nil, err
	}
	if owner.ID != acc.ID {
		return nil, errors.New("account is not owner")
	}

	guid := uuid.New().String()

	sess := &Session{
		ID:      guid,
		Type:    "account",
		Token:   base64.StdEncoding.EncodeToString([]byte(guid)),
		Account: acc.ID,
		Created: time.Now(),
	}

	// store the session
	sessions[guid] = sess
	data.SaveJSON("sessions.json", sessions)

	return sess, nil
}

// CreateSession creates a new session for the given account ID without password validation.
// Used for passkey authentication where identity is verified via WebAuthn.
func CreateSession(id string) (*Session, error) {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}
	owner, err := ownerLocked()
	if err != nil {
		return nil, err
	}
	if owner.ID != acc.ID {
		return nil, errors.New("account is not owner")
	}

	guid := uuid.New().String()

	sess := &Session{
		ID:      guid,
		Type:    "account",
		Token:   base64.StdEncoding.EncodeToString([]byte(guid)),
		Account: id,
		Created: time.Now(),
	}

	sessions[guid] = sess
	data.SaveJSON("sessions.json", sessions)

	return sess, nil
}

func Logout(tk string) error {
	sess, err := ParseToken(tk)
	if err != nil {
		return err
	}

	mutex.Lock()
	delete(sessions, sess.ID)
	data.SaveJSON("sessions.json", sessions)
	mutex.Unlock()

	return nil
}

func GetSession(r *http.Request) (*Session, error) {
	// Try cookie first
	c, err := r.Cookie("session")
	if err == nil && c != nil {
		sess, err := ParseToken(c.Value)
		if err == nil {
			// Reject sessions belonging to accounts outside the singleton owner.
			mutex.Lock()
			owner, ownerErr := ownerLocked()
			accountExists := ownerErr == nil && owner.ID == sess.Account
			if !accountExists {
				delete(sessions, sess.ID)
			}
			mutex.Unlock()

			if !accountExists {
				return nil, errors.New("account no longer exists")
			}

			return sess, nil
		}
	}

	// Try Authorization header (PAT, session token, or Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Support both "Bearer <token>" and just "<token>"
		token := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		// Try as PAT first
		accountID, err := ValidatePAT(token)
		if err == nil {
			return &Session{
				Type:    "token",
				Account: accountID,
			}, nil
		}

		// Try as a session token returned by the MCP login tool.
		sess, err := ParseToken(token)
		if err == nil {
			mutex.Lock()
			owner, ownerErr := ownerLocked()
			accountExists := ownerErr == nil && owner.ID == sess.Account
			if !accountExists {
				delete(sessions, sess.ID)
			}
			mutex.Unlock()
			if accountExists {
				return sess, nil
			}
		}
	}

	// Try X-Micro-Token header (legacy)
	tokenHeader := r.Header.Get("X-Micro-Token")
	if tokenHeader != "" {
		accountID, err := ValidatePAT(tokenHeader)
		if err == nil {
			// Create a pseudo-session for PAT
			return &Session{
				Type:    "token",
				Account: accountID,
			}, nil
		}
	}

	return nil, errors.New("session not found")
}

// RequireSession returns the session and account, or an error if not authenticated
// This is a convenience function that combines GetSession and GetAccount
func RequireSession(r *http.Request) (*Session, *Account, error) {
	sess, err := GetSession(r)
	if err != nil {
		return nil, nil, errors.New("authentication required")
	}

	acc, err := GetAccount(sess.Account)
	if err != nil {
		return nil, nil, errors.New("account not found")
	}

	return sess, acc, nil
}

// TrySession returns the session and account if authenticated, or nil values if not
// Use this for optional auth checks where you want to show different content for guests vs users
func TrySession(r *http.Request) (*Session, *Account) {
	sess, acc, err := RequireSession(r)
	if err != nil {
		return nil, nil
	}
	return sess, acc
}

// RequireAdmin returns the session and account if the user is an admin, or an error
func RequireAdmin(r *http.Request) (*Session, *Account, error) {
	sess, acc, err := RequireSession(r)
	if err != nil {
		return nil, nil, err
	}

	if !IsOwner(acc.ID) {
		return nil, nil, errors.New("admin access required")
	}

	return sess, acc, nil
}

func ParseToken(tk string) (*Session, error) {
	dec, err := base64.StdEncoding.DecodeString(tk)
	if err != nil {
		return nil, errors.New("invalid session")
	}

	id, err := uuid.Parse(string(dec))
	if err != nil {
		return nil, errors.New("invalid session")
	}

	mutex.Lock()
	sess, ok := sessions[id.String()]
	mutex.Unlock()

	if !ok {
		return nil, errors.New("session not found")
	}

	return sess, nil
}

func GenerateToken() string {
	id := uuid.New().String()
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func ValidateToken(tk string) error {
	if len(tk) == 0 {
		return errors.New("invalid token")
	}

	// Try session token first
	sess, err := ParseToken(tk)
	if err == nil {
		if sess.Type != "account" {
			return errors.New("invalid session")
		}
		return nil
	}

	// Try PAT token
	_, err = ValidatePAT(tk)
	if err == nil {
		return nil
	}

	return errors.New("invalid token")
}

// ============================================
// Personal Access Token (PAT) Management
// ============================================

// CreateToken creates a new Personal Access Token for an account
func CreateToken(accountID, name string, permissions []string, expiresAt time.Time) (*Token, string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Verify account exists
	_, exists := accounts[accountID]
	if !exists {
		return nil, "", errors.New("account does not exist")
	}

	// Generate a cryptographically secure token
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return nil, "", err
	}
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// Hash the token for storage
	hash, err := bcrypt.GenerateFromPassword([]byte(rawToken), 10)
	if err != nil {
		return nil, "", err
	}

	tokenID := uuid.New().String()
	token := &Token{
		ID:          tokenID,
		Name:        name,
		Token:       string(hash),
		Account:     accountID,
		Created:     time.Now(),
		LastUsed:    time.Time{},
		ExpiresAt:   expiresAt,
		Permissions: permissions,
	}

	tokens[tokenID] = token
	data.SaveJSON("tokens.json", tokens)

	// Return the unhashed token only once (user must save it)
	return token, rawToken, nil
}

// ValidatePAT validates a Personal Access Token and returns the associated account ID
func ValidatePAT(rawToken string) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Normalize: strip trailing base64 padding so tokens work with or without '='
	rawToken = strings.TrimRight(rawToken, "=")

	// Check all tokens to find a match (try without padding, then with)
	for _, token := range tokens {
		// Try raw token (no padding) first, then with padding for older tokens
		err := bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(rawToken))
		if err != nil {
			// Retry with padding in case the hash was generated with padded token
			padded := rawToken
			if m := len(padded) % 4; m != 0 {
				padded += strings.Repeat("=", 4-m)
			}
			err = bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(padded))
		}
		if err == nil {
			// Check if expired
			if !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt) {
				return "", errors.New("token expired")
			}
			owner, err := ownerLocked()
			if err != nil {
				return "", err
			}
			if owner.ID != token.Account {
				return "", errors.New("account is not owner")
			}

			// Update last used time
			token.LastUsed = time.Now()
			data.SaveJSON("tokens.json", tokens)

			return token.Account, nil
		}
	}

	return "", errors.New("invalid token")
}

// ListTokens returns all PAT tokens for an account (with hashed values)
func ListTokens(accountID string) []*Token {
	mutex.Lock()
	defer mutex.Unlock()

	var result []*Token
	for _, token := range tokens {
		if token.Account == accountID {
			result = append(result, token)
		}
	}
	return result
}

// DeleteToken removes a PAT token
func DeleteToken(tokenID, accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	token, exists := tokens[tokenID]
	if !exists {
		return errors.New("token does not exist")
	}

	// Verify the token belongs to the account
	if token.Account != accountID {
		return errors.New("unauthorized")
	}

	delete(tokens, tokenID)
	data.SaveJSON("tokens.json", tokens)

	return nil
}

// GetTokenByID retrieves a token by ID (for display purposes)
func GetTokenByID(tokenID string) (*Token, error) {
	mutex.Lock()
	defer mutex.Unlock()

	token, exists := tokens[tokenID]
	if !exists {
		return nil, errors.New("token does not exist")
	}

	return token, nil
}

// HasPermission checks if a token has a specific permission
func (t *Token) HasPermission(perm string) bool {
	for _, p := range t.Permissions {
		if p == perm || p == "all" {
			return true
		}
	}
	return false
}
