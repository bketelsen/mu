# Remove Markets Capability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove Mu's dedicated live-markets capability from the binary, agents, user interfaces, clients, SDK, examples, and documentation while preserving generic finance and market language in news, search, and editorial content.

**Architecture:** Delete the `markets` service at its package boundary, then remove each consumer rather than replacing it with compatibility code or synthetic data. Existing `/markets` and market-tool calls disappear through normal not-found and unknown-tool behavior; stale persisted home-card IDs remain inert and require no migration.

**Tech Stack:** Go, go-micro, `net/http`, embedded HTML/CSS/JavaScript, JSON configuration, Discord and Telegram HTTP APIs.

## Global Constraints

- Do not add redirects, deprecation handlers, compatibility tools, or replacement market workflows.
- Preserve ordinary finance and market language when it belongs to news, web search, payments, or editorial content.
- Preserve stale `markets` strings in persisted `Account.HomeCards` data; unknown card IDs are ignored naturally.
- Do not remove wallet cryptocurrency payment support or x402 documentation.
- The final repository must contain no `markets/` package or market-specific image asset.
- Use `go test ./... -short` and `go build ./...` as the completion gates.

---

### Task 1: Remove the Service and Runtime Boundary

**Files:**
- Create: `internal/api/api_test.go`
- Modify: `main.go:47,141-143,184-209,764-781,915-924,1098-1107,1165-1226,1380-1382,2002-2038`
- Modify: `internal/api/api.go:637-666,759-786`
- Modify: `internal/api/mcp.go:295-297,410-416`
- Modify: `internal/api/mcp_test.go:130-132`
- Modify: `admin/diagnostics.go:9-19,103-134,197-213`
- Modify: `home/home.go:14-27,119-131,467-470`
- Modify: `news/digest/digest.go:1-18,301-362`
- Modify: `blog/opinion.go:1-16,307-397`
- Modify: `internal/data/data.go:203-212`
- Modify: `stream/stream.go:22-32,86-91`
- Modify: `stream/handlers.go:203-215`
- Modify: `go.mod`
- Modify: `go.sum`
- Delete: `markets/agent.go`
- Delete: `markets/agent_test.go`
- Delete: `markets/main_test.go`
- Delete: `markets/markets.go`
- Delete: `markets/markets_test.go`
- Delete: `markets/service.go`
- Delete: `markets/service_test.go`
- Delete: `markets/snapshot_test.go`

**Interfaces:**
- Consumes: Existing `api.Endpoints`, dynamic tool registration in `main`, and standard HTTP not-found behavior.
- Produces: A binary with no live-price service, market route, market REST schema, market MCP/native tool, market card registration, market diagnostic, or market health check.

- [ ] **Step 1: Add a failing REST catalogue regression test**

Create `internal/api/api_test.go`:

```go
package api

import "testing"

func TestEndpointsExcludeMarkets(t *testing.T) {
	for _, endpoint := range Endpoints {
		if endpoint.Path == "/markets" || endpoint.Name == "Markets" {
			t.Fatalf("removed Markets endpoint is still advertised: %#v", endpoint)
		}
	}
}
```

- [ ] **Step 2: Run the test and verify the old endpoint is detected**

Run: `go test ./internal/api -run TestEndpointsExcludeMarkets -count=1`

Expected: FAIL with `removed Markets endpoint is still advertised`.

- [ ] **Step 3: Remove runtime registration and direct service consumers**

In `main.go`, remove:

- the `"mu/markets"` import and `markets.Load()` call;
- the `markets.TopMovers(3)` context injection;
- the `markets_list` dynamic tool and `markets` alias registration;
- `api.SetCard("markets_list", "Markets", markets.MarketsHTML)`;
- market-specific tool-description text;
- the `"/markets"` public-route entry and `http.HandleFunc("/markets", markets.Handler)`;
- the Markets status check using `markets.GetAllPrices()`.

