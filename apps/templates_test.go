package apps

import (
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
