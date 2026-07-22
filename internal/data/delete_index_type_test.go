package data

import (
	"testing"
)

func TestDeleteIndexTypeInMemoryRemovesOnlyMatchingEntriesAndPersists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	UseSQLite = false

	indexMutex.Lock()
	index = map[string]*IndexEntry{
		"social-1": {ID: "social-1", Type: "social", Title: "Social migration", Content: "obsolete social content"},
		"news-1":   {ID: "news-1", Type: "news", Title: "News migration", Content: "current news content"},
	}
	indexMutex.Unlock()
	if err := SaveJSON("index.json", index); err != nil {
		t.Fatalf("save initial index: %v", err)
	}

	if err := DeleteIndexType("social"); err != nil {
		t.Fatalf("DeleteIndexType: %v", err)
	}
	if err := DeleteIndexType("absent"); err != nil {
		t.Fatalf("DeleteIndexType absent type: %v", err)
	}

	if entry := GetByID("social-1"); entry != nil {
		t.Fatalf("social entry remains in memory: %#v", entry)
	}
	if entry := GetByID("news-1"); entry == nil {
		t.Fatal("unrelated news entry was removed")
	}

	var persisted map[string]*IndexEntry
	if err := LoadJSON("index.json", &persisted); err != nil {
		t.Fatalf("load persisted index: %v", err)
	}
	if persisted["social-1"] != nil || persisted["news-1"] == nil {
		t.Fatalf("persisted index = %#v, want only news entry", persisted)
	}
}

func TestDeleteIndexTypeSQLiteRemovesFTSMatchesAndPreservesOtherEntries(t *testing.T) {
	resetSQLiteTestDB(t)
	oldUseSQLite := UseSQLite
	UseSQLite = true
	t.Cleanup(func() { UseSQLite = oldUseSQLite })

	if err := IndexSQLite("social-1", "social", "Social migration", "legacy migration content", "", nil); err != nil {
		t.Fatalf("index social entry: %v", err)
	}
	if err := IndexSQLite("news-1", "news", "News migration", "current migration content", "", nil); err != nil {
		t.Fatalf("index news entry: %v", err)
	}
	if err := DeleteIndexType("social"); err != nil {
		t.Fatalf("DeleteIndexType: %v", err)
	}
	if err := DeleteIndexType("absent"); err != nil {
		t.Fatalf("DeleteIndexType absent type: %v", err)
	}

	if entry := GetByID("social-1"); entry != nil {
		t.Fatalf("social entry remains: %#v", entry)
	}
	if entry := GetByID("news-1"); entry == nil {
		t.Fatal("unrelated news entry was removed")
	}
	results := Search("migration", 10)
	if got := ids(results); got["social-1"] || !got["news-1"] {
		t.Fatalf("search results after deletion = %v, want only news entry", got)
	}
	var socialFTSRows, newsFTSRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM index_fts WHERE index_fts MATCH ?`, "legacy").Scan(&socialFTSRows); err != nil {
		t.Fatalf("search social FTS postings: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM index_fts WHERE index_fts MATCH ?`, "current").Scan(&newsFTSRows); err != nil {
		t.Fatalf("search news FTS postings: %v", err)
	}
	if socialFTSRows != 0 || newsFTSRows != 1 {
		t.Fatalf("FTS postings after deletion: social=%d, news=%d; want social=0, news=1", socialFTSRows, newsFTSRows)
	}
}
