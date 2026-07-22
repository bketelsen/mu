package main

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestDomainServicesContainNoPaymentGates(t *testing.T) {
	files := []string{
		"chat/chat.go", "search/search.go", "search/read.go", "search/fetch.go",
		"news/news.go", "video/video.go", "social/social.go", "mail/mail.go",
		"places/places.go", "weather/weather.go", "images/images.go",
	}
	forbidden := []string{
		`"mu/wallet"`, "wallet.CheckQuota(", "wallet.ConsumeQuota(",
		"wallet.DeductCredits(", "wallet.QuotaExceededPage(", "http.StatusPaymentRequired",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(content), needle) {
				t.Errorf("%s retains payment gate %q", file, needle)
			}
		}
	}
}

func TestPaymentImplementationPathsAreDeleted(t *testing.T) {
	paths := []string{"wallet", "internal/cli/wallet.go", "internal/cli/x402.go"}
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("removed payment path still exists: %s (error=%v)", path, err)
		}
	}
}

func TestMainContainsNoPaymentComposition(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"mu/wallet"`, "wallet.Load(", `http.HandleFunc("/wallet`,
		`r.URL.Path == "/wallet/stripe/webhook"`, "chargedWriteOp(",
	} {
		if strings.Contains(string(content), forbidden) {
			t.Errorf("main.go retains payment composition %q", forbidden)
		}
	}
}
