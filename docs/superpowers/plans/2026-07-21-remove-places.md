# Remove Places Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove Places as a Mu capability while performing a retry-safe, one-time deletion of its persisted data and wallet history.

**Architecture:** Delete the Places package and every active interface in one implementation series. Before deleting active behavior, add an isolated `internal/migration` startup migration that asks `wallet` to filter legacy operations, removes fixed Places data paths, and writes a version-1 marker only after success.

**Tech Stack:** Go 1.25, `net/http`, Mu's `internal/data` store, go-micro service/tool registries, embedded HTML/CSS/JavaScript, Go tests.

## Global Constraints

- Hard removal: do not add compatibility routes, MCP tombstones, SDK shims, or a deprecation period.
- Delete Places wallet transaction records but do not change wallet balances or daily quota totals.
- Retain `GOOGLE_API_KEY`, its admin setting, and its environment documentation because Weather uses it.
- Preserve Weather template city input by geocoding with Nominatim and then calling `mu.weather` with coordinates.
- Historical documents under `docs/superpowers/plans/` and `docs/superpowers/specs/` remain unchanged.
- Generic uses of the English word "place" remain when they do not identify the removed service.
- The only executable legacy identifiers allowed after removal are inside `internal/migration/places.go` and its test.

## File Map

- Create `internal/migration/places.go`: versioned orchestration for deleting Places data.
- Create `internal/migration/places_test.go`: migration success, idempotency, and retry tests.
- Modify `wallet/wallet.go` and `wallet/wallet_test.go`: transaction filtering API, then removal of active Places billing constants.
- Modify `main.go` and `main_test.go`: run migration in the required order and remove Places runtime wiring.
- Delete `places/`: remove the complete service implementation and tests.
- Modify `internal/api/api.go`, `internal/api/api_test.go`, `internal/api/mcp.go`, and `internal/api/mcp_test.go`: remove advertised and callable Places interfaces.
- Modify `internal/cli/dispatch.go` and `internal/cli/dispatch_test.go`: remove Places positional argument handling.
- Modify `agent/agent.go`, `agent/agent_test.go`, `agent/answer_guard.go`, `agent/agents.go`, `agent/guest.go`, and `agent/native.go`: remove planner/native Places behavior and rendering.
- Modify `agent/native_test.go`: assert native inventories exclude Places.
- Modify `agent/micro/execute.go`, `agent/micro/registry.go`, `agent/micro/router.go`, and `agent/micro/router_test.go`: remove the specialist, allowlist entries, and keyword routes.
- Modify `apps/apps.go`, `apps/static/sdk.js`, `apps/templates.go`, and `apps/templates_test.go`: remove the SDK and Place Explorer while preserving Weather city lookup.
- Modify `wallet/handlers.go`, `home/pricing.go`, and `home/home_test.go`: remove active billing and pricing surfaces.
- Modify `internal/app/app.go`, `internal/app/private.go`, `internal/app/status.go`, `internal/app/html/mu.js`, `internal/app/html/mu.css`, and `internal/app/ui_test.go`: remove navigation, assets, status, preload, and styles.
- Delete `internal/app/html/places.svg` and `internal/app/html/places.png`.
- Modify current copy in `CLAUDE.md`, `README.md`, `blog/notes.go`, `blog/notes.json`, and the non-archival `docs/*.md` files listed in Task 6.
- Modify `go.mod` and `go.sum` through `go mod tidy`: remove dependencies that become unused.

---

### Task 1: Add The Versioned Removal Migration

**Files:**
- Create: `internal/migration/places.go`
- Create: `internal/migration/places_test.go`
- Modify: `wallet/wallet.go:268-284`
- Modify: `wallet/wallet_test.go`
- Modify: `main.go:38-49,136-150`
- Modify: `main_test.go`

**Interfaces:**
- Produces: `wallet.DeleteTransactionsByOperation(operations ...string) error`
- Produces: `migration.RemovePlaces() error`
- Consumes: `data.Dir()`, `data.LoadJSON`, `data.SaveJSON`, and `data.DeleteFile`

- [ ] **Step 1: Write the failing wallet transaction-filter test**

Add `mu/internal/data` to the imports and add this test to `wallet/wallet_test.go`:

