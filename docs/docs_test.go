package docs

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

var forbiddenSingleOwnerCopy = []string{
	"sign up", "signup tool", "invite-only", "transfer credits to other users",
	"auto-created accounts", "activitypub federation", "pay per request without an account",
	"micro.mu", "pay per-request", "first 10 calls", "public event feed",
	"without an account", "no account needed", "no login to call",
	"pay per call with x402", "x-payment header instead of authenticating",
	"/wallet", "wallet_topup", "wallet_balance", "STRIPE_SECRET_KEY",
	"X402_PAY_TO", "pay per call with x402", "credits meter", "card top-ups",
	"outbound x402", "paid apps",
}

func TestDocument_Structure(t *testing.T) {
	doc := Document{
		Slug:        "test-doc",
		Filename:    "TEST.md",
		Title:       "Test Document",
		Description: "A test doc",
		Category:    "Testing",
	}
	if doc.Slug != "test-doc" {
		t.Error("expected slug")
	}
	if doc.Filename != "TEST.md" {
		t.Error("expected filename")
	}
}

func TestCatalog_NotEmpty(t *testing.T) {
	if len(catalog) == 0 {
		t.Error("catalog should not be empty")
	}
}

func TestCatalog_UniqueSlugs(t *testing.T) {
	seen := make(map[string]bool)
	for _, doc := range catalog {
		if seen[doc.Slug] {
			t.Errorf("duplicate slug: %q", doc.Slug)
		}
		seen[doc.Slug] = true
	}
}

func TestCatalog_AllFieldsPopulated(t *testing.T) {
	for _, doc := range catalog {
		if doc.Slug == "" {
			t.Errorf("doc %q has empty slug", doc.Title)
		}
		if doc.Filename == "" {
			t.Errorf("doc %q has empty filename", doc.Slug)
		}
		if doc.Title == "" {
			t.Errorf("doc %q has empty title", doc.Slug)
		}
		if doc.Description == "" {
			t.Errorf("doc %q has empty description", doc.Slug)
		}
		if doc.Category == "" {
			t.Errorf("doc %q has empty category", doc.Slug)
		}
	}
}

func TestCatalog_HasAboutDoc(t *testing.T) {
	found := false
	for _, doc := range catalog {
		if doc.Slug == "about" {
			found = true
			if doc.Filename != "ABOUT.md" {
				t.Errorf("about doc filename should be ABOUT.md, got %q", doc.Filename)
			}
			break
		}
	}
	if !found {
		t.Error("catalog should contain 'about' document")
	}
}

func TestCatalog_HasCategories(t *testing.T) {
	categories := make(map[string]bool)
	for _, doc := range catalog {
		categories[doc.Category] = true
	}
	expected := []string{"Getting Started", "Features", "Reference", "Developer"}
	for _, cat := range expected {
		if !categories[cat] {
			t.Errorf("expected category %q in catalog", cat)
		}
	}
}

func TestEmbeddedDocumentsDescribeSingleOwnerRuntime(t *testing.T) {
	err := fs.WalkDir(docsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || strings.HasPrefix(path, "superpowers/") {
			return nil
		}

		content, err := docsFS.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(content))
		for _, forbidden := range forbiddenSingleOwnerCopy {
			if strings.Contains(lower, strings.ToLower(forbidden)) {
				t.Errorf("%s contains stale product claim %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentProductCopyDoesNotAdvertiseMarkets(t *testing.T) {
	files := []string{"../README.md"}
	err := fs.WalkDir(docsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") && !strings.HasPrefix(path, "superpowers/") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		var content []byte
		if path == "../README.md" {
			content, err = os.ReadFile(path)
		} else {
			content, err = docsFS.ReadFile(path)
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(content)), "markets") {
			t.Errorf("%s advertises removed Markets capability", path)
		}
	}
}

func TestViewerLinksUseCatalogURLs(t *testing.T) {
	wants := map[string]string{
		"ABOUT.md":        "INSTALLATION.md",
		"INSTALLATION.md": "ENVIRONMENT_VARIABLES.md",
		"APPS.md":         "MCP.md",
		"COPILOT.md":      "ENVIRONMENT_VARIABLES.md",
	}
	for filename, target := range wants {
		want := ""
		for _, doc := range catalog {
			if doc.Filename == target {
				want = "/docs/" + doc.Slug
				break
			}
		}
		if want == "" {
			t.Fatalf("catalog has no document for %s", target)
		}
		content, err := docsFS.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "]("+want+")") {
			t.Errorf("%s does not link to %s", filename, want)
		}
	}
}
