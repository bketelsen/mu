package images

import (
	"net/http"
	"strings"

	"mu/internal/data"
)

// FileHandler serves locally stored generations at /images/file/<name>.
// Images land here when the configured backend (IMAGE_BASE_URL) returns
// inline base64 instead of a hosted URL — local servers have nowhere else
// to host them. Public like the rest of /images; names are random, and the
// gallery controls who sees which URL.
func FileHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/images/file/")
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	b, err := data.LoadFile("images/generated/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(b))
	// Generated files are immutable — a new generation gets a new name.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(b)
}
