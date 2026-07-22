package api

import (
	"strings"
	"testing"
)

func TestEndpointsExcludeMarkets(t *testing.T) {
	for _, endpoint := range Endpoints {
		if endpoint.Path == "/markets" || endpoint.Name == "Markets" {
			t.Fatalf("removed Markets endpoint is still advertised: %#v", endpoint)
		}
	}
}

func TestEndpointsExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	for _, endpoint := range Endpoints {
		if strings.HasPrefix(endpoint.Path, "/"+domain) || strings.Contains(strings.ToLower(endpoint.Name), domain) {
			t.Fatalf("retired location endpoint is still advertised: %#v", endpoint)
		}
	}
}