Do not add a replacement handler. Update adjacent comments and lists so they describe only remaining services.

Remove all remaining direct `mu/markets` consumers before deleting the package: remove the import, `cardFunctions` entry, and `TopMovers` suggestion from `home/home.go`; remove the import and `GetAllPriceData` context block from `news/digest/digest.go`; remove the import and price-data context block from `blog/opinion.go`. Leave prompt/copy cleanup in Task 5, but ensure these composition layers simply omit the removed section and continue with their remaining inputs.

In `admin/diagnostics.go`, remove the import, diagnostic-list entry, and full `checkMarkets` function. In `internal/api/api.go`, delete the complete `Endpoint{Name: "Markets", Path: "/markets"}` block and remove Markets from Agent/MCP prose. In `internal/api/mcp.go` and its test, remove comments claiming Markets is dynamically registered and remove Markets from stream event wording. Delete the unused `TypeMarket` constant and its rendering branch from `stream/stream.go` and `stream/handlers.go`, preserving system and news event rendering. Change the indexed-data comment to:

```go
Type string `json:"type"` // "news", "video"
```

- [ ] **Step 4: Delete the markets package and remove its dependency**

Delete all eight files listed above, then run:

```bash
go mod tidy
```

Expected: `github.com/piquette/finance-go` and its local replacement disappear from `go.mod`; obsolete finance-go checksums disappear from `go.sum`.

- [ ] **Step 5: Run focused runtime tests**

Run:

```bash
go test ./internal/api ./admin ./internal/data -short
go test ./... -run '^$'
```

Expected: PASS; the second command compiles all packages without `mu/markets` imports.

- [ ] **Step 6: Commit the runtime removal**

```bash
git add main.go internal/api internal/data admin home/home.go news/digest/digest.go blog/opinion.go stream go.mod go.sum markets
git commit -m "remove markets service runtime"
```

---

### Task 2: Remove Dedicated Agent Behavior

**Files:**
- Modify: `agent/micro/registry.go:3-93`
- Modify: `agent/micro/router.go:29-59,128-159`
- Modify: `agent/micro/router_test.go:8-171`
- Modify: `agent/micro/execute.go:167-189`
- Modify: `agent/micro/execute_test.go:1-40`
- Modify: `agent/micro/micro.go:1-18`
- Modify: `agent/guest.go:10-32`
- Modify: `agent/native.go:63-76,182-186,400-464`
- Modify: `agent/native_test.go:1-20`
- Modify: `agent/agent.go:165-169,671-750,900-984,1028-1070,1100-1122,1178-1267,1365-1477,1489-1519,1842-1960`
- Modify: `agent/agent_test.go`
- Modify: `agent/answer_guard.go:551-574,981-993,1060-1072`
- Modify: `agent/run.go:66-70,152-156,214-230`

**Interfaces:**
- Consumes: The remaining micro-agent registry, generic LLM routing, news tools, web-search tools, and weather fast path.
- Produces: No Markets agent, market tool permission, market keyword route, price shortcut, market-mover fast path, market result formatter, or live-price claim.

- [ ] **Step 1: Add failing registry and routing assertions**

Add to `agent/micro/router_test.go`:

