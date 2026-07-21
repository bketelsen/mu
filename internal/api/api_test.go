package api

import "testing"

func TestEndpointsIncludeNews(t *testing.T) {
	for _, endpoint := range Endpoints {
		if endpoint.Path == "/news" {
			return
		}
	}
	t.Fatal("news endpoint is not advertised")
}
