# Agent System

Covers `agent/`, `agent/micro/`, `internal/ai/`, `internal/ai/copilot/`,
and `internal/agents/` (which is *not* part of the agent runtime — see
below).

## Three ways a query gets answered

`agent.Query(accountID, prompt, history...)` (`agent/agent.go:74`) is
the synchronous entry point used by channel bots, the `agent` MCP
tool, and the stream `@micro` reply hook. It delegates to
`QueryWithOpts`, which tries, in order:

1. **Explicit specialist addressing** — `@weather`, "ask the weather
   agent", etc. Strips the address and calls `micro.Orchestrate` for
   that one agent directly (`agent.go:81-85`).
2. **Automatic micro-agent routing** — `micro.Route(prompt)`; any
   non-`micro` (i.e. non-catch-all) result is orchestrated
   (`agent.go:87-93`).
3. **Native go-micro agent** — if `AGENT_NATIVE` isn't disabled and a
   native-capable provider (Copilot or Atlas) is configured,
   `queryNative` handles it with framework-native tool calling
   (`agent.go:95-107`). Falls through to (4) if unavailable/erroring.
4. **Hand-rolled plan → execute → synthesize** — the original fallback
   pipeline (`agent.go:109-280`).

The streaming HTTP handler (`/agent`, `handleQuery`,
`agent/agent.go:719-1118`) implements essentially the same decision
tree but persists a `Flow` and emits SSE progress events (`thinking`,
`tool_start`, `tool_done`, `stream_token`, `response`, `done`).
`/agent/run` (`agent/run.go:103-269`) is the synchronous JSON planner
endpoint.

### Classic plan → execute → synthesize pipeline

In `agent/agent.go`:

- **Plan** (`128-176`): builds a tool catalog + context prompt, asks
  the *background* model for a JSON array of `{tool, args}` calls.
  Common requests can skip the LLM round entirely via
  `shortcutToolCalls` (`1120-1184`) — a deterministic fast path for
  frequent intents.
- **Execute** (`178-217`): dedupes calls, enforces guest tool
  restrictions, invokes `api.ExecuteToolAs`, caps each result at 8 KB,
  and falls back to web search if a news search comes up empty.
- **Synthesize** (`219-280`): formats tool outputs as RAG sections,
  appends prior conversation turns and private user context
  (`UserContextFunc`), then asks the *interactive* model for the final
  answer.
- **Answer guard** (`agent/answer_guard.go:18-52`): `completeToolAnswer`
  rejects empty/progress-only/raw-payload replies and synthesizes a
  deterministic answer straight from the RAG data if the model's
  answer doesn't hold up.

## `agent/micro`: specialized agents

### Agent model and registry

`micro.Agent` (`agent/micro/micro.go:8-19`) holds identity, persona
system prompt, an **allow-list** of MCP tool names it may call, a
`MemoryScope` for `internal/memory`, and optional owner/fork metadata
for user-created custom agents.

Built-ins registered at init (`agent/micro/registry.go:3-84`): `micro`
(catch-all, all tools), `news`, `mail`, `weather`, `blog`, `video`,
`apps`, `search`, `github` — each with its own system prompt, narrow
tool allow-list, and memory scope.

Custom (user-created) agents persist per-account in `user_agents.json`
(`agent/micro/userstore.go:41-80`: `SaveUserAgent`, `GetUserAgentFor`,
`UserAgentsFor`) and are resolved via an injected `UserAgentResolver`
rather than living in the global built-in registry.

### Routing

`Route` (`agent/micro/router.go:13-27`) tries three stages in order:

1. Direct `@agent` addressing.
2. Deterministic keyword routing (`:99-155`) — recognizes GitHub/repo
   language, mail/blog terms, weather/news/video/search/apps signals;
   can select up to three specialists; uses token-boundary matching to
   avoid substring false positives (`:178-221`).
3. LLM classification for ambiguous requests (`:223-280`) — sends
   built-in agent descriptions to the background model, expects JSON
   IDs back, validates against the registry, and defaults to
   `["micro"]` on any failure.

### Execution and orchestration

`(*Agent).Execute` (`agent/micro/execute.go`) is another plan →
execute → synthesize loop, but scoped to one agent's tools:

- Builds planner tool descriptions from the live `internal/api`
  registry (`:32-74`, `:132-165`).
- Adds private aggregate user context plus memory scoped to
  `Agent.MemoryScope` via `memory.ForScopedContext` (`:36-56`).
- Rejects any planner-selected tool outside the agent's allow-list and
  applies guest restrictions (`:83-101`).
- Invokes tools as the calling account and synthesizes with the
  specialist's persona (`:103-129`).

`Orchestrate` (`:211-237`) either runs one specialist directly or spins
up several concurrently for multi-specialist routes; successful
answers from multiple specialists are merged by an additional LLM call
using each answer as RAG input (`:239-275`). If every specialist
fails, it falls back to the general `micro` agent.

## Native go-micro agent path

`agent/native.go` replaces hand-written planning with the go-micro
framework's native tool-calling agent:

- `nativeEnabled` — on unless `AGENT_NATIVE` is falsey; streaming
  gated separately by `AGENT_NATIVE_STREAM` (`:25-61`).
- `nativeProvider` — prefers configured Copilot, then Atlas/DeepSeek;
  no native-capable provider ⇒ fall back to the classic planner
  (`:158-171`).