```go
func TestDeleteTransactionsByOperationPreservesAccounting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	oldWallets, oldTransactions, oldDailyUsage := wallets, transactions, dailyUsage
	wallets = map[string]*Wallet{
		"owner": {UserID: "owner", Balance: 42, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{
		"owner": {
			{ID: "search", UserID: "owner", Operation: "places_search", Amount: -5},
			{ID: "keep", UserID: "owner", Operation: OpChatQuery, Amount: -7},
			{ID: "nearby", UserID: "owner", Operation: "places_nearby", Amount: -4},
		},
	}
	dailyUsage = map[string]*DailyUsage{
		"owner:2026-07-21": {UserID: "owner", Date: "2026-07-21", Used: 9},
	}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		wallets, transactions, dailyUsage = oldWallets, oldTransactions, oldDailyUsage
		mutex.Unlock()
	})

	if err := DeleteTransactionsByOperation("places_search", "places_nearby"); err != nil {
		t.Fatal(err)
	}
	got := GetTransactions("owner", 10)
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("transactions after filter = %#v", got)
	}
	if wallets["owner"].Balance != 42 || dailyUsage["owner:2026-07-21"].Used != 9 {
		t.Fatalf("accounting changed: wallet=%#v usage=%#v", wallets["owner"], dailyUsage["owner:2026-07-21"])
	}
	var stored map[string][]*Transaction
	if err := data.LoadJSON("transactions.json", &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored["owner"]) != 1 || stored["owner"][0].ID != "keep" {
		t.Fatalf("stored transactions = %#v", stored)
	}
}
```

- [ ] **Step 2: Run the wallet test and verify it fails**

Run: `go test ./wallet -run TestDeleteTransactionsByOperationPreservesAccounting -count=1`

Expected: FAIL because `DeleteTransactionsByOperation` is undefined.

- [ ] **Step 3: Implement the transaction filter**

Add this function after `GetTransactions` in `wallet/wallet.go`:

```go
// DeleteTransactionsByOperation permanently removes matching history without
// changing balances or quota usage. It is used by destructive data migrations.
func DeleteTransactionsByOperation(operations ...string) error {
	remove := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		remove[operation] = struct{}{}
	}

	mutex.Lock()
	defer mutex.Unlock()

	filtered := make(map[string][]*Transaction, len(transactions))
	for userID, history := range transactions {
		kept := make([]*Transaction, 0, len(history))
		for _, tx := range history {
			if _, shouldRemove := remove[tx.Operation]; !shouldRemove {
				kept = append(kept, tx)
			}
		}
		filtered[userID] = kept
	}
	if err := data.SaveJSON("transactions.json", filtered); err != nil {
		return err
	}
	transactions = filtered
	return nil
}
```

- [ ] **Step 4: Run the wallet test and verify it passes**

Run: `go test ./wallet -run TestDeleteTransactionsByOperationPreservesAccounting -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing migration tests**

Create `internal/migration/places_test.go`:

```go
package migration

import (
	"os"
	"path/filepath"
	"testing"

	"mu/internal/data"
	"mu/wallet"
)

func writeRemovalFixture(t *testing.T, key string) {
	t.Helper()
	path := filepath.Join(data.Dir(), key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePlacesDeletesDataAndHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{
		"places_saved.json",
		"places/london.json",
		"places.db",
		"places.db-wal",
		"places.db-shm",
	} {
		writeRemovalFixture(t, key)
	}
	const owner = "places-removal-owner"
	if err := wallet.AddCredits(owner, 10, "places_search", nil); err != nil {
		t.Fatal(err)
	}
	if err := wallet.AddCredits(owner, 5, wallet.OpTopup, nil); err != nil {
		t.Fatal(err)
	}

	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"places_saved.json", "places", "places.db", "places.db-wal", "places.db-shm",
	} {
		if _, err := os.Stat(filepath.Join(data.Dir(), key)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", key, err)
		}
	}
	if got := wallet.GetTransactions(owner, 10); len(got) != 1 || got[0].Operation != wallet.OpTopup {
		t.Fatalf("remaining transactions = %#v", got)
	}
	if got := wallet.GetBalance(owner); got != 15 {
		t.Fatalf("balance = %d, want 15", got)
	}
	var marker map[string]int
	if err := data.LoadJSON(placesRemovalMarker, &marker); err != nil {
		t.Fatal(err)
	}
	if marker["version"] != placesRemovalVersion {
		t.Fatalf("marker = %#v", marker)
	}
}

func TestRemovePlacesCompletedMarkerMakesRetryNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	writeRemovalFixture(t, "places_saved.json")
	if err := RemovePlaces(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data.Dir(), "places_saved.json")); err != nil {
		t.Fatalf("completed migration ran again: %v", err)
	}
}

func TestRemovePlacesFailureDoesNotWriteMarkerAndIsRetryable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(data.Dir(), "places.db")
	if err := os.MkdirAll(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dbPath, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePlaces(); err == nil {
		t.Fatal("expected deleting a directory as a data file to fail")
	}
	if _, err := os.Stat(filepath.Join(data.Dir(), placesRemovalMarker)); !os.IsNotExist(err) {
		t.Fatalf("marker written after failure: %v", err)
	}
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := RemovePlaces(); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}
```

- [ ] **Step 6: Run the migration tests and verify they fail**

Run: `go test ./internal/migration -count=1`

Expected: FAIL because the package and `RemovePlaces` do not exist.

- [ ] **Step 7: Implement the versioned migration**

Create `internal/migration/places.go`:

```go
package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"mu/internal/data"
	"mu/wallet"
)