```go
func TestMarketsCapabilityIsNotRegistered(t *testing.T) {
	if agent := Get("markets"); agent != nil {
		t.Fatalf("removed Markets agent is still registered: %#v", agent)
	}
	for _, agent := range All() {
		if agent.ID == "markets" {
			t.Fatalf("All() exposed removed Markets agent: %#v", agent)
		}
	}
}

func TestKeywordRouteDoesNotSpecialCaseMarketQuestions(t *testing.T) {
	if got := keywordRoute("what is the BTC price?"); len(got) != 0 {
		t.Fatalf("keywordRoute() = %v, want generic routing", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify both dedicated paths remain**

Run: `go test ./agent/micro -run 'TestMarketsCapabilityIsNotRegistered|TestKeywordRouteDoesNotSpecialCaseMarketQuestions' -count=1`

Expected: FAIL because the Markets agent is registered and BTC is keyword-routed to it.

- [ ] **Step 3: Remove the micro-agent and permission entries**

Delete the Markets `Register(&Agent{...})` block from `registry.go`. Rewrite the two affected prompts as:

```go
SystemPrompt: `You are Micro, a personal AI assistant. You have access to all tools and can help with anything — news, weather, mail, search, places, apps, and more. Be concise, direct, and helpful. Use markdown.`,
```

```go
SystemPrompt: `You are the Apps specialist on Mu. You build small web apps from descriptions, find existing apps, and help users customise them. The app SDK supports mu.ai() for AI-powered apps, mu.store for persistence, and typed helpers such as mu.news for live data. Generate clean, working HTML.`,
```

Remove the market keyword detection/append branch from `router.go`; direct addressing will then resolve only registered agents. Remove `markets` from both guest permission maps, native service lists, native title/label switches, and micro package examples/comments.

Update `router_test.go` by replacing market-address fixtures with existing weather/news fixtures, changing multi-signal expected output to `[]string{"weather", "news"}`, changing validation input to `[]string{"bogus", "news", "news", "weather", "mail"}` with expected `[]string{"news", "weather", "mail"}`, and retaining the two new absence tests. Remove Markets from `execute_test.go`. Replace `markets_Get` in `native_test.go` with an existing parameterized service such as `weather_Forecast` while preserving the dedupe-key behavior under test.

- [ ] **Step 4: Remove planner, shortcut, fallback, and formatting logic**

In `agent/agent.go`:

- remove Markets from tool dropdowns and both tool descriptions;
- remove market-specific planner examples and synthesis rules;
- remove `hasMarketsTool` and its argument from `useFastToolFallback`;
- delete all market aliases and BTC/ETH/mover entries from `shortcutToolCalls`;
- delete `isMarketMoverPrompt`, `wantsMarketMoverExplanation`, `skipMarketMoverCompanionTool`, `formatMarketsResult`, and `formatMarketPrice`;
- remove call sites for companion suppression and the Markets branches in `toolLabel` and `formatToolResult`;
- preserve the weather fast path and generic finance/news reasoning.

Delete the corresponding market-only tests from `agent/agent_test.go`: result formatting, price shortcuts, guest market fallback, and companion suppression. Where a generic test uses Markets only as arbitrary fixture data, replace it with an extant tool instead of deleting the generic behavior test.

In `answer_guard.go`, remove Markets from `readableToolName`, remove the `Live crypto prices:` result shape, and change the fallback sentence to:

```go
return "I checked the live sources, but couldn't produce a complete final answer from the available tool results. Please try again in a moment; if one source is unavailable, ask for the specific slice (news, weather, or search) and I'll show what is reachable."
```

In `agent/run.go`, remove live-market state and market-mover instructions from comments and prompts while retaining generic market subject matter when grounded in news/search.

- [ ] **Step 5: Run agent tests**

Run:

```bash
go test ./agent ./agent/micro -short
```

Expected: PASS with no Markets agent or dedicated market route.

- [ ] **Step 6: Commit the agent removal**

```bash
git add agent
git commit -m "remove markets agent behavior"
```

---

### Task 3: Remove Home UI, Assets, SDK, and Built-In Apps

**Files:**
- Modify: `home/cards.json:20-28`
- Modify: `home/home.go:503-507,597-603`
- Modify: `home/home_test.go`
- Modify: `internal/app/app.go:315-332,363-369,938-945,1010-1016`
- Modify: `internal/app/html/mu.js:4-26,1421-1427`
- Modify: `internal/app/html/mu.css:2023-2099,2496-2504,4476-4656`
- Modify: `internal/auth/homecards_test.go:5-31`
- Delete: `internal/app/html/markets.svg`
- Delete: `internal/app/html/markets.png`
- Modify: `apps/apps.go:1119-1154`
- Modify: `apps/static/sdk.js:13-32`
- Modify: `apps/test.go:21-28,82-126,179-201`
- Modify: `apps/templates.go:12-143,708-761,856-916,1138-1329`
- Create: `apps/templates_test.go`

**Interfaces:**
- Consumes: Embedded `home/cards.json`, `auth.Account` card-selection semantics, the remaining app SDK helpers, and built-in template registry.
- Produces: No Markets card, navigation item, preference, icon, CSS/JS behavior, SDK function, or market-dependent starter app. Legacy persisted card IDs remain harmless.

- [ ] **Step 1: Add failing card and template tests**

Append to `home/home_test.go`:

```go
func TestCardConfigExcludesMarkets(t *testing.T) {
	b, err := f.ReadFile("cards.json")
	if err != nil {
		t.Fatal(err)
	}
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	for _, card := range append(config.Left, config.Right...) {
		if card.ID == "markets" || card.Type == "markets" || card.Link == "/markets" {
			t.Fatalf("removed Markets card remains configured: %#v", card)
		}
	}
}
```

Add `"encoding/json"` to that test's imports. Create `apps/templates_test.go`:

```go
package apps