- `buildNativeAgent` (`:173-229`) composes a fresh go-micro agent
  (`internal/service.NewAgent`) with a selected set of domain
  *services* (not individual tool names), account injection, tool-call
  dedup, a max of six steps, and optional custom persona/service
  allow-list.
- `injectAccount` (`:100-123`) forcibly overwrites/removes
  `account_id` on every tool call the model attempts — this is the
  mitigation against prompt injection (via model output or fetched
  tool content) trying to act as a different account.
- `queryNative`/`streamNative` (`:236-325`) call `a.Ask` /
  `a.StreamAsk` and adapt events into the same tool/token hooks the
  streaming HTTP handler uses.

Key distinction: the classic/micro-agent paths call **MCP tools** by
name through `internal/api`; the native path discovers and calls
**go-micro services** directly (weather, news, search, …), which is a
narrower, framework-native surface.

## `internal/ai`: LLM abstraction

`ai.Prompt` (`internal/ai/ai.go:33-45`) is the central request struct:
system prompt, RAG, history, question, priority, optional
provider/model override, caller attribution, max tokens. `Ask` and
`AskStream` (`:103-112`) are the two entry points.

### Model & provider selection (`internal/ai/providers.go`)

- `DefaultModel` (interactive): Copilot chat model if Copilot
  configured; else configured Anthropic model or Claude Sonnet
  (`:105-116`).
- `PremiumModel`: Copilot premium/chat model, configured Anthropic
  premium model, or Claude Opus (`:118-131`).
- `BackgroundModel` (planner/classification/cheap work): Copilot's
  `gpt-4.1`; else Atlas DeepSeek Flash if Atlas configured; else Claude
  Haiku (`:133-143`).
- `Configured` (`:70-87`) accepts Copilot, Anthropic, Atlas/OpenAI API
  credentials, a configured OpenAI-compatible endpoint, or auto-
  detected local Ollama.
- Concurrency: 5 simultaneous LLM requests, 120s timeout (`:17-21`).
  Background-model callers cap output at 512 tokens
  (`internal/ai/micro.go:79-82`).

All generation is dispatched through go-micro
(`internal/ai/micro.go`): `resolveProvider(model)` (`:20-47`) picks
Atlas Cloud for DeepSeek/Qwen-style model IDs when keyed, else Copilot
if `COPILOT_GITHUB_TOKEN` exists, else Anthropic if keyed, else an
OpenAI-compatible endpoint (including auto-detected local Ollama at
`localhost:11434/v1`). `generateViaMicro`/`streamViaMicro` (`:73-132`,
`:138-224`) build a `gmai.New(provider, apiKey, model, baseURL,
tokenCap)` and record usage.

Note: `Prompt.Provider` is documented as an override but the current
`generate`/`generateStream` route by resolved *model* + configured
credentials rather than reading that field directly
(`providers.go:155-201`, `:203-244`) — worth checking before relying on
it to force a provider.

### `internal/ai/copilot`

A custom go-micro AI provider adapter for GitHub Copilot
(`internal/ai/copilot/copilot.go:24-29`):

- Accepts the long-lived GitHub OAuth token as its "API key", exchanges
  it for a short-lived Copilot bearer token + subscription-specific API
  base, caching sessions until ~2 minutes before expiry
  (`auth.go:54-112`).
- Supports GitHub device-code OAuth (`StartDeviceFlow` /
  `WaitForDeviceToken`, `:124-218`).
- Fetches/caches the model catalog to pick the right wire format
  (`:220-315`).
- Supports both Copilot's OpenAI-compatible `/chat/completions` and the
  newer `/responses` API, with runtime fallback on
  `unsupported_api_for_model` (`copilot.go:65-88`;
  `responses.go:29-59`), and bounded tool-call loops (max 8 rounds).

Copilot is both an LLM gateway for `claude-*`/`gpt-*` model names and
the preferred provider for the native agent path when configured. See
`docs/COPILOT.md` for operator setup (device flow, cost notes, image
generation limitation).

## Conversation/state structures

- `agent.QueryMessage{Role, Text}` / `agent.QueryOpts{History, Public,
  System, Tools}` — caller-supplied turn history and guest/custom-agent
  constraints (`agent/agent.go:59-71`).
- `agent.Flow` — persisted turn state: account, prompt, `FlowStep`s,
  markdown/HTML answer, status/error, selected custom agent, parent
  turn, timestamp (`agent/flows.go:13-33`). In-memory with JSON
  persistence, per-user eviction at 200 flows, mutex-protected.
- `ParentID` chains turns into conversations; `Session` derives a
  stable root/head/title/turn-count; `getConversationHistory` walks
  parents for chronological turns (`flows.go:162-285`).
- `micro.Agent.MemoryScope` scopes `internal/memory` reads during
  specialist execution (`agent/micro/execute.go:41-45`).
- `UserContextFunc`, wired once in `main.go:297-314`, injects private
  cross-agent context (unread mail count, remembered
  preferences/notes) into both the main agent and every micro-agent.

## Not part of the agent runtime

`internal/agents/agents.go` (`Handler`, `:10-42`) only renders the
owner-facing `/agents` integrations landing page (links to MCP docs,
REST docs, PAT creation) — do not confuse it with `agent/` or
`agent/micro/`.