const (
	placesRemovalVersion = 1
	placesRemovalMarker  = "places_removal_migration.json"
)

// RemovePlaces deletes data owned by the retired Places service. This migration
// can be removed after all supported installations have upgraded through v1.
func RemovePlaces() error {
	var marker map[string]int
	if err := data.LoadJSON(placesRemovalMarker, &marker); err == nil {
		if marker["version"] == placesRemovalVersion {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("load places removal marker: %w", err)
	}

	if err := wallet.DeleteTransactionsByOperation("places_search", "places_nearby"); err != nil {
		return fmt.Errorf("delete places wallet history: %w", err)
	}
	if err := data.DeleteFile("places_saved.json"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete saved places searches: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(data.Dir(), "places")); err != nil {
		return fmt.Errorf("delete places cache: %w", err)
	}
	for _, key := range []string{"places.db", "places.db-wal", "places.db-shm"} {
		if err := data.DeleteFile(key); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	if err := data.SaveJSON(placesRemovalMarker, map[string]int{"version": placesRemovalVersion}); err != nil {
		return fmt.Errorf("save places removal marker: %w", err)
	}
	return nil
}
```

- [ ] **Step 8: Run the migration tests and verify they pass**

Run: `go test ./internal/migration ./wallet -run 'Test(RemovePlaces|DeleteTransactionsByOperation)' -count=1`

Expected: PASS.

- [ ] **Step 9: Write the failing startup-order test**

Add this test to `main_test.go`:

```go
func TestPlacesRemovalRunsBeforeDataAndServices(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	ownerMigration := strings.Index(string(source), "if err := migrateSingleOwner(); err != nil")
	placesMigration := strings.Index(string(source), "if err := migration.RemovePlaces(); err != nil")
	dataLoad := strings.Index(string(source), "data.Load()")
	if ownerMigration < 0 || placesMigration < 0 || dataLoad < 0 || ownerMigration >= placesMigration || placesMigration >= dataLoad {
		t.Fatalf("startup order is owner=%d places=%d data=%d", ownerMigration, placesMigration, dataLoad)
	}
}
```

- [ ] **Step 10: Run the startup-order test and verify it fails**

Run: `go test . -run TestPlacesRemovalRunsBeforeDataAndServices -count=1`

Expected: FAIL because `main.go` does not call the migration.

- [ ] **Step 11: Wire the migration into startup**

Import `mu/internal/migration` in `main.go` and insert this block immediately after the successful `migrateSingleOwner()` call and before `data.Load()`:

```go
	if err := migration.RemovePlaces(); err != nil {
		app.Log("migration", "places removal failed: %v", err)
		os.Exit(1)
	}
```

- [ ] **Step 12: Run Task 1 tests**

Run: `go test . ./internal/migration ./wallet -count=1`

Expected: PASS.

- [ ] **Step 13: Commit Task 1**

```bash
git add main.go main_test.go wallet/wallet.go wallet/wallet_test.go internal/migration/places.go internal/migration/places_test.go
git commit -m "migration: delete retired Places data"
```

---

### Task 2: Delete The Service, Routes, API, MCP, And CLI Interfaces

**Files:**
- Modify: `main.go:19-57,160-177,948-956,1164-1166`
- Modify: `main_test.go`
- Delete: `places/city.go`
- Delete: `places/google.go`
- Delete: `places/index.go`
- Delete: `places/index_test.go`
- Delete: `places/locations.json`
- Delete: `places/main_test.go`
- Delete: `places/places.go`
- Delete: `places/saved.go`
- Delete: `places/service.go`
- Delete: `places/service_test.go`
- Modify: `internal/api/api.go:614-661,688,710`
- Modify: `internal/api/api_test.go`
- Modify: `internal/api/mcp.go:429-455`
- Modify: `internal/api/mcp_test.go:128-186,188-210`
- Modify: `internal/cli/dispatch.go:225-238`
- Modify: `internal/cli/dispatch_test.go:117-142`

**Interfaces:**
- Consumes: `migration.RemovePlaces()` from Task 1; this is the only remaining runtime code allowed to use legacy Places identifiers.
- Produces: no Places package, route, REST catalog entry, MCP tool, or CLI default.

- [ ] **Step 1: Add failing exclusion tests**

Add to `main_test.go`:

```go
func TestExecutableExcludesRetiredLocationRuntime(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{
		`"mu/pla` + `ces"`,
		"pla" + "ces.Load()",
		`http.HandleFunc("/pla` + `ces`,
	} {
		if strings.Contains(string(source), removed) {
			t.Errorf("main.go retains retired location runtime wiring %q", removed)
		}
	}
}
```

Add to `internal/api/api_test.go`:

```go
func TestEndpointsExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	for _, endpoint := range Endpoints {
		if strings.HasPrefix(endpoint.Path, "/"+domain) || strings.Contains(strings.ToLower(endpoint.Name), domain) {
			t.Fatalf("retired location endpoint is still advertised: %#v", endpoint)
		}
	}
}
```

Add `strings` to that test file's imports. In `TestMCPHandler_ToolsList`, add this check inside the tools loop:

```go
		if name == "pla"+"ces_search" || name == "pla"+"ces_nearby" {
			t.Errorf("retired location tool remains in tools/list: %s", name)
		}
```

Replace `TestMCPHandler_ToolsCallUnknown` with a table test that also exercises both removed tool names without leaving contiguous legacy identifiers in the test source:

```go
func TestMCPHandler_ToolsCallUnknown(t *testing.T) {
	domain := "pla" + "ces"
	for _, name := range []string{"unknown_tool", domain + "_search", domain + "_nearby"} {
		t.Run(name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":` + strconv.Quote(name) + `,"arguments":{}}}`
			req := ownerRequest(t, "POST", "/mcp", strings.NewReader(body))
			w := httptest.NewRecorder()
			MCPHandler(w, req)

			var resp jsonrpcResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Error == nil || !strings.Contains(resp.Error.Message, name) {
				t.Fatalf("unknown tool response for %q = %#v", name, resp)
			}
		})
	}
}
```

Add `strconv` to `internal/api/mcp_test.go` imports.

Change the `TestDefaultArgKey` table in `internal/cli/dispatch_test.go` to remove `"places_search": "q"`, then add:

```go
	if _, ok := defaultArgKey("pla" + "ces_search"); ok {
		t.Error("retired location search tool retains positional argument handling")
	}
