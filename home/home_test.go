package home

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHtmlEsc(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"a & b", "a &amp; b"},
		{"", ""},
	}
	for _, tt := range tests {
		got := htmlEsc(tt.input)
		if got != tt.expected {
			t.Errorf("htmlEsc(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCardConfigExcludesMarkets(t *testing.T) {
	b, err := f.ReadFile("cards.json")
	if err != nil {
		t.Fatal(err)
	}
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	for _, card := range append(config.Left, config.Right...) {
		if card.ID == "markets" || card.Type == "markets" || card.Link == "/markets" {
			t.Fatalf("removed Markets card remains configured: %#v", card)
		}
	}
}

func TestPricingExcludesRetiredLocationDomain(t *testing.T) {
	req := httptest.NewRequest("GET", "/pricing", nil)
	recorder := httptest.NewRecorder()
	PricingHandler(recorder, req)
	if strings.Contains(strings.ToLower(recorder.Body.String()), "<td>"+"pla"+"ces") {
		t.Fatalf("pricing retains retired location domain: %s", recorder.Body.String())
	}
}
