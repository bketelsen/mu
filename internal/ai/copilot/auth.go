package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GitHub Copilot authentication is two-tiered: a long-lived GitHub OAuth token
// (obtained once via the device flow below, scoped to a Copilot-enabled app)
// is exchanged for a short-lived Copilot bearer token (~30 minutes). The
// exchange response also carries the API base URL to use, which differs for
// Business/Enterprise plans, so we never hardcode the endpoint for requests.
var (
	// Endpoints are vars, not consts, so tests can point them at a local server.
	tokenEndpoint       = "https://api.github.com/copilot_internal/v2/token"
	deviceCodeEndpoint  = "https://github.com/login/device/code"
	accessTokenEndpoint = "https://github.com/login/oauth/access_token"

	defaultAPIBase = "https://api.githubcopilot.com"
)

// DefaultClientID is the OAuth app id of VS Code, a Copilot-enabled client.
// Copilot bearer tokens are only minted for OAuth tokens issued to such apps —
// a classic PAT will not work.
const DefaultClientID = "Iv1.b507a08c87ecfe98"

// editorHeaders identify the calling client to GitHub. Sent on every request.
func editorHeaders(h http.Header) {
	h.Set("Editor-Version", "vscode/1.99.3")
	h.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")
	h.Set("Copilot-Integration-Id", "vscode-chat")
	h.Set("User-Agent", "GitHubCopilotChat/0.26.7")
}

// session is a cached Copilot bearer token and the API base it is valid for.
type session struct {
	token     string
	apiBase   string
	expiresAt time.Time
}

var (
	sessMu   sync.Mutex
	sessions = map[string]*session{} // keyed by GitHub OAuth token
)

// bearer returns a valid Copilot bearer token and API base for the given
// GitHub token, exchanging and caching as needed. The cache is package-level
// because mu constructs a fresh provider instance per request.
func bearer(ctx context.Context, githubToken string) (token, apiBase string, err error) {
	sessMu.Lock()
	defer sessMu.Unlock()
	if s, ok := sessions[githubToken]; ok && time.Until(s.expiresAt) > 2*time.Minute {
		return s.token, s.apiBase, nil
	}
	s, err := exchange(ctx, githubToken)
	if err != nil {
		return "", "", err
	}
	sessions[githubToken] = s
	return s.token, s.apiBase, nil
}

// exchange trades a GitHub OAuth token for a short-lived Copilot bearer token.
func exchange(ctx context.Context, githubToken string) (*session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	editorHeaders(req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot token exchange (%s): %s — check the GitHub token has an active Copilot subscription", resp.Status, strings.TrimSpace(string(body)))
	}

	var out struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
		Endpoints struct {
			API string `json:"api"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("copilot token exchange: parse: %w", err)
	}
	if out.Token == "" {
		return nil, fmt.Errorf("copilot token exchange: empty token in response")
	}
	apiBase := strings.TrimRight(out.Endpoints.API, "/")
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	expires := time.Unix(out.ExpiresAt, 0)
	if out.ExpiresAt == 0 {
		expires = time.Now().Add(15 * time.Minute)
	}
	return &session{token: out.Token, apiBase: apiBase, expiresAt: expires}, nil
}

// DeviceCode is an in-progress GitHub device-flow authorization.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartDeviceFlow begins the GitHub device authorization flow and returns the
// code the user must enter at the verification URL.
func StartDeviceFlow(ctx context.Context) (*DeviceCode, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id": DefaultClientID,
		"scope":     "read:user",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device flow: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device flow (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var dc DeviceCode
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, fmt.Errorf("device flow: parse: %w", err)
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// WaitForDeviceToken polls GitHub until the user approves the device code,
// then returns the long-lived GitHub OAuth token to store.
func WaitForDeviceToken(ctx context.Context, dc *DeviceCode) (string, error) {
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn == 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}
	interval := time.Duration(dc.Interval) * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		payload, _ := json.Marshal(map[string]string{
			"client_id":   DefaultClientID,
			"device_code": dc.DeviceCode,
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenEndpoint, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("device flow poll: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var out struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			Interval    int    `json:"interval"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", fmt.Errorf("device flow poll: parse: %w", err)
		}
		switch out.Error {
		case "":
			if out.AccessToken != "" {
				return out.AccessToken, nil
			}
		case "authorization_pending":
			continue
		case "slow_down":
			if out.Interval > 0 {
				interval = time.Duration(out.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		default:
			return "", fmt.Errorf("device flow: %s", out.Error)
		}
	}
	return "", fmt.Errorf("device flow: timed out waiting for approval")
}

// ModelInfo is one entry from the Copilot /models catalog.
type ModelInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	// SupportedEndpoints lists the API shapes that serve this model, e.g.
	// "/chat/completions" or "/responses". Newer OpenAI (Codex-generation)
	// models are /responses-only and reject /chat/completions with
	// unsupported_api_for_model.
	SupportedEndpoints []string `json:"supported_endpoints"`
}

// catalogEntry caches the /models catalog per GitHub token so per-request
// endpoint routing doesn't refetch it. Short TTL: model availability changes
// rarely, but a stale-forever cache would mask newly enabled models.
type catalogEntry struct {
	models  []ModelInfo
	fetched time.Time
}

var (
	catalogMu    sync.Mutex
	catalogCache = map[string]*catalogEntry{}
)

const catalogTTL = 10 * time.Minute

// cachedModels returns the model catalog, fetching at most once per TTL.
// ok is false when the catalog could not be fetched (e.g. rate-limited), so
// callers can fall back to defaults instead of failing the request.
func cachedModels(ctx context.Context, githubToken string) (models []ModelInfo, ok bool) {
	catalogMu.Lock()
	if e, hit := catalogCache[githubToken]; hit && time.Since(e.fetched) < catalogTTL {
		catalogMu.Unlock()
		return e.models, true
	}
	catalogMu.Unlock()

	fetched, err := ListModels(ctx, githubToken)
	if err != nil {
		return nil, false
	}
	catalogMu.Lock()
	catalogCache[githubToken] = &catalogEntry{models: fetched, fetched: time.Now()}
	catalogMu.Unlock()
	return fetched, true
}

// catalogEndpoints returns the supported endpoints for a model from the
// cached catalog. ok is false when the catalog was unavailable — distinct
// from a model that is present but lists no endpoints.
func catalogEndpoints(ctx context.Context, githubToken, model string) (endpoints []string, ok bool) {
	models, ok := cachedModels(ctx, githubToken)
	if !ok {
		return nil, false
	}
	for _, m := range models {
		if m.ID == model {
			return m.SupportedEndpoints, true
		}
	}
	return nil, true
}

// ListModels returns the models available to the subscription behind the
// given GitHub token — both OpenAI (gpt-*) and Anthropic (claude-*) families.
func ListModels(ctx context.Context, githubToken string) ([]ModelInfo, error) {
	tok, apiBase, err := bearer(ctx, githubToken)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	editorHeaders(req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot models: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot models (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("copilot models: parse: %w", err)
	}
	return out.Data, nil
}