```

- [ ] **Step 2: Run the exclusion tests and verify they fail**

Run: `go test . ./internal/api ./internal/cli -run 'Test(ExecutableExcludesRetiredLocationRuntime|EndpointsExcludeRetiredLocationDomain|MCPHandler_ToolsList|DefaultArgKey)' -count=1`

Expected: FAIL because runtime wiring, endpoints, tools, and CLI handling still exist.

- [ ] **Step 3: Remove runtime wiring and current API descriptions**

In `main.go`:

- Remove the `mu/places` import.
- Remove the `places.Load()` block.
- Remove both `/places` route registrations.
- Change the agent tool description to:

```go
Description: "Ask the AI agent a question. The agent can search GitHub, news, web, video, weather, and more to answer your question.",
```

In `internal/api/api.go`, delete both Places endpoint append blocks and use these current descriptions:

```go
Description: "Query the AI agent. The agent plans and executes tool calls (GitHub, news, weather, and more) then synthesizes a response. Requires authentication. Costs credits per query. Returns a Server-Sent Events stream.",
```

```go
Description: "Owner-authenticated Model Context Protocol server for AI tool integration. Tools include GitHub, chat, news, blog, video, mail, search, wallet, and weather. Metered tools use the owner's wallet credits. x402 is available only for owner-initiated outbound calls to remote services.",
```

- [ ] **Step 4: Remove static MCP and CLI behavior**

Delete the complete `places_search` and `places_nearby` `Tool` entries from `internal/api/mcp.go` so the slice ends after the `dismiss` tool. Change the CLI switch branch to:

```go
	case "web_search", "search":
		return "q", true
```

- [ ] **Step 5: Delete the Places package**

Delete every file listed under Task 2's `Delete` section. Verify the directory is gone:

Run: `test ! -d places`

Expected: exit status 0.

- [ ] **Step 6: Run Task 2 tests**

Run: `go test . ./internal/api ./internal/cli -count=1`

Expected: PASS.

Run: `go test ./... -run '^$'`

Expected: PASS compilation for every package.

- [ ] **Step 7: Commit Task 2**

```bash
git add -A main.go main_test.go places internal/api internal/cli
git commit -m "remove Places service interfaces"
```

---

### Task 3: Remove Places From Both Agent Pipelines

**Files:**
- Modify: `agent/agent.go:680-681,714-715,739-740,1430-1433,1488-1489,1683-1758,1776-1777,2119-2178`
- Modify: `agent/agent_test.go:18-136`
- Modify: `agent/answer_guard.go:565-566,1045`
- Modify: `agent/agents.go:90`
- Modify: `agent/guest.go:29-30`
- Modify: `agent/native.go:63-76,182-186`
- Modify: `agent/native_test.go`
- Modify: `agent/micro/execute.go:186-187`
- Modify: `agent/micro/registry.go:4-47`
- Modify: `agent/micro/router.go:107-124`
- Modify: `agent/micro/router_test.go`

**Interfaces:**
- Produces: agent tool inventories and built-in micro-agent registry with no Places capability.
- Produces: generic tool formatting for any unknown legacy tool result; no Places-specific card or text formatter.

- [ ] **Step 1: Write failing agent inventory tests**

Add `slices` and `strings` imports to `agent/native_test.go`, then add:

```go
func TestAgentInventoriesExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	for _, services := range [][]string{nativeServices(true), nativeServices(false), AllAgentTools()} {
		if slices.Contains(services, domain) {
			t.Fatalf("agent service inventory retains retired location domain: %v", services)
		}
	}
}