import (
	"strings"
	"testing"
)

func TestTemplatesExcludeMarketsCapability(t *testing.T) {
	if template := GetTemplate("markets"); template != nil {
		t.Fatalf("removed Markets template remains: %#v", template)
	}
	if template := GetTemplate("portfolio"); template != nil {
		t.Fatalf("market-dependent Portfolio template remains: %#v", template)
	}
	for _, template := range Templates {
		if strings.Contains(template.HTML, "mu.markets") {
			t.Fatalf("template %q calls removed mu.markets API", template.ID)
		}
	}
}
```

- [ ] **Step 2: Run tests and verify UI/app exposure is detected**

Run: `go test ./home ./apps -run 'TestCardConfigExcludesMarkets|TestTemplatesExcludeMarketsCapability' -count=1`

Expected: FAIL for the configured Markets card and Markets template.

- [ ] **Step 3: Remove home and shell exposure**

Delete the Markets object from `home/cards.json`. The import, card function-map entry, and `TopMovers` suggestion were removed in Task 1; now remove the Markets preference entry and tooltip from `home/home.go`. In `internal/app/app.go`, remove the Markets nav link and both card preference/universe entries; do not add migration code.

Remove `/markets.svg` from `STATIC_CACHE` and the Markets entry from `availableCards` in `mu.js`; increment its `VERSION` by one so installed service workers discard old caches. Delete both market assets.

Delete dedicated market ticker/page/table/card CSS. Preserve shared selectors by changing:

```css
#markets, #video {
```

to:

```css
#video {
```

Remove `#markets { order: 1; }` and renumber the remaining explicit mobile card orders from 1 without gaps.

- [ ] **Step 4: Preserve and test inert persisted card preferences**

Keep `"markets": true` in `preHomeCardsSeen`; it documents a previously offered card and prevents stale data from being mistaken for a newly introduced card. Keep at least one `HomeCards`/`HomeCardsSeen` fixture containing `markets`, and add this case to `TestShowHomeCard`:

```go
{"stale markets id does not affect known cards",
	Account{HomeCards: []string{"markets", "blog"}, HomeCardsSeen: []string{"blog", "news", "markets", "social", "video", "images", "mail", "web"}}, "blog", true},
```

The rendering regression is covered by `TestCardConfigExcludesMarkets`: no remaining card calls `ShowHomeCard("markets")`.

- [ ] **Step 5: Remove the app SDK API and market-dependent examples**

Delete `markets:function(...)` from both `apps/apps.go` and `apps/static/sdk.js`. Remove the market regex/path extraction and market response-shape validation from `apps/test.go`.

In `apps/templates.go`:

