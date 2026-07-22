package auth

import (
	"testing"

	"mu/internal/data"
)

func TestShowHomeCard(t *testing.T) {
	cases := []struct {
		name string
		acc  Account
		id   string
		want bool
	}{
		{"fresh account shows every default", Account{}, "images", true},
		{"legacy set (no seen) shows new card images",
			Account{HomeCards: []string{"blog", "news", "markets", "social", "video"}}, "images", true},
		{"legacy set keeps a chosen card",
			Account{HomeCards: []string{"blog", "news"}}, "blog", true},
		{"legacy set hides a pre-existing deselected card",
			Account{HomeCards: []string{"blog", "news"}}, "social", false},
		{"post-save deselect of images sticks",
			Account{HomeCards: []string{"blog", "news"}, HomeCardsSeen: []string{"blog", "news", "markets", "social", "video", "images", "mail", "web"}}, "images", false},
		{"post-save select of images shows",
			Account{HomeCards: []string{"blog", "images"}, HomeCardsSeen: []string{"blog", "news", "markets", "social", "video", "images", "mail", "web"}}, "images", true},
		{"future card after a save defaults on",
			Account{HomeCards: []string{"blog"}, HomeCardsSeen: []string{"blog", "news", "markets", "social", "video", "images", "mail", "web"}}, "audio", true},
		{"stale markets id does not affect known cards",
			Account{HomeCards: []string{"markets", "blog"}, HomeCardsSeen: []string{"blog", "news", "markets", "social", "video", "images", "mail", "web"}}, "blog", true},
	}
	for _, c := range cases {
		if got := c.acc.ShowHomeCard(c.id); got != c.want {
			t.Errorf("%s: ShowHomeCard(%q)=%v want %v", c.name, c.id, got, c.want)
		}
	}
}

func TestHomeCardActiveOptIn(t *testing.T) {
	if (&Account{}).HomeCardActive("mail") {
		t.Error("fresh account should not have mail opt-in active")
	}
	if !(&Account{HomeCards: []string{"mail"}}).HomeCardActive("mail") {
		t.Error("mail in HomeCards should be active")
	}
}

func TestRemoveHomeCardRemovesCardFromAllPreferencesAndPersists(t *testing.T) {
	resetMigrationState(t, map[string]*Account{
		"owner": {ID: "owner", HomeCards: []string{"news", "social", "social"}, HomeCardsSeen: []string{"social", "video"}},
	})

	if err := RemoveHomeCard("social"); err != nil {
		t.Fatalf("RemoveHomeCard: %v", err)
	}
	if got := accounts["owner"].HomeCards; len(got) != 1 || got[0] != "news" {
		t.Fatalf("HomeCards = %v, want [news]", got)
	}
	if got := accounts["owner"].HomeCardsSeen; len(got) != 1 || got[0] != "video" {
		t.Fatalf("HomeCardsSeen = %v, want [video]", got)
	}

	var persisted map[string]*Account
	if err := data.LoadJSON("accounts.json", &persisted); err != nil {
		t.Fatalf("load persisted accounts: %v", err)
	}
	if got := persisted["owner"].HomeCards; len(got) != 1 || got[0] != "news" {
		t.Fatalf("persisted HomeCards = %v, want [news]", got)
	}
}
