package admin

import (
	"fmt"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/flag"
)

// Delegated functions — building blocks should import internal/moderation directly.
// These exist only so admin's own handlers can call them.
var (
	RegisterDeleter = flag.RegisterDeleter
	SetAnalyzer     = flag.SetAnalyzer
	CheckContent    = flag.CheckContent
	IsHidden        = flag.IsHidden
	AdminFlag       = flag.AdminFlag
)

func Load() {
	flag.Load()
}

// ============================================
// HTTP HANDLERS
// ============================================

// FlagHandler handles flag submissions
func FlagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var contentType, contentID string

	if app.SendsJSON(r) {
		var req struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.RespondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		contentType = req.Type
		contentID = req.ID
	} else {
		contentType = r.FormValue("type")
		contentID = r.FormValue("id")
	}

	if contentID == "" || contentType == "" {
		http.Error(w, "Content ID and type required", http.StatusBadRequest)
		return
	}

	// Get the authenticated user
	flagger := "Anonymous"
	_, acc := auth.TrySession(r)
	if acc != nil {
		flagger = acc.Name
	}

	// Add flag
	count, alreadyFlagged, err := flag.Add(contentType, contentID, flagger)
	if err != nil {
		http.Error(w, "Failed to flag content", http.StatusInternalServerError)
		return
	}

	if alreadyFlagged {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": false, "message": "Already flagged"}`))
		return
	}

	// Refresh cache if content was hidden
	if count >= 3 {
		if deleter, ok := flag.GetDeleter(contentType); ok {
			deleter.RefreshCache()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true, "count": ` + fmt.Sprintf("%d", count) + `}`))
}