- delete the `markets` and `portfolio` registry entries and their complete HTML constants;
- change Dashboard to News + Weather and remove its market panel/call;
- change its description to `"News + weather in a single view"`;
- rework the Mu App example to use news categories/headlines rather than crypto/futures tabs and `mu.markets` calls.

The resulting templates must contain no `/markets`, `mu.markets`, live-price field, crypto-price panel, or futures-price panel.

- [ ] **Step 6: Run focused UI and app tests**

Run:

```bash
go test ./home ./internal/app ./internal/auth ./apps -short
```

Expected: PASS.

- [ ] **Step 7: Commit the UI and SDK removal**

```bash
git add home internal/app internal/auth apps
git commit -m "remove markets UI and app SDK"
```

---

### Task 4: Remove Client Commands and Legacy Chat Price Lookups

**Files:**
- Modify: `chat/chat.go:173-253`
- Modify: `chat/chat_test.go:8-38`
- Modify: `client/discord/rich.go:124-146`
- Modify: `client/discord/interactions.go:96-108`
- Modify: `client/discord/discord.go:313-315`
- Create: `client/discord/rich_test.go`
- Modify: `client/telegram/telegram.go:65-82,160-193`
- Modify: `home/chat_test.go:8-24`
- Modify: `internal/app/chat.go:137`

**Interfaces:**
- Consumes: Existing Discord slash-command registry, Telegram generic `/ask`, news/weather commands, and ordinary agent handling.
- Produces: No `/markets` chat command or direct `market_<symbol>` indexed-price lookup; finance questions may still reach generic agent/news/search behavior.

- [ ] **Step 1: Add a failing Discord command regression test**

Create `client/discord/rich_test.go`:

```go
package discord

import "testing"

func TestSlashCommandsExcludeMarkets(t *testing.T) {
	for _, command := range slashCommands {
		if command.Name == "markets" {
			t.Fatalf("removed /markets command remains registered: %#v", command)
		}
	}
}
```

- [ ] **Step 2: Run the test and verify the command remains**

Run: `go test ./client/discord -run TestSlashCommandsExcludeMarkets -count=1`

Expected: FAIL with `removed /markets command remains registered`.

- [ ] **Step 3: Remove Discord and Telegram commands**

Delete the Markets object from `slashCommands` and the `case "markets"` interaction branch. Remove Markets from Discord capability copy. Preserve the generic green embed-color branch for finance-related prose because it does not expose a service.

Delete Telegram's command metadata and `/markets` switch branch; update `/start` copy to:

```go
sendTelegram(token, chatID, "Hi! I'm Micro — your agent across news, mail, weather, search and more. Ask me anything.\n\nIn groups, use /ask followed by your question.")
```

- [ ] **Step 4: Remove the legacy direct price matcher and stale suggestions**

Delete the complete price-pattern block in `chat.handlePatternMatch` that reads `market_<symbol>` entries. Delete `TestHandlePatternMatchRecognizesKnownPricePromptsWithoutData`; retain unrelated pattern matching tests. Remove `"What is moving in markets?"` from `home/chat_test.go` and replace the inline suggestion in `internal/app/chat.go` with:

```js
'Search for nearby coffee shops'
```

- [ ] **Step 5: Run client and chat tests**

Run:

```bash
go test ./chat ./client/discord ./client/telegram ./home ./internal/app -short
```

Expected: PASS.

- [ ] **Step 6: Commit client removal**

```bash
git add chat client home/chat_test.go internal/app/chat.go
git commit -m "remove markets client commands"
```

---

### Task 5: Remove Live Prices from Composition and Product Prompts

