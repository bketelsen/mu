package agent

import (
	"os"
	"slices"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/ai"
)

func TestAgentInventoriesExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	for _, services := range [][]string{nativeServices(true), nativeServices(false), AllAgentTools()} {
		if slices.Contains(services, domain) {
			t.Fatalf("agent service inventory retains retired location domain: %v", services)
		}
	}
}

func TestNativePromptSourceExcludesRetiredLocationCapability(t *testing.T) {
	source, err := os.ReadFile("native.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "pla"+"ces and points of interest") {
		t.Fatal("native prompt still advertises retired location capability")
	}
}

func TestNativeToolCallKeyDedupesEquivalentInputs(t *testing.T) {
	first := nativeToolCallKey(gmai.ToolCall{Name: "weather_Forecast", Input: map[string]any{"lat": 51.5074, "lon": -0.1278}})
	second := nativeToolCallKey(gmai.ToolCall{Name: "weather_Forecast", Input: map[string]any{"lon": -0.1278, "lat": 51.5074}})
	if first != second {
		t.Fatalf("expected equivalent native tool inputs to share a dedupe key: %q vs %q", first, second)
	}
	if first == nativeToolCallKey(gmai.ToolCall{Name: "weather_Forecast", Input: map[string]any{"lat": 40.7128, "lon": -74.0060}}) {
		t.Fatal("expected distinct native weather inputs to keep distinct dedupe keys")
	}
}
