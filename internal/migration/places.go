package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"mu/internal/data"
)

const (
	placesRemovalVersion = 1
	placesRemovalMarker  = "places_removal_migration.json"
)

// RemovePlaces deletes data owned by the retired Places service. This migration
// can be removed after all supported installations have upgraded through v1.
func RemovePlaces() error {
	var marker map[string]int
	if err := data.LoadJSON(placesRemovalMarker, &marker); err == nil {
		if marker["version"] == placesRemovalVersion {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("load places removal marker: %w", err)
	}

	if err := data.DeleteFile("places_saved.json"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete saved places searches: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(data.Dir(), "places")); err != nil {
		return fmt.Errorf("delete places cache: %w", err)
	}
	for _, key := range []string{"places.db", "places.db-wal", "places.db-shm"} {
		if err := data.DeleteFile(key); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	if err := data.SaveJSON(placesRemovalMarker, map[string]int{"version": placesRemovalVersion}); err != nil {
		return fmt.Errorf("save places removal marker: %w", err)
	}
	return nil
}
