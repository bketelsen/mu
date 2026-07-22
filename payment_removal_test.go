package main

import (
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
