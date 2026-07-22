package migration

import (
	"os"
	"path/filepath"
	"testing"

	"mu/internal/data"
	"mu/wallet"
)

func writeRemovalFixture(t *testing.T, key string) {
	t.Helper()
	path := filepath.Join(data.Dir(), key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePlacesDeletesDataAndHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{
		"places_saved.json",
		"places/london.json",
		"places.db",
		"places.db-wal",
		"places.db-shm",
	} {
		writeRemovalFixture(t, key)
	}
	const owner = "places-removal-owner"
	if err := wallet.AddCredits(owner, 10, "places_search", nil); err != nil {
		t.Fatal(err)
	}
	if err := wallet.AddCredits(owner, 5, wallet.OpTopup, nil); err != nil {
		t.Fatal(err)
	}

	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"places_saved.json", "places", "places.db", "places.db-wal", "places.db-shm",
	} {
		if _, err := os.Stat(filepath.Join(data.Dir(), key)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", key, err)
		}
	}
	if got := wallet.GetTransactions(owner, 10); len(got) != 1 || got[0].Operation != wallet.OpTopup {
		t.Fatalf("remaining transactions = %#v", got)
	}
	if got := wallet.GetBalance(owner); got != 15 {
		t.Fatalf("balance = %d, want 15", got)
	}
	var marker map[string]int
	if err := data.LoadJSON(placesRemovalMarker, &marker); err != nil {
		t.Fatal(err)
	}
	if marker["version"] != placesRemovalVersion {
		t.Fatalf("marker = %#v", marker)
	}
}

func TestRemovePlacesCompletedMarkerMakesRetryNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	writeRemovalFixture(t, "places_saved.json")
	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data.Dir(), "places_saved.json")); err != nil {
		t.Fatalf("completed migration ran again: %v", err)
	}
}

func TestRemovePlacesFailureDoesNotWriteMarkerAndIsRetryable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(data.Dir(), "places.db")
	if err := os.MkdirAll(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dbPath, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePlaces(); err == nil {
		t.Fatal("expected deleting a directory as a data file to fail")
	}
	if _, err := os.Stat(filepath.Join(data.Dir(), placesRemovalMarker)); !os.IsNotExist(err) {
		t.Fatalf("marker written after failure: %v", err)
	}
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := RemovePlaces(); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}