func TestNativePromptSourceExcludesRetiredLocationCapability(t *testing.T) {
	source, err := os.ReadFile("native.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "pla"+"ces and points of interest") {
		t.Fatal("native prompt still advertises retired location capability")
	}
}
```

Also add `os` to the imports. Add to `agent/micro/router_test.go`:

```go
func TestBuiltInAgentsExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	if Get(domain) != nil {
		t.Fatal("retired location specialist remains registered")
	}
	weather := Get("weather")
	if weather == nil {
		t.Fatal("weather specialist is missing")
	}
	for _, tool := range weather.Tools {
		if tool == domain+"_search" || tool == domain+"_nearby" {
			t.Fatalf("weather specialist retains retired location tool %q", tool)
		}
	}
}

func TestKeywordRouteNeverReturnsRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	for _, prompt := range []string{"find coworking", "cafes nearby", "restaurant"} {
		for _, id := range keywordRoute(prompt) {
			if id == domain {
				t.Fatalf("keywordRoute(%q) returned removed agent", prompt)
			}
		}
	}
}
```

- [ ] **Step 2: Run the agent inventory tests and verify they fail**

Run: `go test ./agent ./agent/micro -run 'Test(AgentInventoriesExcludeRetiredLocationDomain|NativePromptSourceExcludesRetiredLocationCapability|BuiltInAgentsExcludeRetiredLocationDomain|KeywordRouteNeverReturnsRetiredLocationDomain)' -count=1`

Expected: FAIL because both agent pipelines still advertise and route to Places.

- [ ] **Step 3: Remove planner Places behavior**

In `agent/agent.go`:

- Remove Places entries from both rendered tool lists.
- Remove `places_search` and `places_nearby` progress labels.
- Remove the Places branch from tool-card rendering.
- Delete `renderPlacesCard`, `placesMapURL`, `formatPlacesResult`, and `placeItem` completely.
- Remove the Places branch from `formatToolResult`.

Delete these Places-only tests from the start of `agent/agent_test.go`:

```text
TestPlacesMapURL_QueryAndNear
TestPlacesMapURL_QueryOnly
TestPlacesMapURL_AddressArg
TestPlacesMapURL_FallbackToCoordinates
TestPlacesMapURL_FallbackToPlacesPage
TestFormatPlacesResult_WithResults
TestFormatPlacesResult_EmptyResults
TestFormatPlacesResult_InvalidJSON
TestRenderPlacesCard_MapLink
TestRenderPlacesCard_Empty
```

In `agent/answer_guard.go`, remove the Places source mapping and remove `"no places found"` from unavailable phrases. Remove Places entries from `agent/guest.go`. Change `agent/agents.go` persona-designer copy to:

```go
sys := `You design AI agent personas for Mu, a personal assistant with tools for news, weather, mail, web search, video, and social. Given a brief, output ONLY minified JSON with exactly these keys:
```

- [ ] **Step 4: Remove native and micro-agent behavior**

Use these service lists in `agent/native.go`:

```go
	pub := []string{"weather", "news", "social", "video", "blog", "search"}
```

```go
	return []string{"weather", "news", "social", "video", "blog", "search", "recall", "apps", "mail"}
```

Change the native prompt capability sentence to:

```go
		"Use the available tools for live or personal data (weather, news, " +
		"social, video, blog, web search, the user's own mail inbox, and recall across their news/mail). " +
```

In `agent/micro/registry.go`:

- Remove Places from Micro's system prompt.
- Set Weather's tools to `[]string{"weather_forecast"}`.
- Delete the complete built-in agent whose ID is `places`.

In `agent/micro/router.go`, delete the fixed routes for `coworking`, `nearby`, `restaurant`, and `cafe`. Remove both Places entries from `agent/micro/execute.go`.

- [ ] **Step 5: Run all agent tests**

Run: `go test ./agent ./agent/micro -count=1`

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

```bash
git add agent
git commit -m "remove Places agent behavior"
```

---

### Task 4: Remove The App SDK And Place Explorer

**Files:**
- Modify: `apps/apps.go:1123-1140`
- Modify: `apps/static/sdk.js:13-22`
- Modify: `apps/templates.go:77-118,620-692,951-1046`
- Modify: `apps/templates_test.go`

**Interfaces:**
- Produces: `window.mu` without a `places` member in both SDK implementations.
- Preserves: Weather template city search through direct Nominatim geocoding.

- [ ] **Step 1: Write failing app surface tests**

Extend `apps/templates_test.go` with:

