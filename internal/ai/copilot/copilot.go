// Package copilot implements a go-micro ai provider backed by a GitHub
// Copilot subscription. Copilot's API speaks the OpenAI chat-completions wire
// format and serves both OpenAI (gpt-*) and Anthropic (claude-*) model
// families through the same endpoint, so one driver covers both.
//
// The APIKey option carries the long-lived GitHub OAuth token; each request
// exchanges it (with package-level caching, see auth.go) for the short-lived
// Copilot bearer token and the plan-specific API base URL.
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go-micro.dev/v6/ai"
)

func init() {
	ai.Register("copilot", func(opts ...ai.Option) ai.Model {
		return NewProvider(opts...)
	})
	ai.RegisterStream("copilot")
}

// maxToolRounds bounds the driver-side tool loop. The agent layer applies its
// own MaxSteps guardrail on top by refusing calls past its budget.
const maxToolRounds = 8

// Provider implements the ai.Model interface for GitHub Copilot.
type Provider struct {
	opts ai.Options
}

// NewProvider creates a new Copilot provider.
func NewProvider(opts ...ai.Option) *Provider {
	options := ai.NewOptions(opts...)
	if options.Model == "" {
		// gpt-4.1 is included at a 0x premium-request multiplier on paid
		// Copilot plans, making it the safe default for unattended use.
		options.Model = "gpt-4.1"
	}
	return &Provider{opts: options}
}

// Init initializes the provider with options.
func (p *Provider) Init(opts ...ai.Option) error {
	for _, o := range opts {
		o(&p.opts)
	}
	return nil
}

// Options returns the provider options.
func (p *Provider) Options() ai.Options { return p.opts }

// String returns the provider name.
func (p *Provider) String() string { return "copilot" }

// Generate generates a response from the model, running a bounded multi-round
// tool loop when the model requests tool calls and a ToolHandler is set.
func (p *Provider) Generate(ctx context.Context, req *ai.Request, opts ...ai.GenerateOption) (*ai.Response, error) {
	var tools []map[string]any
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters": map[string]any{
					"type":       "object",
					"properties": t.Properties,
				},
			},
		})
	}

	messages := []map[string]any{
		{"role": "system", "content": req.SystemPrompt},
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
	}
	if req.Prompt != "" {
		messages = append(messages, map[string]any{"role": "user", "content": req.Prompt})
	}

	build := func(msgs []map[string]any) map[string]any {
		r := map[string]any{
			"model":    p.opts.Model,
			"messages": msgs,
		}
		if p.opts.MaxTokens > 0 {
			r["max_tokens"] = p.opts.MaxTokens
		}
		if len(tools) > 0 {
			r["tools"] = tools
		}
		return r
	}

	resp, rawMessage, err := p.callAPI(ctx, build(messages), len(tools) > 0)
	if err != nil {
		return nil, err
	}
	if len(resp.ToolCalls) == 0 || p.opts.ToolHandler == nil {
		return resp, nil
	}

	// Tool loop: execute calls, send results back with tools still offered,
	// repeat until the model answers in text or the round budget runs out.
	pending := resp.ToolCalls
	raw := rawMessage
	for round := 0; round < maxToolRounds && len(pending) > 0; round++ {
		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    raw["content"],
			"tool_calls": raw["tool_calls"],
		})
		for i := range pending {
			content := p.opts.ToolHandler(ctx, pending[i]).Content
			pending[i].Result = content
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": pending[i].ID,
				"content":      content,
			})
		}

		next, nextRaw, err := p.callAPI(ctx, build(messages), true)
		if err != nil {
			return resp, nil // keep what we have; tool results are recorded
		}
		resp.Usage.InputTokens += next.Usage.InputTokens
		resp.Usage.OutputTokens += next.Usage.OutputTokens
		resp.Usage.TotalTokens += next.Usage.TotalTokens

		if len(next.ToolCalls) > 0 {
			resp.ToolCalls = append(resp.ToolCalls, next.ToolCalls...)
			pending = next.ToolCalls
			raw = nextRaw
			continue
		}
		resp.Answer = next.Reply
		pending = nil
	}
	return resp, nil
}

// Stream generates a streaming response from the Copilot chat completions
// endpoint. Tool definitions are not sent on the streaming path — mirroring
// the other go-micro OpenAI-format drivers, whose agents run tools via
// Generate and stream only final answers.
func (p *Provider) Stream(ctx context.Context, req *ai.Request, opts ...ai.GenerateOption) (ai.Stream, error) {
	messages := []map[string]any{
		{"role": "system", "content": req.SystemPrompt},
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
	}
	if req.Prompt != "" {
		messages = append(messages, map[string]any{"role": "user", "content": req.Prompt})
	}
	apiReq := map[string]any{
		"model":    p.opts.Model,
		"messages": messages,
		"stream":   true,
	}
	if p.opts.MaxTokens > 0 {
		apiReq["max_tokens"] = p.opts.MaxTokens
	}
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("copilot: marshal stream request: %w", err)
	}

	httpReq, err := p.newAPIRequest(ctx, "/chat/completions", body, false)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("copilot: stream request: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("copilot: stream API error (%s): %s", httpResp.Status, strings.TrimSpace(string(respBody)))
	}
	return &copilotStream{body: httpResp.Body, scanner: bufio.NewScanner(httpResp.Body)}, nil
}

type copilotStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	closed  bool
}

func (s *copilotStream) Recv() (*ai.Response, error) {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil, io.EOF
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // tolerate non-JSON keepalives
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			return &ai.Response{Reply: chunk.Choices[0].Delta.Content}, nil
		}
		if chunk.Usage != nil {
			return &ai.Response{Usage: ai.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
			}}, nil
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (s *copilotStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

// newAPIRequest builds an authenticated request against the Copilot API base,
// exchanging the GitHub token for a bearer token as needed.
func (p *Provider) newAPIRequest(ctx context.Context, path string, body []byte, agentic bool) (*http.Request, error) {
	tok, apiBase, err := bearer(ctx, p.opts.APIKey)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	editorHeaders(req.Header)
	// Some models are gated on the traffic type; tool-driven requests are
	// agent traffic, plain chat is user traffic.
	if agentic {
		req.Header.Set("X-Initiator", "agent")
	} else {
		req.Header.Set("X-Initiator", "user")
	}
	return req, nil
}

// callAPI makes a chat-completions request and parses the response.
func (p *Provider) callAPI(ctx context.Context, apiReq map[string]any, agentic bool) (*ai.Response, map[string]any, error) {
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, nil, fmt.Errorf("copilot: marshal request: %w", err)
	}
	httpReq, err := p.newAPIRequest(ctx, "/chat/completions", reqBody, agentic)
	if err != nil {
		return nil, nil, err
	}
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("copilot: API request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("copilot: API error (%s): %s", httpResp.Status, strings.TrimSpace(string(respBody)))
	}

	var chatResp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("copilot: parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("copilot: no response from API")
	}

	choice := chatResp.Choices[0]
	response := &ai.Response{
		Reply: choice.Message.Content,
		Usage: ai.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
			input = map[string]any{}
		}
		response.ToolCalls = append(response.ToolCalls, ai.ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	rawMessage := map[string]any{
		"content":    choice.Message.Content,
		"tool_calls": choice.Message.ToolCalls,
	}
	return response, rawMessage, nil
}
