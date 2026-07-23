# Tool Registry, MCP, and A2A

Covers `internal/api` (the tool registry, MCP page/handler), `internal/a2a`,
and how `main.go` wires domain capabilities in as tools.

## `internal/api`: the canonical tool registry

`internal/api` is the single catalog of everything an agent, MCP
client, or CLI command can invoke. It's read by: MCP (`/mcp`), the
main agent planner and `agent/micro` executor, the `/api` docs page,
and `internal/cli`.

### `api.Tool`

Defined at `internal/api/mcp.go:103-116`:

- `Name`, `Aliases` — canonical + alternate invocation names.
- `Description`, `Params []ToolParam` — used to build both the MCP
  JSON schema and the planner's tool catalog prompt.
- `Method`/`Path` — optional: dispatches as an internal authenticated
  HTTP request to that route via `http.DefaultServeMux`. Only used as
  a *fallback* when the tool has neither `HandleAuth` nor `Handle` set
  — a tool can define both a direct handler and `Method`/`Path`
  (e.g. `apps_read`), in which case the handler always wins.
- `Handle func(args map[string]any) (string, error)` — direct handler,
  no account context.
- via `RegisterToolWithAuth`, a second handler form receives
  `accountID` bound server-side from the session — this is the only
  safe form for anything ownership-sensitive.

### Registration

- `RegisterTool` (`:161`) / `RegisterToolWithAuth` (`:166`) append to
  the runtime registry. Static built-in tools live directly in the
  `tools` slice (`:225-391`).
- `main.go:414-945` is where most domain-specific tools actually get
  registered, *after* `service.Load()` calls run. Pattern: most
  handlers call `service.Call(ctx, "<domain>", "Server.<Method>", req,
  &rsp)` to invoke the in-process go-micro service, then return
  `rsp.Text` (a model-ready formatted string) rather than raw
  structured data — tools are designed to be "AI-first": the RPC
  response is already synthesized into text a model can quote/act on.
- `api.SetCard(toolName, title, renderFunc)` (`internal/api/card.go:19`)
  attaches a visual dashboard card renderer to a tool so an agent
  answer can render a rich card for it. Note: the home dashboard does
  *not* consume this registry — `home/home.go` independently maps
  `home/cards.json` entries straight to domain renderers, so a card
  registered only via `api.SetCard` shows up in agent answers but not
  automatically on the home screen.

### Invocation

- `ExecuteToolAs` (`mcp.go:393-404`) creates a temporary account
  session for non-HTTP/background callers (e.g. the agent planner).
- `ExecuteTool` (`:406-480`) resolves aliases, requires a session, and
  dispatches in strict precedence order: `HandleAuth` first (if set),
  else `Handle` (if set), else the registered `Method`+`Path` as an
  internal authenticated HTTP request. A tool with both a direct
  handler and `Method`/`Path` always runs the handler.
- Every tool call — whether from MCP, the planner, or the CLI — goes
  through this same authorization boundary. There is no path that
  lets a model or client argument choose which account a tool acts as;
  `RegisterToolWithAuth` handlers only ever see the session-bound
  `accountID`.

### `ToolDescriptions()`

`:201-223` produces the live, current tool catalog text used to build
planner prompts in `agent/agent.go` and `agent/micro/execute.go` — new
tools registered anywhere automatically become available to both the
classic planner and every micro-agent (subject to that agent's tool
allow-list).

## MCP endpoint

- `GET /mcp` renders an HTML tool-browsing page
  (`internal/api/mcp_page.go:13-32`).
- `POST /mcp` is JSON-RPC, delegated to a go-micro MCP gateway/manual
  resolver (`internal/api/mcp_micro.go:19-67`) that derives JSON
  schemas from the registered `ToolParam`s and executes through Mu's
  own authenticated `ExecuteTool` dispatcher — so MCP clients get
  exactly the same tool set and auth boundary as the agent.
- Auth: PAT or session bearer token (see `auth-and-identity.md` and
  `docs/MCP.md`). OAuth 2.1 (dynamic client registration + PKCE) is
  also supported for MCP clients that need it
  (`internal/auth/oauth.go`).
- `MCP_GATEWAY_ADDR` optionally starts a second, separate go-micro MCP
  gateway on its own port that auto-exposes every registered go-micro
  *service* (not just registry tools) as MCP — additive, doesn't
  change `/mcp` (`main.go:359-366`).

## A2A (Agent2Agent protocol)

`internal/a2a` implements a Google A2A-style discovery + task
JSON-RPC façade on top of `agent/micro`:

- Types: `AgentCard`, `Task`, `Message`, `Artifact`
  (`internal/a2a/a2a.go:29-130`).
- `AgentCardHandler` (`:161`) serves `/.well-known/agent.json`; its
  advertised skills come from `micro.All()`, which is only the
  built-in agent registry (`agent/micro/micro.go:50-58`) — it excludes
  both the `micro` catch-all fallback and any per-owner custom agents
  stored via `agent/micro/userstore.go`.
- `Handler` (`:169`) serves `/a2a` JSON-RPC. `SendMessage` routes the
  prompt through `micro.Route` then synchronously calls
  `micro.Orchestrate` (`:208-304`) — A2A shares the exact same routing/
  execution machinery as the main agent, just a different transport.
- Tasks are in-memory only, capped and pruned; there's no streaming.
  Cancellation only flips stored task state — it does not interrupt an
  already-running orchestration (`:375` and around).
- `a2a.BaseURL` is set from `MU_DOMAIN` in `main.go:1019-1026`.

## Registering a new tool — checklist

1. Decide if it needs account context. If yes, use
   `api.RegisterToolWithAuth`; if it's safe for anonymous/public use
   (e.g. `web_search`, `weather_forecast`), use `api.RegisterTool`.
2. If the capability lives in a go-micro service, register/call it via
   `service.Register`/`service.Call` rather than importing the package
   directly and calling a Go function — this keeps the native agent
   path (which discovers services, not tool names) and the MCP gateway
   path (`MCP_GATEWAY_ADDR`) both able to see it.
3. Return model-ready text from the handler (a formatted string), not
   raw JSON, unless the tool is explicitly a structured "get me a
   record" type call (e.g. `db_get`, `apps_read`).
4. If the tool should render a home/agent-answer card, wire it with
   `api.SetCard`.
5. Add it to the relevant `agent/micro` built-in agent's tool
   allow-list (`agent/micro/registry.go`) if a specialist should be
   able to use it; the catch-all `micro` agent gets every tool
   automatically.
