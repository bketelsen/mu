package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/service"
)

func TestLegacyPaymentFieldsAreIgnoredAndOmitted(t *testing.T) {
	var legacy App
	if err := json.Unmarshal([]byte(`{
		"id":"legacy","slug":"legacy-app","name":"Legacy","author_id":"owner",
		"price":25,"earnings":400,"public":true
	}`), &legacy); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"price"`, `"earnings"`} {
		if bytes.Contains(out, []byte(forbidden)) {
			t.Errorf("saved app retains removed field %s: %s", forbidden, out)
		}
	}
}

func TestLegacyPricedAppLaunchesWithoutPayment(t *testing.T) {
	var legacy App
	if err := json.Unmarshal([]byte(`{
		"id":"legacy","slug":"legacy-app","name":"Legacy","author_id":"owner",
		"html":"<!doctype html><title>Legacy</title>","price":25,"public":true
	}`), &legacy); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	original := apps
	apps = map[string]*App{legacy.Slug: &legacy}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		apps = original
		mutex.Unlock()
	}()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/apps/legacy-app/run", nil)
	handleRun(recorder, request, legacy.Slug)
	if recorder.Code == http.StatusUnauthorized || recorder.Code == http.StatusPaymentRequired {
		t.Fatalf("legacy app was payment-gated: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestAppsBuildViaMesh verifies the apps service RPC round-trip and endpoint
// name. Without an AI provider configured, Build returns an AI error — which
// still proves the request reached the handler (not a transport/endpoint error).
func TestAppsBuildViaMesh(t *testing.T) {
	if err := service.Register("apps", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp BuildResponse
	err := service.Call(context.Background(), "apps", "Server.Build",
		&BuildRequest{Prompt: "a water counter", AccountID: "u1"}, &rsp)
	if err == nil {
		return // an AI provider was configured and it built — also fine
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(strings.ToLower(err.Error()), "connection") {
		t.Fatalf("transport/endpoint error (routing broken): %v", err)
	}
}
