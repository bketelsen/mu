package api

import "testing"

func TestEndpointsExcludeMarkets(t *testing.T) {
	for _, endpoint := range Endpoints {
		if endpoint.Path == "/markets" || endpoint.Name == "Markets" {
			t.Fatalf("removed Markets endpoint is still advertised: %#v", endpoint)
		}
	}
}
