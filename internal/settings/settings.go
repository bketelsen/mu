// Package settings provides persistent configuration that can be edited
// from the admin UI. Values are stored in settings.json. Environment
// variables take precedence when set.
package settings

import (
	"os"
	"sync"

	"mu/internal/data"
)

var (
	mu     sync.RWMutex
	values = map[string]string{}
)

func Load() {
	loaded := map[string]string{}
	if err := data.LoadJSON("settings.json", &loaded); err != nil {
		return
	}

	mu.Lock()
	values = loaded
	mu.Unlock()
}

// Get returns the value for a key. Environment variable takes precedence
// over the stored value.
func Get(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	mu.RLock()
	defer mu.RUnlock()
	return values[key]
}

// Set stores a value persistently. Does not affect the environment.
//
// The write is read-modify-write: the file is re-read first so keys written
// by another process since our Load (e.g. `mu setup` beside a running server)
// survive this save instead of being clobbered by a stale in-memory map. If
// the file is missing or unreadable the in-memory map is the base, matching
// Load's behavior.
func Set(key, value string) {
	mu.Lock()
	defer mu.Unlock()
	loaded := map[string]string{}
	if err := data.LoadJSON("settings.json", &loaded); err == nil {
		values = loaded
	}
	if value == "" {
		delete(values, key)
	} else {
		values[key] = value
	}
	data.SaveJSON("settings.json", values)
}

// IsSet returns true if the key has a value (from env or stored).
func IsSet(key string) bool {
	return Get(key) != ""
}

// Source returns where the value comes from: "env", "saved", or "".
func Source(key string) string {
	if os.Getenv(key) != "" {
		return "env"
	}
	mu.RLock()
	defer mu.RUnlock()
	if values[key] != "" {
		return "saved"
	}
	return ""
}

// All returns all stored settings (not env vars).
func All() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	result := make(map[string]string, len(values))
	for k, v := range values {
		result[k] = v
	}
	return result
}
