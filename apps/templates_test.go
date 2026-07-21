package apps

import "testing"

func TestDashboardTemplateExists(t *testing.T) {
	if template := GetTemplate("dashboard"); template == nil {
		t.Fatal("dashboard template is not registered")
	}
}