**Files:**
- Modify: `news/digest/digest.go:1-18,245-275,301-362`
- Modify: `blog/opinion.go:1-16,219-250,307-397`
- Modify: `internal/ai/ai.go:54-90`
- Modify: `internal/ai/ai_test.go:1-55`
- Modify: `internal/a2a/a2a.go:65-89`
- Modify: `internal/agents/agents.go:21-28`
- Modify: `home/landing.go:13-20,45-50`
- Modify: `home/pricing.go:20-42,74-88`
- Modify: `blog/notes.go:43-58`
- Modify: `blog/notes.json`
- Modify: `blog/seed.go:39-50`

**Interfaces:**
- Consumes: News, video, search, weather, and existing AI context inputs; Task 1 has already removed live-price data blocks.
- Produces: Digests, opinions, landing copy, A2A metadata, and generated editorial prompts that do not claim live market prices.

- [ ] **Step 1: Update AI prompt tests to reject live-market context claims**

In `internal/ai/ai_test.go`, replace market-price RAG fixtures with news fixtures such as:

```go
Rag: []string{"### news\nA current headline with source context."},
```

and add this assertion to the test that inspects the rendered prompt:

```go
if strings.Contains(got, "live market data") {
	t.Fatalf("prompt still claims removed live market data: %s", got)
}
```

- [ ] **Step 2: Run the focused AI test and verify the old prompt claim fails**

Run: `go test ./internal/ai -count=1`

Expected: FAIL because the prompt still contains `live market data`.

- [ ] **Step 3: Align composition prompts with their remaining inputs**

In `news/digest/digest.go`, rewrite supplied-input claims to say news headlines and video content; retain generic market analysis instructions only where source headlines can support them. Confirm the live-price block removed in Task 1 has not been replaced.

In `blog/opinion.go`, rewrite comments that promise “supporting market data”; preserve general editorial language about market effects where evidence can come from news/search. Confirm the live-price block removed in Task 1 has not been replaced.

In `internal/ai/ai.go`, replace both occurrences of:

```text
Current context (live market data, recent news, or articles fetched now):
```

with:

```text
Current context (recent news, search results, or articles fetched now):
```

- [ ] **Step 4: Remove capability claims from generated product copy**

Remove Markets/live-price claims from A2A and internal agent descriptions, landing and pricing pages, blog generation facts, notes, and seed posts. Delete the `Market answers without the detour` note from `blog/notes.json`. Replace typed SDK examples with existing helpers such as `mu.weather`, `mu.news`, and `mu.video`; replace live-market agent examples with current-news or place-search examples.

Do not remove `chat/prompts.json` Crypto/Finance editorial prompts, news finance classifications, blog opinion-memory market subject matter, or wallet/x402 cryptocurrency references.

- [ ] **Step 5: Run composition and copy tests**

Run:

```bash
go test ./news/digest ./blog ./internal/ai ./internal/a2a ./internal/agents ./home -short
```

Expected: PASS.

- [ ] **Step 6: Commit composition changes**

```bash
git add news/digest blog internal/ai internal/a2a internal/agents home/landing.go home/pricing.go
git commit -m "remove live markets composition inputs"
```

---

