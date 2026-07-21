package ai

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/data"
	"mu/internal/settings"
)

// ImageModelID is the Atlas Cloud text-to-image model Mu generates with.
// Atlas model ids are vendor/model/task (e.g. "google/nano-banana/text-to-image");
// the "/text-to-image" suffix is required. Override with the IMAGE_MODEL setting.
const ImageModelID = "google/nano-banana-2-lite/text-to-image"

// atlasImageBase is Atlas Cloud's async image API host.
const atlasImageBase = "https://api.atlascloud.ai"

var imageHTTPClient = &http.Client{Timeout: 30 * time.Second}

// imageModel returns the configured image model id.
func imageModel() string {
	if v := strings.TrimSpace(settings.Get("IMAGE_MODEL")); v != "" {
		return v
	}
	return ImageModelID
}

// imageBaseURL returns the OpenAI-compatible image endpoint, if configured.
func imageBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(settings.Get("IMAGE_BASE_URL")), "/")
}

// GenerateImage turns a text prompt into an image and returns a URL to the
// result. Two backends, resolved in order:
//
//  1. IMAGE_BASE_URL — an OpenAI-compatible /images/generations server (e.g. a
//     local Lemonade or Stable Diffusion box). Being image-specific it wins
//     over the general Atlas key, and it covers gateways like Copilot that
//     have no image models at all. Optional IMAGE_API_KEY and IMAGE_MODEL.
//  2. Atlas Cloud's async image API (needs ATLAS_API_KEY). Called directly
//     with the documented minimal body ({model, prompt, aspect_ratio}); the
//     go-micro provider hardcodes gpt-image-2 params that nano-banana rejects.
func GenerateImage(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	// Keep prompts within a sane bound so the API doesn't reject overlong input.
	if len(prompt) > 2000 {
		prompt = prompt[:2000]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	if base := imageBaseURL(); base != "" {
		return generateImageOpenAI(ctx, base, prompt)
	}

	key := getAtlasAPIKey()
	if key == "" {
		return "", fmt.Errorf("image generation needs an Atlas Cloud API key (ATLAS_API_KEY) or an OpenAI-compatible image endpoint (IMAGE_BASE_URL)")
	}
	id, err := submitImage(ctx, key, prompt)
	if err != nil {
		return "", err
	}
	return pollImage(ctx, key, id)
}

// generateImageOpenAI calls an OpenAI-compatible /images/generations endpoint.
// Servers answer with either a hosted URL or inline base64; base64 results are
// stored in the data dir and served back via /images/file/.
func generateImageOpenAI(ctx context.Context, base, prompt string) (string, error) {
	payload := map[string]any{"prompt": prompt}
	// No defaults here: local servers generate with whatever model they have
	// loaded when none is named, and at their configured resolution when no
	// size is given.
	if m := strings.TrimSpace(settings.Get("IMAGE_MODEL")); m != "" {
		payload["model"] = m
	}
	if s := strings.TrimSpace(settings.Get("IMAGE_SIZE")); s != "" {
		payload["size"] = s // OpenAI wire format, e.g. "1024x1024"
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(settings.Get("IMAGE_API_KEY")); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	// Local diffusion can take minutes on modest hardware; the context bounds
	// the wait, not the short client used for Atlas submissions.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("image endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image API error (%s): %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) == 0 {
		return "", fmt.Errorf("unexpected image API response: %s", strings.TrimSpace(string(raw)))
	}
	if u := out.Data[0].URL; u != "" {
		return u, nil
	}
	img, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
	if err != nil || len(img) == 0 {
		return "", fmt.Errorf("image API returned no usable image")
	}

	suffix := make([]byte, 4)
	rand.Read(suffix)
	name := fmt.Sprintf("%d-%s.png", time.Now().UnixNano(), hex.EncodeToString(suffix))
	if err := data.SaveFile("images/generated/"+name, string(img)); err != nil {
		return "", fmt.Errorf("failed to store generated image: %w", err)
	}
	return "/images/file/" + name, nil
}

// submitImage POSTs the generation request and returns the prediction id.
func submitImage(ctx context.Context, key, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":        imageModel(),
		"prompt":       prompt,
		"aspect_ratio": "1:1",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, atlasImageBase+"/api/v1/model/generateImage", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image API error (%s): %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("unexpected image API response: %s", strings.TrimSpace(string(raw)))
	}
	if out.Code != 200 || out.Data.ID == "" {
		if out.Msg != "" {
			return "", fmt.Errorf("image generation failed: %s", out.Msg)
		}
		return "", fmt.Errorf("image generation failed")
	}
	return out.Data.ID, nil
}

// pollImage waits for the prediction to complete and returns the first image URL.
func pollImage(ctx context.Context, key, id string) (string, error) {
	url := atlasImageBase + "/api/v1/model/prediction/" + id
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("image generation timed out")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("Authorization", "Bearer "+key)
			resp, err := imageHTTPClient.Do(req)
			if err != nil {
				return "", err
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var out struct {
				Data struct {
					Status  string   `json:"status"`
					Outputs []string `json:"outputs"`
					Error   string   `json:"error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				continue // transient; keep polling until the deadline
			}
			switch out.Data.Status {
			case "completed":
				if len(out.Data.Outputs) > 0 && out.Data.Outputs[0] != "" {
					return out.Data.Outputs[0], nil
				}
				return "", fmt.Errorf("image generation returned no output")
			case "failed":
				msg := out.Data.Error
				if msg == "" {
					msg = "generation failed"
				}
				return "", fmt.Errorf("image generation failed: %s", msg)
			}
		}
	}
}