```go
func TestTemplatesAndSDKExcludeRetiredLocationCapability(t *testing.T) {
	domain := "pla" + "ces"
	if template := GetTemplate("place-" + "explorer"); template != nil {
		t.Fatalf("removed Place Explorer template remains: %#v", template)
	}
	for _, template := range Templates {
		if strings.Contains(template.HTML, "mu."+domain) {
			t.Fatalf("template %q calls retired location API", template.ID)
		}
	}
	for _, path := range []string{"apps.go", "static/sdk.js"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), domain+":") || strings.Contains(string(source), "/"+domain+"/") {
			t.Fatalf("%s retains retired location SDK methods", path)
		}
	}
}

func TestWeatherTemplateGeocodesCitiesWithoutRetiredLocationAPI(t *testing.T) {
	weather := GetTemplate("weather")
	if weather == nil {
		t.Fatal("weather template is missing")
	}
	if strings.Contains(weather.HTML, "mu."+("pla"+"ces")) {
		t.Fatal("weather template still depends on retired location API")
	}
	if !strings.Contains(weather.HTML, "nominatim.openstreetmap.org/search") {
		t.Fatal("weather template lost city geocoding")
	}
}
```

Add `os` to the imports.

- [ ] **Step 2: Run the app tests and verify they fail**

Run: `go test ./apps -run 'Test(TemplatesAndSDKExcludeRetiredLocationCapability|WeatherTemplateGeocodesCitiesWithoutRetiredLocationAPI)' -count=1`

Expected: FAIL because the SDK, Place Explorer, and Weather dependency remain.

- [ ] **Step 3: Remove Places from both SDKs and delete Place Explorer**

Delete this object from both `apps/apps.go` and `apps/static/sdk.js`:

```js
places:{
  search:function(o){return post('/places/search',o)},
  nearby:function(o){return post('/places/nearby',o)},
},
```

In `apps/templates.go`, delete the `place-explorer` registry entry and delete the complete `templatePlaceExplorer` constant.

- [ ] **Step 4: Replace Weather template city lookup**

Replace the Weather template's `searchCity` function with:

```js
function searchCity() {
  var city = document.getElementById('city').value.trim();
  if (!city) return;
  document.getElementById('content').innerHTML = '<div class="loading">Finding city...</div>';
  fetch('https://nominatim.openstreetmap.org/search?q=' + encodeURIComponent(city) + '&format=json&limit=1', {
    headers: {'Accept': 'application/json'}
  }).then(function(r) {
    if (!r.ok) throw new Error('Location search failed');
    return r.json();
  }).then(function(results) {
    if (!results || results.length === 0) { showError('City not found'); return; }
    showWeather(parseFloat(results[0].lat), parseFloat(results[0].lon));
  }).catch(function(e) { showError(e.message); });
}
```

- [ ] **Step 5: Run all app tests**

Run: `go test ./apps -count=1`

Expected: PASS.

- [ ] **Step 6: Commit Task 4**

```bash
git add apps/apps.go apps/static/sdk.js apps/templates.go apps/templates_test.go
git commit -m "remove Places app capability"
```

---

### Task 5: Remove Active Billing, Navigation, Assets, Status, And Styles

**Files:**
- Modify: `wallet/wallet.go:20-44,52-83,360-394`
- Modify: `wallet/wallet_test.go:115-160`
- Modify: `wallet/handlers.go:135-136,325-326,587-588`
- Modify: `home/pricing.go:18-27`
- Modify: `home/home_test.go`
- Modify: `internal/app/app.go:281`
- Modify: `internal/app/private.go:33-34`
- Modify: `internal/app/status.go:168-173`
- Modify: `internal/app/html/mu.js:19`
- Modify: `internal/app/html/mu.css:4385-4553`
- Modify: `internal/app/ui_test.go`
- Delete: `internal/app/html/places.svg`
- Delete: `internal/app/html/places.png`

**Interfaces:**
- Consumes: literal legacy operation names only through `internal/migration/places.go`.
- Produces: active wallet operation and UI inventories with no Places entries.

- [ ] **Step 1: Write failing billing and UI exclusion tests**

In `wallet/wallet_test.go`, remove active Places cases from `TestGetOperationCost` and `TestOperationConstants`, then add:

```go
func TestRetiredLocationOperationsUseNoActivePricing(t *testing.T) {
	domain := "pla" + "ces"
	for _, operation := range []string{domain + "_search", domain + "_nearby"} {
		if got := GetOperationCost(operation); got != 1 {
			t.Fatalf("GetOperationCost(%q) = %d, want default 1", operation, got)
		}
	}
}
```

Add `net/http/httptest` and `strings` to `home/home_test.go`, then add:

