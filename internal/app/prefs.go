package app

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// UserPrefs stores per-user content preferences (saves, dismissals, blocks)
type UserPrefs struct {
	Saved     map[string]time.Time `json:"saved"`     // "type:id" → saved time
	Dismissed map[string]time.Time `json:"dismissed"` // "type:id" → dismissed time
}

var (
	prefsMu sync.RWMutex
	prefs   = map[string]*UserPrefs{} // userID → prefs
)

func init() {
	b, _ := data.LoadFile("prefs.json")
	if len(b) > 0 {
		json.Unmarshal(b, &prefs)
	}
}

func savePrefs() error {
	return data.SaveJSON("prefs.json", prefs)
}

func getUserPrefs(userID string) *UserPrefs {
	p, ok := prefs[userID]
	if !ok {
		p = &UserPrefs{
			Saved:     map[string]time.Time{},
			Dismissed: map[string]time.Time{},
		}
		prefs[userID] = p
	}
	return p
}

// SaveItem bookmarks a content item for the user
func SaveItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	p.Saved[contentType+":"+contentID] = time.Now()
	savePrefs()
}

// UnsaveItem removes a bookmark
func UnsaveItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	delete(p.Saved, contentType+":"+contentID)
	savePrefs()
}

// IsSaved checks if the user has saved this item
func IsSaved(userID, contentType, contentID string) bool {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return false
	}
	_, saved := p.Saved[contentType+":"+contentID]
	return saved
}

// DismissItem hides a content item from the user's view
func DismissItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	p.Dismissed[contentType+":"+contentID] = time.Now()
	savePrefs()
}

// IsDismissed checks if the user has dismissed this item
func IsDismissed(userID, contentType, contentID string) bool {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return false
	}
	_, dismissed := p.Dismissed[contentType+":"+contentID]
	return dismissed
}

// GetSavedItems returns all saved item keys for a user
func GetSavedItems(userID string) map[string]time.Time {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return nil
	}
	return p.Saved
}

// ClearUserPrefs removes all preferences for a deleted user.
func ClearUserPrefs(userID string) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	delete(prefs, userID)
	return savePrefs()
}

// RemoveContentTypePrefs removes saved and dismissed preferences for a content type.
func RemoveContentTypePrefs(contentType string) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	prefix := contentType + ":"
	for _, p := range prefs {
		for key := range p.Saved {
			if strings.HasPrefix(key, prefix) {
				delete(p.Saved, key)
			}
		}
		for key := range p.Dismissed {
			if strings.HasPrefix(key, prefix) {
				delete(p.Dismissed, key)
			}
		}
	}
	return savePrefs()
}
