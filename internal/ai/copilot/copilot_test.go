package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-micro.dev/v6/ai"
)

// newTestServer stands in for both the GitHub token-exchange endpoint and the
// Copilot API. The exchange response points endpoints.api back at the server,
// mirroring how real plans get their API base from the exchange.
func newTestServer(t *testing.T, chat http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var exchanges atomic.Int64
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/copilot_internal/v2/token", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "token ") {
			t.Errorf("token exchange auth = %q, want token prefix", got)
		}
		exchanges.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"token":      "short-lived-bearer",
			"expires_at": time.Now().Add(30 * time.Minute).Unix(),
			"endpoints":  map[string]string{"api": srv.URL},
		})
	})
	mux.HandleFunc("/chat/completions", chat)
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer short-lived-bearer" {
			t.Errorf("models auth = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-4.1", "name": "GPT-4.1", "vendor": "openai"},
				{"id": "claude-sonnet-4.5", "name": "Claude Sonnet 4.5", "vendor": "anthropic"},
			},
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldToken := tokenEndpoint
	tokenEndpoint = srv.URL + "/copilot_internal/v2/token"
	t.Cleanup(func() { tokenEndpoint = oldToken })
	return srv, &exchanges
}

func chatText(reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": reply}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}
}

func TestGenerateAndTokenCache(t *testing.T) {
	var headers atomic.Value
	_, exchanges := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		headers.Store(r.Header.Clone())
		chatText("hello there")(w, r)
	})

	p := NewProvider(ai.WithAPIKey("gho_cache_test"), ai.WithModel("gpt-4.1"))
	for i := 0; i < 3; i++ {
		resp, err := p.Generate(context.Background(), &ai.Request{SystemPrompt: "sys", Prompt: "hi"})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if resp.Reply != "hello there" {
			t.Fatalf("Reply = %q", resp.Reply)
		}
		if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
			t.Fatalf("Usage = %+v", resp.Usage)
		}
	}
	if n := exchanges.Load(); n != 1 {
		t.Fatalf("token exchanged %d times, want 1 (cache)", n)
	}

	h := headers.Load().(http.Header)
	if got := h.Get("Authorization"); got != "Bearer short-lived-bearer" {
		t.Errorf("Authorization = %q", got)
	}
	if got := h.Get("Copilot-Integration-Id"); got != "vscode-chat" {
		t.Errorf("Copilot-Integration-Id = %q", got)
	}
	if h.Get("Editor-Version") == "" || h.Get("Editor-Plugin-Version") == "" {
		t.Error("missing editor headers")
	}
	if got := h.Get("X-Initiator"); got != "user" {
		t.Errorf("X-Initiator = %q, want user (no tools)", got)
	}
}

func TestGenerateToolLoop(t *testing.T) {
	var calls atomic.Int64
	_, _ = newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]any `json:"messages"`
			Tools    []any            `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Tools) == 0 {
			t.Error("tools missing from request")
		}
		if got := r.Header.Get("X-Initiator"); got != "agent" {
			t.Errorf("X-Initiator = %q, want agent (tools present)", got)
		}
		switch calls.Add(1) {
		case 1:
			// Round 1: model asks for a tool.
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{
					"content": "",
					"tool_calls": []map[string]any{{
						"id": "call_1",
						"function": map[string]any{
							"name":      "weather_Weather_Now",
							"arguments": `{"city":"London"}`,
						},
					}},
				}}},
			})
		default:
			// Round 2: tool result must be in the conversation.
			var sawToolResult bool
			for _, m := range req.Messages {
				if m["role"] == "tool" && m["content"] == "sunny, 21C" {
					sawToolResult = true
				}
			}
			if !sawToolResult {
				t.Error("tool result missing from follow-up messages")
			}
			chatText("It is sunny and 21C in London.")(w, r)
		}
	})

	var handled atomic.Int64
	p := NewProvider(
		ai.WithAPIKey("gho_tool_test"),
		ai.WithModel("claude-sonnet-4.5"),
		ai.WithToolHandler(func(ctx context.Context, call ai.ToolCall) ai.ToolResult {
			handled.Add(1)
			if call.Name != "weather_Weather_Now" || call.Input["city"] != "London" {
				t.Errorf("unexpected tool call %s %v", call.Name, call.Input)
			}
			return ai.ToolResult{ID: call.ID, Content: "sunny, 21C"}
		}),
	)
	resp, err := p.Generate(context.Background(), &ai.Request{
		SystemPrompt: "sys",
		Prompt:       "weather in london?",
		Tools: []ai.Tool{{
			Name:        "weather_Weather_Now",
			Description: "current weather",
			Properties:  map[string]any{"city": map[string]any{"type": "string"}},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if handled.Load() != 1 {
		t.Fatalf("tool handler ran %d times, want 1", handled.Load())
	}
	if resp.Answer != "It is sunny and 21C in London." {
		t.Fatalf("Answer = %q", resp.Answer)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Result != "sunny, 21C" {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
}

func TestStream(t *testing.T) {
	_, _ = newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Error("stream not requested")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, tok := range []string{"Hel", "lo"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
		}
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})

	p := NewProvider(ai.WithAPIKey("gho_stream_test"), ai.WithModel("gpt-4.1"))
	stream, err := p.Stream(context.Background(), &ai.Request{SystemPrompt: "sys", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var text strings.Builder
	var usage ai.Usage
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		text.WriteString(resp.Reply)
		if resp.Usage.TotalTokens > 0 {
			usage = resp.Usage
		}
	}
	if text.String() != "Hello" {
		t.Fatalf("streamed text = %q", text.String())
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestListModels(t *testing.T) {
	_, _ = newTestServer(t, chatText("unused"))
	models, err := ListModels(context.Background(), "gho_models_test")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0].ID != "gpt-4.1" || models[1].Vendor != "anthropic" {
		t.Fatalf("models = %+v", models)
	}
}

func TestRegistered(t *testing.T) {
	m := ai.New("copilot", ai.WithAPIKey("gho_x"))
	if m == nil || m.String() != "copilot" {
		t.Fatal("copilot provider not registered")
	}
	caps := ai.ProviderCapabilities("copilot")
	if !caps.Model || !caps.Stream {
		t.Fatalf("capabilities = %+v, want model+stream", caps)
	}
}