```go
func TestPricingExcludesRetiredLocationDomain(t *testing.T) {
	req := httptest.NewRequest("GET", "/pricing", nil)
	recorder := httptest.NewRecorder()
	PricingHandler(recorder, req)
	if strings.Contains(strings.ToLower(recorder.Body.String()), "pla"+"ces") {
		t.Fatalf("pricing retains retired location domain: %s", recorder.Body.String())
	}
}
```

Add to `internal/app/ui_test.go`:

```go
func TestRenderedNavigationAndStatusExcludeRetiredLocationDomain(t *testing.T) {
	domain := "pla" + "ces"
	html := RenderHTML("Test", "Test", "<p>body</p>")
	if strings.Contains(strings.ToLower(html), "/"+domain) || strings.Contains(strings.ToLower(html), domain+"</span>") {
		t.Fatal("navigation retains retired location domain")
	}
	for _, service := range buildStatus().Services {
		if strings.Contains(strings.ToLower(service.Name), domain) {
			t.Fatal("status retains retired location API")
		}
	}
}
```

Extend `TestPublicPrivateAssetsTrackCurrentNavigationAssets`:

```go
	domain := "pla" + "ces"
	for _, asset := range []string{"/" + domain + ".svg", "/" + domain + ".png"} {
		if publicPrivateAssets[asset] {
			t.Fatalf("retired location asset remains public: %s", asset)
		}
	}
```

Add to `TestCSSExcludesMarketCardStyles`, after its existing assertions:

```go
	domain := "pla" + "ces"
	for _, selector := range []string{"/* " + "Pla" + "ces page */", "." + domain + "-page", "." + domain + "-forms", ".place-card", ".city-grid"} {
		if strings.Contains(string(css), selector) {
			t.Fatalf("retired location CSS remains: %s", selector)
		}
	}
```

- [ ] **Step 2: Run the billing and UI tests and verify they fail**

Run: `go test ./wallet ./home ./internal/app -run 'Test(RetiredLocationOperationsUseNoActivePricing|PricingExcludesRetiredLocationDomain|RenderedNavigationAndStatusExcludeRetiredLocationDomain|PublicPrivateAssetsTrackCurrentNavigationAssets|CSSExcludesMarketCardStyles)' -count=1`

Expected: FAIL because active Places costs and UI surfaces remain.

- [ ] **Step 3: Remove active wallet pricing**

In `wallet/wallet.go`, delete:

```go
CostPlacesSearch = getEnvInt("CREDIT_COST_PLACES_SEARCH", 5)
CostPlacesNearby = getEnvInt("CREDIT_COST_PLACES_NEARBY", 4)
```

Delete the `OpPlacesSearch` and `OpPlacesNearby` constants and their branches in `GetOperationCost`. Delete all Places pricing/operation rows from `wallet/handlers.go`. Delete the Places row from `home/pricing.go`.

- [ ] **Step 4: Remove navigation, assets, status, and CSS**

- Delete the `/places` navigation anchor from `internal/app/app.go`.
- Delete `/places.png` and `/places.svg` entries from `internal/app/private.go`.
- Delete the Google Places status check from `internal/app/status.go`, but keep all `GOOGLE_API_KEY` handling used by Weather.
- Delete `'/places.svg',` from the preload list in `internal/app/html/mu.js`.
- Delete the complete CSS region from `/* Places page */` through the `.type-filter-btn` rule, leaving the Weather section header as the next block.
- Delete `internal/app/html/places.svg` and `internal/app/html/places.png`.

- [ ] **Step 5: Run Task 5 tests**

Run: `go test ./wallet ./home ./internal/app -count=1`

Expected: PASS.

- [ ] **Step 6: Compile all packages**

Run: `go test ./... -run '^$'`

Expected: PASS, proving no deleted billing symbol remains referenced.

- [ ] **Step 7: Commit Task 5**

```bash
git add -A wallet home internal/app
git commit -m "remove Places billing and UI"
```

---

### Task 6: Remove Current Product Copy And Unused Dependencies

**Files:**
- Modify: `CLAUDE.md:8`
- Modify: `README.md:6`
- Modify: `blog/notes.go:46`
- Modify: `blog/notes.json:20`
- Modify: `docs/ABOUT.md:20`
- Modify: `docs/APPS.md:37`
- Modify: `docs/ARCHITECTURE.md:26`
- Modify: `docs/ENVIRONMENT_VARIABLES.md:78-79`
- Modify: `docs/MIGRATION.md:56,160-161`
- Modify: `docs/SYSTEM_DESIGN.md:4`
- Modify: `docs/VISION.md:4`
- Modify: `docs/WHITEPAPER.md:32`
- Modify: `internal/app/usage.go:13`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: current documentation and generated-content prompts that describe only supported capabilities.
- Preserves: `GOOGLE_API_KEY` documentation for Weather and all archival superpowers documents.

- [ ] **Step 1: Record the failing current-surface residue scan**

Run:

