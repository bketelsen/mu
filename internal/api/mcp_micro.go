package api

import (
	"context"
	"net/http"

	gwmcp "go-micro.dev/v6/gateway/mcp"
)

// mcpReqKey threads the originating HTTP request to the resolver so per-tool
// guards and authenticated execution see the real caller.
type mcpReqKey struct{}

// mcpResolver builds a go-micro MCP manual resolver from the registered tools.
// go-micro owns the MCP protocol/transport; mu keeps ownership of which tools
// exist and how they execute — the per-IP guard and authenticated dispatch
// ExecuteTool performs. No framework internals are
// exposed (no store/broker tools).
func mcpResolver() gwmcp.Resolver {
	res := gwmcp.NewManualResolver()
	st := sortedTools()
	for i := range st {
		t := st[i]
		props := map[string]interface{}{}
		var required []string
		for _, p := range t.Params {
			props[p.Name] = map[string]interface{}{"type": p.Type, "description": p.Description}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		schema := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}

		name := t.Name
		res.Add(gwmcp.Tool{Name: name, Description: t.Description, InputSchema: schema},
			func(ctx context.Context, args map[string]interface{}) (*gwmcp.CallResult, error) {
				r, _ := ctx.Value(mcpReqKey{}).(*http.Request)

				// Per-tool pre-check — protocol error.
				if ToolGuard != nil && r != nil {
					if err := ToolGuard(r, name); err != nil {
						return nil, &gwmcp.RPCError{Code: -32000, Message: err.Error()}
					}
				}
				// Tool execution — a tool-level error is an isError result, not
				// a protocol error.
				text, isErr, err := ExecuteTool(r, name, args)
				if err != nil {
					return &gwmcp.CallResult{Text: err.Error(), IsError: true}, nil
				}
				return &gwmcp.CallResult{Text: text, IsError: isErr}, nil
			})
	}
	return res
}

// serveMCP serves a JSON-RPC MCP request through go-micro's gateway/mcp handler.
func serveMCP(w http.ResponseWriter, r *http.Request) {
	handler := gwmcp.NewHandler(mcpResolver(),
		gwmcp.WithServerInfo("mu", "1.0.0"),
		gwmcp.WithProtocolVersion(MCPVersion))
	ctx := context.WithValue(r.Context(), mcpReqKey{}, r)
	handler.ServeHTTP(w, r.WithContext(ctx))
}
