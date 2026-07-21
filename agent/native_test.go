package agent

import (
	"testing"

	gmai "go-micro.dev/v6/ai"
)

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
