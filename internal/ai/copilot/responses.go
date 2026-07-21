package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"go-micro.dev/v6/ai"
)

// Copilot serves newer OpenAI (Codex-generation) models only through the
// OpenAI Responses API (/responses); calling /chat/completions for them fails
// with error code unsupported_api_for_model. This file implements the
// Responses wire format: `input` items instead of messages, flat function
// tool definitions, function_call/function_call_output items for the tool
// loop, max_output_tokens, and typed SSE events for streaming.

// modelEndpoints remembers, per model id, whether the model is
// /responses-only. Seeded from the /models catalog (supported_endpoints) and
// corrected by unsupported_api_for_model errors when the catalog is stale or
// unavailable.
var modelEndpoints sync.Map // model id → bool (true = /responses-only)

// responsesOnly reports whether the provider's model must use /responses.
func (p *Provider) responsesOnly(ctx context.Context) bool {
	if v, ok := modelEndpoints.Load(p.opts.Model); ok {
		return v.(bool)
	}
	eps, ok := catalogEndpoints(ctx, p.opts.APIKey, p.opts.Model)
	if !ok {
		// Catalog unavailable (e.g. rate-limited): don't cache a guess —
		// default to chat and let the error fallback correct us.
		return false
	}
	var chat, responses bool
	for _, e := range eps {
		if strings.Contains(e, "chat/completions") {
			chat = true
		}
		if strings.Contains(e, "responses") {
			responses = true
		}
	}
	ro := responses && !chat
	modelEndpoints.Store(p.opts.Model, ro)
	return ro
}

// isUnsupportedAPI matches the Copilot error for a model called on the wrong
// endpoint, e.g. `model "gpt-5.6-terra" is not accessible via the
// /chat/completions endpoint (unsupported_api_for_model)`.
func isUnsupportedAPI(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unsupported_api_for_model")
}

// responsesTools converts go-micro tools to the Responses API shape, which
// flattens the function definition instead of nesting it.
func responsesTools(reqTools []ai.Tool) []map[string]any {
	var tools []map[string]any
	for _, t := range reqTools {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": t.Properties,
			},
		})
	}
	return tools
}

// responsesInput converts conversation history + prompt into input items.
func responsesInput(req *ai.Request) []map[string]any {
	var input []map[string]any
	for _, m := range req.Messages {
		input = append(input, map[string]any{"role": m.Role, "content": m.Content})
	}
	if req.Prompt != "" {
		input = append(input, map[string]any{"role": "user", "content": req.Prompt})
	}
	return input
}

func (p *Provider) buildResponsesRequest(req *ai.Request, input []map[string]any, tools []map[string]any, stream bool) map[string]any {
	r := map[string]any{
		"model": p.opts.Model,
		"input": input,
		// Stateless: the full conversation is resent each round, matching
		// how the chat-completions path works.
		"store": false,
	}
	if req.SystemPrompt != "" {
		r["instructions"] = req.SystemPrompt
	}
	if p.opts.MaxTokens > 0 {
		r["max_output_tokens"] = p.opts.MaxTokens
	}
	if len(tools) > 0 {
		r["tools"] = tools
	}
	if stream {
		r["stream"] = true
	}
	return r
}

// generateResponses is the /responses counterpart of the chat Generate path,
// including the bounded multi-round tool loop.
func (p *Provider) generateResponses(ctx context.Context, req *ai.Request) (*ai.Response, error) {
	tools := responsesTools(req.Tools)
	input := responsesInput(req)

	resp, callItems, err := p.callResponses(ctx, p.buildResponsesRequest(req, input, tools, false), len(tools) > 0)
	if err != nil {
		return nil, err
	}
	if len(resp.ToolCalls) == 0 || p.opts.ToolHandler == nil {
		return resp, nil
	}

	pending := resp.ToolCalls
	items := callItems
	for round := 0; round < maxToolRounds && len(pending) > 0; round++ {
		// Echo the model's function_call items, then append the outputs.
		input = append(input, items...)
		for i := range pending {
			content := p.opts.ToolHandler(ctx, pending[i]).Content
			pending[i].Result = content
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": pending[i].ID,
				"output":  content,
			})
		}

		next, nextItems, err := p.callResponses(ctx, p.buildResponsesRequest(req, input, tools, false), true)
		if err != nil {
			return resp, nil // keep what we have; tool results are recorded
		}
		resp.Usage.InputTokens += next.Usage.InputTokens
		resp.Usage.OutputTokens += next.Usage.OutputTokens
		resp.Usage.TotalTokens += next.Usage.TotalTokens

		if len(next.ToolCalls) > 0 {
			resp.ToolCalls = append(resp.ToolCalls, next.ToolCalls...)
			pending = next.ToolCalls
			items = nextItems
			continue
		}
		resp.Answer = next.Reply
		pending = nil
	}
	return resp, nil
}

// callResponses makes a /responses request. Alongside the parsed response it
// returns the raw function_call items, which the tool loop must echo back in
// the next round's input.
func (p *Provider) callResponses(ctx context.Context, apiReq map[string]any, agentic bool) (*ai.Response, []map[string]any, error) {
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, nil, fmt.Errorf("copilot: marshal request: %w", err)
	}
	httpReq, err := p.newAPIRequest(ctx, "/responses", reqBody, agentic)
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

	var out struct {
		Output []struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, nil, fmt.Errorf("copilot: parse response: %w", err)
	}

	response := &ai.Response{
		Usage: ai.Usage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
			TotalTokens:  out.Usage.TotalTokens,
		},
	}
	var callItems []map[string]any
	var text strings.Builder
	for _, item := range out.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
				}
			}
		case "function_call":
			var input map[string]any
			if err := json.Unmarshal([]byte(item.Arguments), &input); err != nil {
				input = map[string]any{}
			}
			response.ToolCalls = append(response.ToolCalls, ai.ToolCall{
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			})
			callItems = append(callItems, map[string]any{
				"type":      "function_call",
				"call_id":   item.CallID,
				"name":      item.Name,
				"arguments": item.Arguments,
			})
		}
		// Other item types (reasoning, …) are ignored.
	}
	response.Reply = text.String()
	return response, callItems, nil
}

// streamResponses streams a /responses request, adapting the typed SSE events
// (response.output_text.delta, response.completed) to the ai.Stream shape.
func (p *Provider) streamResponses(ctx context.Context, req *ai.Request) (ai.Stream, error) {
	body, err := json.Marshal(p.buildResponsesRequest(req, responsesInput(req), nil, true))
	if err != nil {
		return nil, fmt.Errorf("copilot: marshal stream request: %w", err)
	}
	httpReq, err := p.newAPIRequest(ctx, "/responses", body, false)
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
	return &responsesStream{body: httpResp.Body, scanner: bufio.NewScanner(httpResp.Body)}, nil
}

type responsesStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	closed  bool
}

func (s *responsesStream) Recv() (*ai.Response, error) {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue // event: lines, keepalives, blanks
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil, io.EOF
		}
		var ev struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				return &ai.Response{Reply: ev.Delta}, nil
			}
		case "response.completed":
			return &ai.Response{Usage: ai.Usage{
				InputTokens:  ev.Response.Usage.InputTokens,
				OutputTokens: ev.Response.Usage.OutputTokens,
				TotalTokens:  ev.Response.Usage.TotalTokens,
			}}, nil
		case "response.failed", "error":
			return nil, fmt.Errorf("copilot: stream error event: %s", data)
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (s *responsesStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}
