package apps

import (
	"os"
	"strings"
	"testing"
)

func TestTemplatesExcludeMarketsCapability(t *testing.T) {
	if template := GetTemplate("markets"); template != nil {
		t.Fatalf("removed Markets template remains: %#v", template)
	}
	if template := GetTemplate("portfolio"); template != nil {
		t.Fatalf("market-dependent Portfolio template remains: %#v", template)
	}
	for _, template := range Templates {
		if strings.Contains(template.HTML, "mu.markets") {
			t.Fatalf("template %q calls removed mu.markets API", template.ID)
		}
	}
}

func TestTemplatesAndSDKExcludeRetiredLocationCapability(t *testing.T) {
	domain := "pla" + "ces"
	if template := GetTemplate("place-" + "explorer"); template != nil {
		t.Fatalf("removed Place Explorer template remains: %#v", template)
	}
	for _, template := range Templates {
		if strings.Contains(template.HTML, "mu."+domain) {
			t.Fatalf("template %q calls retired location API", template.ID)
		}
	}
	for _, path := range []string{"apps.go", "static/sdk.js"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), domain+":") || strings.Contains(string(source), "/"+domain+"/") {
			t.Fatalf("%s retains retired location SDK methods", path)
		}
	}
}

func TestWeatherTemplateGeocodesCitiesWithoutRetiredLocationAPI(t *testing.T) {
	weather := GetTemplate("weather")
	if weather == nil {
		t.Fatal("weather template is missing")
	}
	if strings.Contains(weather.HTML, "mu."+("pla"+"ces")) {
		t.Fatal("weather template still depends on retired location API")
	}
	if !strings.Contains(weather.HTML, "nominatim.openstreetmap.org/search") {
		t.Fatal("weather template lost city geocoding")
	}
}