```bash
rg -n -i '\bplaces\b|places_search|places_nearby|/places|mu\.places|CostPlaces|OpPlaces|mu/places|place-explorer|places\.(svg|png)|google_places' \
  --glob '!docs/superpowers/plans/**' \
  --glob '!docs/superpowers/specs/**' \
  --glob '!internal/migration/places.go' \
  --glob '!internal/migration/places_test.go'
```

Expected: matches in current product copy and `internal/app/usage.go`. Do not proceed if matches reveal an executable surface not assigned to Tasks 2-5; add that file to the applicable task first.

- [ ] **Step 2: Update current product and architecture copy**

Make these exact semantic edits:

- Remove Places from the service lists in `CLAUDE.md`, `README.md`, `docs/ABOUT.md`, `docs/SYSTEM_DESIGN.md`, `docs/VISION.md`, and `docs/WHITEPAPER.md` without changing the surrounding claims.
- Remove `places/` from the private-owner-services row in `docs/ARCHITECTURE.md`.
- Remove Places from the app-helper list in `docs/APPS.md`.
- Remove `CREDIT_COST_PLACES_SEARCH` and `CREDIT_COST_PLACES_NEARBY` from `docs/ENVIRONMENT_VARIABLES.md`; retain `GOOGLE_API_KEY`.
- Remove Places from the completed service list and delete the obsolete Places follow-up in `docs/MIGRATION.md`.
- Remove Places from the capabilities sentence in `blog/notes.go`.
- Delete the complete `"Places are part of the everyday stack"` entry from `blog/notes.json`, preserving valid JSON.
- Change the example service comment in `internal/app/usage.go` from `google_places` to `brave`.

- [ ] **Step 3: Format and validate modified text data**

Run:

```bash
gofmt -w main.go main_test.go \
  wallet/wallet.go wallet/wallet_test.go wallet/handlers.go \
  internal/migration/places.go internal/migration/places_test.go \
  internal/api/api.go internal/api/api_test.go internal/api/mcp.go internal/api/mcp_test.go \
  internal/cli/dispatch.go internal/cli/dispatch_test.go \
  agent/agent.go agent/agent_test.go agent/answer_guard.go agent/agents.go agent/guest.go agent/native.go agent/native_test.go \
  agent/micro/execute.go agent/micro/registry.go agent/micro/router.go agent/micro/router_test.go \
  apps/apps.go apps/templates.go apps/templates_test.go \
  home/pricing.go home/home_test.go \
  internal/app/app.go internal/app/private.go internal/app/status.go internal/app/ui_test.go internal/app/usage.go
```

Expected: no output.

Run: `go test ./blog ./apps -count=1`

Expected: PASS, including JSON/template loading.

- [ ] **Step 4: Remove unused modules**

Run: `go mod tidy`

Expected: `github.com/asim/quadtree` is removed from `go.mod`; Google API modules used by Video remain.

Run: `git diff -- go.mod go.sum`

Expected: only dependencies made unused by deleting Places are removed. If `google.golang.org/api` is removed, stop and investigate because `video/video.go` imports it.

- [ ] **Step 5: Run the final residue scan**

Run:

```bash
rg -n -i '\bplaces\b|places_search|places_nearby|/places|mu\.places|CostPlaces|OpPlaces|mu/places|place-explorer|places\.(svg|png)|google_places' \
  --glob '!docs/superpowers/plans/**' \
  --glob '!docs/superpowers/specs/**' \
  --glob '!internal/migration/places.go' \
  --glob '!internal/migration/places_test.go'
```

Expected: no output. Review generic uses of the singular word `place` separately; do not remove unrelated prose or identifiers such as `placeholder`.

- [ ] **Step 6: Run complete verification**

Run: `go test ./... -short`

Expected: PASS.

Run: `go vet ./...`

Expected: PASS with no diagnostics.

Run: `go build ./...`

Expected: PASS.

Run: `git diff --check`

Expected: no output.

- [ ] **Step 7: Review the complete removal diff**

Run: `git status --short && git diff --stat && git diff`

Expected: only Places removal, migration, tests, dependency cleanup, and current-copy changes are present. Confirm that `docs/superpowers/specs/2026-07-21-remove-places-design.md` and older archival documents were not rewritten.

- [ ] **Step 8: Commit Task 6**

```bash
git add CLAUDE.md README.md blog docs/ABOUT.md docs/APPS.md docs/ARCHITECTURE.md docs/ENVIRONMENT_VARIABLES.md docs/MIGRATION.md docs/SYSTEM_DESIGN.md docs/VISION.md docs/WHITEPAPER.md docs/superpowers/plans/2026-07-21-remove-places.md internal/app/usage.go go.mod go.sum
git commit -m "docs: remove Places product references"
```

- [ ] **Step 9: Confirm the branch is complete**

Run: `git status --short`

Expected: no output.

Run: `git log --oneline -6`

Expected: the six implementation commits appear in task order after the design commit.
