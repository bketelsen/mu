package apps

import (
	"os"
	"strings"
	"testing"
)

func TestSocialSDKIntegrationsAreRemoved(t *testing.T) {
	if calls := extractSDKCalls(`<script>mu.social()</script>`); len(calls) != 0 {
		t.Fatalf("social SDK call was still recognized: %#v", calls)
	}

	sdk, err := os.ReadFile("static/sdk.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sdk), "social:") || strings.Contains(string(sdk), "/social") {
		t.Fatal("shipped SDK still exposes social")
	}
}