### Task 6: Update Documentation and Perform the Final Audit

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/ABOUT.md`
- Modify: `docs/APPS.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/CLI.md`
- Modify: `docs/DATA_PUBLISHING_ARCHITECTURE.md`
- Modify: `docs/DESIGN_SYSTEM.md`
- Modify: `docs/DISCORD.md`
- Modify: `docs/GO_MICRO_ARCHITECTURE.md`
- Modify: `docs/MCP.md`
- Modify: `docs/MIGRATION.md`
- Modify: `docs/SERVICES.md`
- Modify: `docs/SYSTEM_DESIGN.md`
- Modify: `docs/TELEGRAM.md`
- Modify: `docs/VISION.md`
- Modify: `docs/WHITEPAPER.md`
- Modify: `docs/docs.go:102-107`

**Interfaces:**
- Consumes: The completed runtime and UI behavior from Tasks 1-5.
- Produces: Current documentation that describes only shipped capabilities and a verified repository with no dedicated Markets remnants.

- [ ] **Step 1: Remove dedicated capability claims and examples**

Across the listed files:

- remove Markets from service, tool, route, card, command, and architecture lists;
- delete dedicated Markets sections, `/markets` and `markets_list` rows, Yahoo/CoinGecko provider claims, and `mu.markets` examples;
- update service counts after removing Markets;
- replace BTC/live-price examples with supported news, weather, places, or search examples;
- update composition diagrams and trees so `home`, digest, and opinion no longer import Markets;
- reframe snapshot architecture examples around an extant service such as News;
- preserve generic financial-news discussion and wallet/x402 cryptocurrency payment documentation.

Do not edit `docs/superpowers/specs/2026-07-21-remove-markets-design.md`; it is the historical contract for this removal.

- [ ] **Step 2: Format changed source files**

Run:

```bash
gofmt -w main.go admin/diagnostics.go internal/api/api.go internal/api/api_test.go internal/api/mcp.go internal/api/mcp_test.go internal/data/data.go stream/stream.go stream/handlers.go agent/agent.go agent/agent_test.go agent/answer_guard.go agent/guest.go agent/native.go agent/native_test.go agent/run.go agent/micro/execute.go agent/micro/execute_test.go agent/micro/micro.go agent/micro/registry.go agent/micro/router.go agent/micro/router_test.go chat/chat.go chat/chat_test.go client/discord/discord.go client/discord/interactions.go client/discord/rich.go client/discord/rich_test.go client/telegram/telegram.go home/home.go home/home_test.go home/chat_test.go home/landing.go home/pricing.go internal/app/app.go internal/app/chat.go internal/auth/homecards_test.go apps/apps.go apps/test.go apps/templates.go apps/templates_test.go blog/notes.go blog/opinion.go blog/seed.go news/digest/digest.go internal/ai/ai.go internal/ai/ai_test.go internal/a2a/a2a.go internal/agents/agents.go docs/docs.go
```

Expected: command exits 0. Review `git diff --stat` to ensure formatting did not touch unrelated generated or vendored files.

- [ ] **Step 3: Run dedicated-reference audits**

Run:

```bash
test ! -d markets
test ! -e internal/app/html/markets.svg
test ! -e internal/app/html/markets.png
rg -n '"mu/markets"|markets\.|markets_list|/markets\b|mu\.markets|market_[A-Za-z<]|Markets Agent|MarketsHTML|TopMovers|GetAllPriceData' . --glob '!docs/superpowers/specs/2026-07-21-remove-markets-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-markets.md'
rg -n 'piquette/finance-go|psanford/finance-go' go.mod go.sum
```

Expected: both `test` commands exit 0; both `rg` commands exit 1 with no matches. If a match identifies a dedicated capability remnant, remove it in its owning file. Do not remove generic news/editorial uses merely to make a broad word search empty.

- [ ] **Step 4: Review every remaining market word semantically**

Run:

```bash
rg -n -i '\bmarkets?\b|\bcrypto(currency)?\b|\bbitcoin\b|\bethereum\b|\bfutures\b|\bcommodit(y|ies)\b' . --glob '!docs/superpowers/specs/2026-07-21-remove-markets-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-markets.md'
```

Expected: remaining matches are limited to generic financial news/editorial language, news fixtures/classification, marketplace terminology, or wallet/x402 cryptocurrency payments. Remove any match that advertises, routes to, fetches, caches, or formats Mu-provided live prices.

- [ ] **Step 5: Run full verification**

Run:

```bash
go test ./... -short
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit documentation and final cleanup**

```bash
git add README.md CLAUDE.md docs
git commit -m "docs: remove markets capability references"
```

- [ ] **Step 7: Inspect the completed change set**

Run:

```bash
git status --short
```

Expected: clean worktree; the recent log contains the design commit followed by Tasks 1-6; the six-task aggregate diff deletes the Markets package and removes all dedicated integration surfaces without introducing a replacement.
