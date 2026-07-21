package images

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/data"
)

func TestFileHandlerServesStoredImage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	png := "\x89PNG\r\n\x1a\n0123456789"
	if err := data.SaveFile("images/generated/test.png", png); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	FileHandler(rec, httptest.NewRequest("GET", "/images/file/test.png", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != png {
		t.Error("served bytes differ from stored bytes")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestFileHandlerRejectsTraversalAndMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, path := range []string{
		"/images/file/",
		"/images/file/../settings.json",
		"/images/file/nope.png",
	} {
		rec := httptest.NewRecorder()
		FileHandler(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Errorf("GET %s status = %d, want 404", path, rec.Code)
		}
	}
}
