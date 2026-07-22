package app

import (
	"reflect"
	"testing"
	"time"

	"mu/internal/data"
)

func TestRemoveContentTypePrefsRemovesOnlyMatchingKeysAndPersists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stamp := time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC)
	oldPrefs := prefs
	prefs = map[string]*UserPrefs{
		"one": {
			Saved: map[string]time.Time{
				"social:post-1":  stamp,
				"news:article-1": stamp,
			},
			Dismissed: map[string]time.Time{
				"social:post-2": stamp,
				"blog:post-1":   stamp,
			},
		},
		"two": {
			Saved:     map[string]time.Time{"social:post-3": stamp},
			Dismissed: map[string]time.Time{"video:clip-1": stamp},
		},
	}
	t.Cleanup(func() { prefs = oldPrefs })

	if err := RemoveContentTypePrefs("social"); err != nil {
		t.Fatalf("RemoveContentTypePrefs: %v", err)
	}
	if err := RemoveContentTypePrefs("social"); err != nil {
		t.Fatalf("second RemoveContentTypePrefs: %v", err)
	}

	if got, want := prefs["one"].Saved, map[string]time.Time{"news:article-1": prefs["one"].Saved["news:article-1"]}; !reflect.DeepEqual(got, want) {
		t.Errorf("first user's saved preferences = %v, want %v", got, want)
	}
	if got, want := prefs["one"].Dismissed, map[string]time.Time{"blog:post-1": prefs["one"].Dismissed["blog:post-1"]}; !reflect.DeepEqual(got, want) {
		t.Errorf("first user's dismissed preferences = %v, want %v", got, want)
	}
	if got, want := prefs["two"].Saved, map[string]time.Time{}; !reflect.DeepEqual(got, want) {
		t.Errorf("second user's saved preferences = %v, want %v", got, want)
	}
	if got, want := prefs["two"].Dismissed, map[string]time.Time{"video:clip-1": prefs["two"].Dismissed["video:clip-1"]}; !reflect.DeepEqual(got, want) {
		t.Errorf("second user's dismissed preferences = %v, want %v", got, want)
	}

	var saved map[string]*UserPrefs
	if err := data.LoadJSON("prefs.json", &saved); err != nil {
		t.Fatalf("load persisted preferences: %v", err)
	}
	if !reflect.DeepEqual(saved, prefs) {
		t.Errorf("persisted preferences = %#v, want %#v", saved, prefs)
	}
}
