# Remove Wallet and Payments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permanently destroy Mu's wallet and payment data and remove every wallet, credit, Stripe, cryptocurrency, x402, paid-app, billing, top-up, and payment-gating capability while keeping formerly metered owner features available.

**Architecture:** Add a fail-closed, one-time migration in `internal/data` that deletes the four legacy data files and wallet seed before startup writes its completion marker. Remove metering at its boundaries instead of replacing it with no-op abstractions: authenticated requests proceed directly to existing service validation and provider calls, apps become free, and all wallet interfaces and implementations disappear.

**Tech Stack:** Go 1.25, standard library HTTP/filesystem packages, existing Mu package tests, shell acceptance checks with `rg`, `go test`, `go build`, and `go vet`.

## Global Constraints

- This is an intentional breaking removal; do not add redirects, deprecated handlers, compatibility commands, or no-op quota interfaces.
- Formerly metered capabilities remain available to the authenticated owner without quota checks or charges.
- Preserve authentication, authorization, CSRF, rate limiting, provider credential checks, SSRF protection, and service validation.
- Permanently delete payment ledgers and private keys without inspecting balances, exporting keys, creating backups, or offering recovery.
- Treat missing migration targets as success; fail startup on home-resolution, deletion, verification, or marker-write errors.
- Remove `Price` and `Earnings` from the app model; old JSON fields are ignored on load and omitted on later saves.
- Preserve generic finance/payment language that is content rather than a Mu payment capability.
- Do not modify `docs/superpowers/specs/2026-07-21-remove-wallet-payments-design.md` during implementation.

---

## File Structure

- Create `internal/data/remove_wallet_payments.go` for the destructive migration only.
- Create `internal/data/remove_wallet_payments_test.go` for deletion, idempotency, marker, and error tests.
- Keep existing service files in place; remove only wallet imports and payment gates from them.
- Delete the complete `wallet/` package after all consumers have been removed.
- Delete dedicated wallet/x402 CLI files, pricing page, wallet image, x402 script, and wallet-specific docs.
- Update existing API, MCP, agent, app, client, navigation, status, configuration, and documentation files in their current packages.

### Task 1: Implement Destructive Payment Data Migration

**Files:**
- Create: `internal/data/remove_wallet_payments.go`
- Create: `internal/data/remove_wallet_payments_test.go`

**Interfaces:**
- Produces: `data.RemoveWalletPayments() error`, called by server startup in Task 5.
- Internal test seam: `removeWalletPayments(homeDir func() (string, error)) error`.
- Internal worker: `removeWalletPaymentFiles(home string) error`.

- [ ] **Step 1: Write failing deletion and marker tests**

Create `internal/data/remove_wallet_payments_test.go` with tests that create all five targets under `t.TempDir()`, call the internal worker, and require every target to be absent and the marker to exist:

```go
package data

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLegacyPaymentFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("legacy-secret"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveWalletPaymentFilesDeletesAllTargetsAndMarksComplete(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".mu", "data")
	targets := []string{
		filepath.Join(dataDir, "wallets.json"),
		filepath.Join(dataDir, "transactions.json"),
		filepath.Join(dataDir, "daily_usage.json"),
		filepath.Join(dataDir, "trade_wallets.json"),
		filepath.Join(home, ".mu", "keys", "wallet.seed"),
	}
	for _, target := range targets {
		writeLegacyPaymentFile(t, target)
	}

	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("target %s remains; stat error = %v", target, err)
		}
	}
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if info, err := os.Stat(marker); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("marker %s missing or invalid: info=%v err=%v", marker, info, err)
	}
}

func TestRemoveWalletPaymentFilesAllowsMissingTargetsAndReruns(t *testing.T) {
	home := t.TempDir()
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

func TestRemoveWalletPaymentFilesDoesNotMarkPartialDeletion(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, ".mu", "data", "wallets.json")
	writeLegacyPaymentFile(t, filepath.Join(blocker, "child"))

	err := removeWalletPaymentFiles(home)
	if err == nil || !strings.Contains(err.Error(), blocker) {
		t.Fatalf("error = %v, want path %s", err, blocker)
	}
	marker := filepath.Join(home, ".mu", "data", removeWalletPaymentsMarker)
	if _, err := os.Lstat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("marker exists after failed deletion: %v", err)
	}

	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestRemoveWalletPaymentsFailsWhenHomeCannotResolve(t *testing.T) {
	want := errors.New("home unavailable")
	err := removeWalletPayments(func() (string, error) { return "", want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestRemoveWalletPaymentFilesRejectsNonRegularMarker(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, ".mu", "data", removeWalletPaymentsMarker)
	if err := os.MkdirAll(marker, 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("error = %v, want invalid marker path", err)
	}
}

func TestRemoveWalletPaymentFilesLeavesNoMarkerWhenMarkerWriteFails(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".mu", "data")
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if err := os.MkdirAll(marker+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeWalletPaymentFiles(home); err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("error = %v, want marker path", err)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("marker exists after marker-write failure: %v", err)
	}
}
```

- [ ] **Step 2: Run the migration tests to verify they fail**

Run: `go test ./internal/data -run 'TestRemoveWallet' -count=1`

Expected: build failure because `removeWalletPaymentFiles`, `removeWalletPayments`, and `removeWalletPaymentsMarker` do not exist.

- [ ] **Step 3: Implement fail-closed deletion and durable marker creation**

Create `internal/data/remove_wallet_payments.go`:

```go
package data

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const removeWalletPaymentsMarker = "remove-wallet-payments-v1.done"

var removeWalletPaymentDataFiles = []string{
	"wallets.json",
	"transactions.json",
	"daily_usage.json",
	"trade_wallets.json",
}

// RemoveWalletPayments permanently removes legacy payment persistence once.
func RemoveWalletPayments() error {
	return removeWalletPayments(os.UserHomeDir)
}

func removeWalletPayments(homeDir func() (string, error)) error {
	home, err := homeDir()
	if err != nil {
		return fmt.Errorf("remove wallet payments migration: resolve home directory: %w", err)
	}
	if home == "" {
		return errors.New("remove wallet payments migration: resolve home directory: empty path")
	}
	return removeWalletPaymentFiles(home)
}

func removeWalletPaymentFiles(home string) error {
	dataDir := filepath.Join(home, ".mu", "data")
	marker := filepath.Join(dataDir, removeWalletPaymentsMarker)
	if info, err := os.Lstat(marker); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("remove wallet payments migration: marker %s is not a regular file", marker)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove wallet payments migration: inspect marker %s: %w", marker, err)
	}

	targets := make([]string, 0, len(removeWalletPaymentDataFiles)+1)
	for _, name := range removeWalletPaymentDataFiles {
		targets = append(targets, filepath.Join(dataDir, name))
	}
	targets = append(targets, filepath.Join(home, ".mu", "keys", "wallet.seed"))
	for _, target := range targets {
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove wallet payments migration: delete %s: %w", target, err)
		}
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("remove wallet payments migration: target remains after deletion: %s", target)
			}
			return fmt.Errorf("remove wallet payments migration: verify deletion of %s: %w", target, err)
		}
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("remove wallet payments migration: create marker directory %s: %w", dataDir, err)
	}
	temp := marker + ".tmp"
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("remove wallet payments migration: create marker %s: %w", temp, err)
	}
	if _, err = f.WriteString("completed\n"); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("remove wallet payments migration: write marker %s: %w", marker, err)
	}
	if err := os.Rename(temp, marker); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("remove wallet payments migration: commit marker %s: %w", marker, err)
	}
	if err := syncDirectory(dataDir); err != nil {
		_ = os.Remove(marker)
		return fmt.Errorf("remove wallet payments migration: sync marker directory %s: %w", dataDir, err)
	}
	return nil
}
```

Use `os.Remove`, not `os.RemoveAll`, for targets so an unexpected non-empty directory causes startup failure instead of recursive data loss.

- [ ] **Step 4: Run migration tests**

Run: `go test ./internal/data -run 'TestRemoveWallet' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the migration helper**

```bash
git add internal/data/remove_wallet_payments.go internal/data/remove_wallet_payments_test.go
git commit -m "data: destroy legacy wallet payment state"
```

### Task 2: Remove Generic API and Agent Metering

**Files:**
- Modify: `internal/api/mcp.go`
- Modify: `internal/api/mcp_micro.go`
- Modify: `internal/api/mcp_page.go`
- Modify: `internal/api/api_page.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/mcp_test.go`
- Modify: `internal/agents/agents.go`
- Modify: `agent/agent.go`
- Modify: `agent/run.go`
- Modify: `agent/agent_test.go`
- Modify: `main.go`

**Interfaces:**
- `api.Tool` no longer has `WalletOp`.
- `api.QuotaCheck`, `agent.QuotaCheck`, and `agent.ChargeQuota` cease to exist.
- `api.ToolGuard` and `api.ExecuteTool` retain their current signatures and behavior.

- [ ] **Step 1: Replace quota tests with direct-execution and absence tests**

In `internal/api/mcp_test.go`, delete `TestMCPHandler_QuotaCheckBlocks`, `TestMCPHandler_QuotaCheckAllows`, `TestMCPHandler_IncludedToolsSkipQuotaCheck`, and `TestMCPHandler_MeteredToolsHaveWalletOp`. Add package-level tests that inspect the existing `tools` slice:

```go
func TestMCPToolsExcludeWalletAndTopup(t *testing.T) {
	for _, tool := range tools {
		switch tool.Name {
		case "wallet", "wallet_balance", "wallet_topup", "pay":
			t.Fatalf("removed payment tool is still registered: %s", tool.Name)
		}
	}
}

func TestMCPToolDescriptionsContainNoPaymentGating(t *testing.T) {
	got := strings.ToLower(ToolDescriptions())
	for _, forbidden := range []string{"credits", "top up", "x402", "wallet balance"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("tool descriptions still contain %q", forbidden)
		}
	}
}
```

In `agent/agent_test.go`, delete wallet-balance and top-up formatter tests and add:

```go
func TestModelsHaveNoPaymentMetadata(t *testing.T) {
	for _, model := range Models {
		metadata := strings.ToLower(fmt.Sprintf("%+v", model))
		if strings.Contains(metadata, "agent_query") || strings.Contains(metadata, "wallet") {
			t.Fatalf("model %q exposes payment metadata: %s", model.Name, metadata)
		}
	}
}
```

Add `fmt` to the existing `agent/agent_test.go` imports.

- [ ] **Step 2: Run focused tests to verify the old registrations fail them**

Run: `go test ./internal/api ./agent -count=1`

Expected: FAIL because wallet tools and payment descriptions are still registered.

- [ ] **Step 3: Remove the generic metering model and rendering**

Apply these exact structural changes:

```go
// internal/api/mcp.go
type Tool struct {
	Name        string                                       `json:"name"`
	Aliases     []string                                     `json:"-"`
	Description string                                       `json:"description"`
	Title       string                                       `json:"title,omitempty"`
	Icon        string                                       `json:"icon,omitempty"`
	Method      string                                       `json:"method,omitempty"`
	Path        string                                       `json:"path,omitempty"`
	Params      []ToolParam                                  `json:"params,omitempty"`
	Handle      func(map[string]any) (string, error)         `json:"-"`
	HandleAuth  func(map[string]any, string) (string, error) `json:"-"`
	Card        func() string                                `json:"-"`
}
```

Delete `api.QuotaCheck`, all `WalletOp:` literals, and the static
`wallet_balance` and `wallet_topup` tool entries.

In `internal/api/mcp_micro.go`, remove the captured `walletOp`, quota branch, `formatCredits`, and `itoa`. The guarded handler must reduce to this order:

```go
if ToolGuard != nil && r != nil {
	if err := ToolGuard(r, name); err != nil {
		return "", err
	}
}
result, found, err := ExecuteTool(r, name, args)
if !found {
	return "", fmt.Errorf("unknown tool: %s", name)
}
return result, err
```

In `internal/api/mcp_page.go`, `internal/api/api_page.go`, and `internal/agents/agents.go`, remove metered selectors, JSON `metered` fields, credit badges, the Payments card, x402 copy, and wallet wording. In `internal/api/api.go`, delete wallet endpoint catalog entries and remove credit/payment wording from weather, search, agent, and MCP descriptions.

- [ ] **Step 4: Remove agent quota behavior and main wiring**

In `agent/agent.go`, remove `Model.WalletOp`, wallet operation values, `QuotaCheck`, `ChargeQuota`, the streaming query quota branch, wallet/pay planner instructions, wallet result formatting, and top-up formatting. In `agent/run.go`, retain authentication/model selection and call the existing execution path directly.

In `main.go`, delete the complete assignments to `api.QuotaCheck`,
`agent.QuotaCheck`, `apps.QuotaCheck`, `apps.ChargeQuota`, and
`agent.ChargeQuota`, including the local `chargeUser` closure.

Delete every `WalletOp:` field from dynamic `api.Tool` registrations. Keep wallet tool handlers and routes temporarily; Task 5 removes those after all imports are gone.

- [ ] **Step 5: Run focused API and agent tests**

Run: `go test ./internal/api ./internal/agents ./agent -count=1`

Expected: PASS.

Run: `rg -n 'WalletOp|QuotaCheck|ChargeQuota|Insufficient credits|top.?up|x402|metered' agent internal/api internal/agents main.go`

Expected: no Mu payment-gating matches; remaining `main.go` wallet route/tool implementation is removed in Task 5.

- [ ] **Step 6: Commit generic metering removal**

```bash
git add agent internal/api internal/agents main.go
git commit -m "agent: remove credit metering"
```

### Task 3: Make Domain Services Execute Without Credits

**Files:**
- Create: `payment_removal_test.go`
- Modify: `chat/chat.go`
- Modify: `search/search.go`
- Modify: `search/read.go`
- Modify: `search/fetch.go`
- Modify: `news/news.go`
- Modify: `video/video.go`
- Modify: `social/social.go`
- Modify: `mail/mail.go`
- Modify: `places/places.go`
- Modify: `weather/weather.go`
- Modify: `weather/weather_test.go`
- Modify: `images/images.go`
- Modify: `chat/chat_test.go`
- Modify: `search/search_test.go`
- Modify: `search/fetch_test.go`
- Modify: `news/news_test.go`
- Modify: `video/video_test.go`
- Modify: `social/main_test.go`
- Modify: `mail/mail_test.go`
- Modify: `places/service_test.go`
- Modify: `images/file_test.go`

**Interfaces:**
- Each existing HTTP handler and exported service function keeps its signature.
- Valid authenticated requests proceed to existing provider/service logic without a wallet call.

- [ ] **Step 1: Change service assertions from credit gating to direct availability**

Create `payment_removal_test.go` in package `main` with a structural regression
that fails before the service edits and remains useful after removal:

```go
package main

import (
	"os"
	"strings"
	"testing"
)

func TestDomainServicesContainNoPaymentGates(t *testing.T) {
	files := []string{
		"chat/chat.go", "search/search.go", "search/read.go", "search/fetch.go",
		"news/news.go", "video/video.go", "social/social.go", "mail/mail.go",
		"places/places.go", "weather/weather.go", "images/images.go",
	}
	forbidden := []string{
		`"mu/wallet"`, "wallet.CheckQuota(", "wallet.ConsumeQuota(",
		"wallet.DeductCredits(", "wallet.QuotaExceededPage(", "http.StatusPaymentRequired",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(content), needle) {
				t.Errorf("%s retains payment gate %q", file, needle)
			}
		}
	}
}
```

Update existing handler tests in each package so valid owner requests assert they never return `http.StatusPaymentRequired` and never contain `credit`, `top up`, or `/wallet`. Add this reusable assertion locally in each affected package test file rather than creating a cross-package helper:

```go
func assertNoPaymentGate(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code == http.StatusPaymentRequired {
		t.Fatalf("request was payment-gated: %s", recorder.Body.String())
	}
	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"insufficient credits", "top up", "/wallet"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response contains removed payment copy %q: %s", forbidden, recorder.Body.String())
		}
	}
}
```

Use the package's existing authenticated request/session setup and provider stubs. Do not weaken assertions for `401`, `403`, `429`, validation errors, or provider failures. In `weather/weather_test.go`, replace the expected phrase `credit-backed refreshes require an account` with owner-authenticated feature copy that does not mention payment.

- [ ] **Step 2: Run affected package tests and observe payment assertions fail**

Run:

```bash
go test . ./chat ./search ./news ./video ./social ./mail ./places ./weather ./images -short -count=1
```

Expected: FAIL in tests that still encounter credit checks or old payment copy.

- [ ] **Step 3: Delete service-level wallet checks and charges**

Remove `"mu/wallet"` imports and these exact behaviors:

| File | Remove | Preserve direct flow |
|---|---|---|
| `chat/chat.go` | `CheckQuota`, quota page/402, `ConsumeQuota` | Existing prompt validation and chat execution |
| `search/search.go` | web-search check/page/consume | Existing Brave search and response rendering |
| `search/read.go` | web-fetch check/page/consume | Existing URL validation and reader |
| `search/fetch.go` | web-fetch check/page/consume | Existing fetch and SSRF/error handling |
| `news/news.go` | API/HTML search checks, 402/page, consume | Existing query search paths |
| `video/video.go` | all three search checks, JSON/HTML 402, consume | Existing search/provider paths |
| `social/social.go` | both social-search checks and consume | Existing search and rendering |
| `mail/mail.go` | internal/external checks, `useFree`, 402, consume | Existing recipient checks and SMTP/internal delivery |
| `places/places.go` | search/nearby checks, wallet links, deductions | Existing Google/place provider calls |
| `weather/weather.go` | forecast/pollen checks, deductions, pollen price, top-up JS | Existing forecast and optional pollen calls |
| `images/images.go` | generation check/consume, image price copy, forced 402 | Existing owner/prompt validation and provider errors |

For image JSON errors, replace the unconditional payment status with the existing handler's ordinary provider/input error status; do not convert unrelated provider failures to success.

- [ ] **Step 4: Format and run domain tests**

Run: `gofmt -w chat/chat.go search/search.go search/read.go search/fetch.go news/news.go video/video.go social/social.go mail/mail.go places/places.go weather/weather.go weather/weather_test.go images/images.go`

Run:

```bash
go test . ./chat ./search ./news ./video ./social ./mail ./places ./weather ./images -short -count=1
```

Expected: PASS.

Run:

```bash
rg -n '"mu/wallet"|wallet\.|StatusPaymentRequired|Insufficient credits|top.?up|credits? per' chat search news video social mail places weather images
```

Expected: no Mu payment behavior. Preserve `stripe.com` and payment phrases in mail spam/filter content.

- [ ] **Step 5: Commit direct service execution**

```bash
git add payment_removal_test.go chat search news video social mail places weather images
git commit -m "services: remove credit gates"
```

### Task 4: Make Apps Permanently Free

**Files:**
- Modify: `apps/apps.go`
- Modify: `apps/editor.go`
- Modify: `apps/fetch.go`
- Modify: `apps/micro_build.go`
- Modify: `apps/service_test.go`
- Modify: `main.go`
- Modify: `docs/APPS.md`

**Interfaces:**
- `apps.App` loses `Price` and `Earnings`.
- `UpdateApp(slug, name, description, tags, html, icon string) (*App, error)`.
- `UpdateAppOwned(accountID, slug, name, description, tags, html, icon string) (*App, error)`.

- [ ] **Step 1: Add a legacy JSON omission test**

Add to `apps/service_test.go`:

```go
func TestLegacyPaymentFieldsAreIgnoredAndOmitted(t *testing.T) {
	var legacy App
	if err := json.Unmarshal([]byte(`{
		"id":"legacy","slug":"legacy-app","name":"Legacy","author_id":"owner",
		"price":25,"earnings":400,"public":true
	}`), &legacy); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"price"`, `"earnings"`} {
		if bytes.Contains(out, []byte(forbidden)) {
			t.Errorf("saved app retains removed field %s: %s", forbidden, out)
		}
	}
}

func TestLegacyPricedAppLaunchesWithoutPayment(t *testing.T) {
	var legacy App
	if err := json.Unmarshal([]byte(`{
		"id":"legacy","slug":"legacy-app","name":"Legacy","author_id":"owner",
		"html":"<!doctype html><title>Legacy</title>","price":25,"public":true
	}`), &legacy); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	original := apps
	apps = map[string]*App{legacy.Slug: &legacy}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		apps = original
		mutex.Unlock()
	}()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/apps/legacy-app/run", nil)
	handleRun(recorder, request, legacy.Slug)
	if recorder.Code == http.StatusUnauthorized || recorder.Code == http.StatusPaymentRequired {
		t.Fatalf("legacy app was payment-gated: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
```

Add the required `bytes`, `encoding/json`, `net/http`, and `net/http/httptest`
imports.

- [ ] **Step 2: Run app tests to verify legacy fields are still emitted**

Run: `go test ./apps -run 'TestLegacyPaymentFields|Test.*Launch' -count=1`

Expected: FAIL because `App` still serializes `price` and `earnings`, or because paid launch still charges.

- [ ] **Step 3: Remove paid-app state and behavior**

Delete `Price` and `Earnings` from `App`. Remove price/earnings from list JSON, creation/update request structs, form parsing, clamps, filters, editor controls, list/detail/preview markup, launch labels, earnings displays, `handleRun`, and all save mutations. Change update signatures exactly to:

```go
func UpdateApp(slug, name, description, tags, html, icon string) (*App, error)
func UpdateAppOwned(accountID, slug, name, description, tags, html, icon string) (*App, error)
```

Retain ownership checks, public/private behavior, install tracking, versioning, and app validation. Every app launches through the same free path.

Delete app `QuotaCheck` and `ChargeQuota`. In `apps/fetch.go`, retain authentication and SSRF checks but call the fetch directly. In SDK AI handling, retain auth/input/provider behavior and remove only quota callbacks. Remove the obsolete wallet-middleware comment in `apps/micro_build.go`.

In `main.go`, remove `price` parameters and parsing from app create/update MCP tools, update both function call signatures, and remove app build/run metering fields left from Task 2. In `docs/APPS.md`, remove credit-charge and app-price claims.

- [ ] **Step 4: Format and verify apps**

Run: `gofmt -w apps/apps.go apps/editor.go apps/fetch.go apps/micro_build.go apps/service_test.go main.go`

Run: `go test ./apps . -count=1`

Expected: PASS.

Run: `rg -n 'Price|Earnings|price|earnings|credits/use|ChargeAppUse|QuotaCheck|ChargeQuota' apps main.go docs/APPS.md`

Expected: no payment/pricing behavior; generic non-payment uses of “price” must be reviewed and may remain only if unrelated to app billing.

- [ ] **Step 5: Commit free apps**

```bash
git add apps main.go docs/APPS.md
git commit -m "apps: remove pricing and charges"
```

### Task 5: Delete Wallet Implementation and External Interfaces

**Files:**
- Delete: `wallet/basewallet.go`
- Delete: `wallet/eip3009.go`
- Delete: `wallet/evm.go`
- Delete: `wallet/handlers.go`
- Delete: `wallet/spendlimit.go`
- Delete: `wallet/stripe.go`
- Delete: `wallet/wallet.go`
- Delete: `wallet/x402.go`
- Delete: `wallet/x402_cdp.go`
- Delete: `wallet/x402client.go`
- Delete: all `wallet/*_test.go`
- Delete: `internal/cli/wallet.go`
- Delete: `internal/cli/x402.go`
- Modify: `internal/cli/dispatch.go`
- Modify: `internal/cli/client.go`
- Modify: `internal/cli/dispatch_test.go`
- Modify: `internal/env/env.go`
- Modify: `internal/env/env_test.go`
- Modify: `client/discord/rich.go`
- Modify: `client/discord/interactions.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `payment_removal_test.go`

**Interfaces:**
- Consumes: `data.RemoveWalletPayments() error` from Task 1.
- Deleted commands: `mu wallet`, `mu x402`, Discord `/balance`.
- Deleted routes/tools: `/wallet`, `/wallet/*`, `wallet`, `pay`.

- [ ] **Step 1: Add startup migration and interface-removal tests**

In `main.go`, add the test seam near existing migration variables:

```go
var runWalletPaymentsMigration = data.RemoveWalletPayments

func migrateWalletPayments() error {
	return runWalletPaymentsMigration()
}
```

In `main_test.go`, delete `mu/wallet` imports and `TestChargedWriteOp`. Add:

```go
func TestMigrateWalletPaymentsPropagatesFailure(t *testing.T) {
	original := runWalletPaymentsMigration
	defer func() { runWalletPaymentsMigration = original }()
	want := errors.New("cannot delete seed")
	runWalletPaymentsMigration = func() error { return want }
	if err := migrateWalletPayments(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
```

Extend `payment_removal_test.go` with a deletion assertion that fails while the
wallet package and dedicated commands remain:

```go
func TestPaymentImplementationPathsAreDeleted(t *testing.T) {
	paths := []string{"wallet", "internal/cli/wallet.go", "internal/cli/x402.go"}
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("removed payment path still exists: %s (error=%v)", path, err)
		}
	}
}

func TestMainContainsNoPaymentComposition(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"mu/wallet"`, "wallet.Load(", `http.HandleFunc("/wallet`,
		`r.URL.Path == "/wallet/stripe/webhook"`, "chargedWriteOp(",
	} {
		if strings.Contains(string(content), forbidden) {
			t.Errorf("main.go retains payment composition %q", forbidden)
		}
	}
}
```

Add `errors` and `io/fs` to `payment_removal_test.go`. In
`internal/env/env_test.go`, replace X402 fixtures with neutral names such as
`MU_TEST_ONE` and `MU_TEST_TWO` while preserving dotenv parsing coverage.

- [ ] **Step 2: Run focused tests before deletion**

Run: `go test . ./internal/cli ./internal/env ./client/discord -count=1`

Expected: FAIL while wallet/x402 commands, Discord balance, and main payment composition remain.

- [ ] **Step 3: Wire migration before service initialization and remove main composition**

In server mode, immediately after the `ServeFlag` check and before `service.Init()`, add:

```go
if err := migrateWalletPayments(); err != nil {
	app.Log("migration", "wallet payment removal failed: %v", err)
	os.Exit(1)
}
```

This call is safe only in the same commit that deletes the `wallet` import/package, because the old package `init()` reads ledger files before `main()`.

Remove from `main.go`: `mu/wallet`, wallet account cleanup hooks, `wallet.Load`, wallet balance context, wallet/pay MCP tools, `/wallet` routes, `/pricing`, Stripe webhook CSRF exemption, central charged-write logic and `chargedWriteOp`, and payment comments. Preserve owner auth and `auth.CheckPostRate` for writes.

- [ ] **Step 4: Remove CLI and Discord interfaces**

Delete `internal/cli/wallet.go` and `internal/cli/x402.go`. Remove their dispatch cases. Remove the special HTTP 402/top-up error from `internal/cli/client.go` so generic non-success handling applies. Remove wallet initialization wording from `internal/env/env.go`.

Remove Discord `balance` command metadata and interaction handling, including `mu/wallet` import and USDC embed construction.

- [ ] **Step 5: Delete the wallet package and tests**

Delete every file under `wallet/`. Do not retain crypto helpers, x402 clients, Stripe handlers, ledger structs, operation constants, or no-op stubs.

- [ ] **Step 6: Format and verify all Go packages compile without wallet**

Run: `gofmt -w main.go main_test.go internal/cli/dispatch.go internal/cli/client.go internal/cli/dispatch_test.go internal/env/env.go internal/env/env_test.go client/discord/rich.go client/discord/interactions.go`

Run: `go test ./... -short`

Expected: PASS.

Run:

```bash
test ! -d wallet
rg -n '"mu/wallet"|wallet\.|/wallet\b|stripe/webhook|chargedWriteOp|X402_|STRIPE_' --glob '*.go' .
```

Expected: no executable payment implementation references. Migration filenames and removal tests are expected matches.

- [ ] **Step 7: Commit wallet and interface deletion**

```bash
git add -A wallet internal/cli internal/env client/discord main.go main_test.go payment_removal_test.go internal/data
git commit -m "wallet: remove payment subsystem"
```

### Task 6: Remove Payment UI, Configuration, Status, Assets, and Script

**Files:**
- Delete: `home/pricing.go`
- Delete: `internal/app/html/wallet.png`
- Delete: `scripts/deploy/verify-x402.sh`
- Modify: `home/landing.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/html/mu.js`
- Modify: `internal/app/html/mu.css`
- Modify: `internal/app/private.go`
- Modify: `internal/app/private_test.go`
- Modify: `internal/app/status.go`
- Modify: `admin/env.go`
- Modify: `admin/env_test.go`
- Modify: `admin/diagnostics.go`

**Interfaces:**
- Navigation and status APIs expose no wallet/payment entry.
- Stripe webhook is no longer a public-path exception.

- [ ] **Step 1: Add configuration and private-route absence tests**

In `admin/env_test.go`, add:

```go
func TestSettingGroupsExcludePayments(t *testing.T) {
	for _, group := range settingGroups {
		for _, key := range group.Vars {
			upper := strings.ToUpper(key)
			if strings.HasPrefix(upper, "STRIPE_") || strings.HasPrefix(upper, "X402_") ||
				strings.HasPrefix(upper, "CREDIT_COST_") || upper == "DAILY_QUOTA" ||
				upper == "FREE_DAILY_QUOTA" || upper == "TRADE_RPC_URL" || upper == "TRADE_CHAIN" {
				t.Errorf("payment setting remains: %s", key)
			}
		}
	}
}
```

Change `admin/env_test.go` to an import block containing `strings` and
`testing`.

In `internal/app/private_test.go`, remove the Stripe webhook from the public
allowlist expectation and assert `/wallet/stripe/webhook` follows private-route
authentication.

- [ ] **Step 2: Run focused UI/config tests and verify failure**

Run: `go test ./admin ./internal/app ./home -count=1`

Expected: FAIL because payment settings and public wallet webhook exceptions remain.

- [ ] **Step 3: Remove visual and configuration surfaces**

Delete `home/pricing.go`, wallet image, and x402 verification script. Remove the Pricing landing link, wallet header/sidebar elements, wallet balance fetch/cache/login redirect JavaScript, and wallet/top-up/pricing-only CSS. Preserve shared table/layout CSS used elsewhere.

Remove `/wallet.png` and `/wallet/stripe/webhook` from `internal/app/private.go`.
Remove the Payments status probe from `internal/app/status.go`. Remove the
Payments and obsolete Trading environment groups from `admin/env.go`; remove
the matching `TRADE_RPC_URL`/`TRADE_CHAIN` diagnostic from
`admin/diagnostics.go` because the deleted Base wallet was their final runtime
consumer.

- [ ] **Step 4: Verify UI and configuration cleanup**

Run: `gofmt -w home/landing.go internal/app/app.go internal/app/private.go internal/app/private_test.go internal/app/status.go admin/env.go admin/env_test.go admin/diagnostics.go`

Run: `go test ./admin ./internal/app ./home -count=1`

Expected: PASS.

Run:

```bash
rg -n '/wallet|wallet\.png|head-wallet|nav-wallet|STRIPE_|X402_|Payments|Pricing|topup-option' home internal/app admin scripts
```

Expected: only destructive migration/removal-test references outside these directories; no UI/config matches here.

- [ ] **Step 5: Commit UI and configuration cleanup**

```bash
git add -A home internal/app admin scripts
git commit -m "ui: remove wallet payment surfaces"
```

### Task 7: Remove Current Payment Documentation and Product Claims

**Files:**
- Delete: `docs/WALLET_AND_CREDITS.md`
- Delete: `docs/X402.md`
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `docs/docs.go`
- Modify: `docs/docs_test.go`
- Modify: `docs/ABOUT.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/ENVIRONMENT_VARIABLES.md`
- Modify: `docs/GO_MICRO.md`
- Modify: `docs/MCP.md`
- Modify: `docs/MIGRATION.md`
- Modify: `docs/PRINCIPLES.md`
- Modify: `docs/SECURITY.md`
- Modify: `docs/SYSTEM_DESIGN.md`
- Modify: `docs/VISION.md`
- Modify: `docs/WHITEPAPER.md`
- Modify: `docs/whitepaper.go`
- Modify: `docs/DISCORD.md`
- Modify: `docs/COPILOT.md`
- Modify: `docs/MESSAGING_SYSTEM.md`

**Interfaces:**
- Documentation catalog no longer links wallet/x402 pages.
- Environment docs identify removed payment variables as obsolete for manual operator cleanup, without implying runtime support.

- [ ] **Step 1: Expand documentation regression tests**

In `docs/docs_test.go`, extend the forbidden product claims list with exact removed surfaces:

```go
forbidden := []string{
	"/wallet", "wallet_topup", "wallet_balance", "STRIPE_SECRET_KEY",
	"X402_PAY_TO", "pay per call with x402", "credits meter",
	"card top-ups", "outbound x402", "paid apps",
}
```

Apply the list to rendered/current documentation while excluding the approved removal spec. Keep existing generic-content allowances.

- [ ] **Step 2: Run docs tests and verify old claims fail**

Run: `go test ./docs -count=1`

Expected: FAIL with current wallet/payment documentation matches.

- [ ] **Step 3: Delete dedicated docs and scrub current guidance**

Delete `docs/WALLET_AND_CREDITS.md` and `docs/X402.md`; remove their entries from `docs/docs.go`. Apply these precise content rules:

- `CLAUDE.md`: remove x402 protocol, wallet package row, wallet crypto convention, and payment authentication convention.
- `README.md`, `ABOUT.md`, `VISION.md`, `SYSTEM_DESIGN.md`, `WHITEPAPER.md`: delete Payments sections and credit/x402 product claims.
- `ARCHITECTURE.md`, `GO_MICRO.md`: remove wallet package/component and x402 migration/framework claims.
- `ENVIRONMENT_VARIABLES.md`: remove active Stripe/x402/credit-cost/daily-quota configuration; add one concise obsolete-variable note telling operators Mu no longer reads them and they may remove them manually.
- `MCP.md`: remove billing/x402 section and metered-tool wording.
- `MIGRATION.md`: remove wallet metering from request-flow descriptions and document only the destructive removal migration's startup behavior.
- `PRINCIPLES.md`: replace pay-as-you-go language with self-hosted owner access wording.
- `SECURITY.md`: remove payment section and wallet review guidance; retain auth/provider-webhook security unrelated to payments.
- `whitepaper.go`: replace “Native Payments” metadata with a non-payment product description.
- `DISCORD.md`: remove wallet from linked-owner data wording.
- `COPILOT.md`: remove Mu payment-subsystem wording but preserve provider-owned subscription/key guidance.
- `MESSAGING_SYSTEM.md`: remove the statement that messages consume credits.

- [ ] **Step 4: Verify current documentation**

Run: `go test ./docs -count=1`

Expected: PASS.

Run:

```bash
rg -ni '(wallet|stripe|x402|top.?up|credit.?cost|daily.?quota|meter(?:ed|ing)?|pay per call|billing)' README.md CLAUDE.md docs --glob '!docs/superpowers/**' --glob '!docs/superpowers/specs/2026-07-21-remove-wallet-payments-design.md'
```

Expected: only legitimate generic content such as editorial topics, spam detection, provider subscriptions, or explicit obsolete-variable cleanup. Review every match manually.

- [ ] **Step 5: Commit current documentation cleanup**

```bash
git add -A CLAUDE.md README.md docs
git commit -m "docs: remove wallet payment product claims"
```

### Task 8: Scrub Historical Payment Claims

**Files:**
- Modify: `docs/superpowers/plans/2026-07-21-single-user-mu.md`
- Modify: `docs/superpowers/specs/2026-07-21-single-user-mu-design.md`
- Modify: `docs/superpowers/plans/2026-07-21-remove-markets.md`

**Interfaces:**
- Historical records no longer claim wallet/payment functionality is retained.
- The approved removal spec remains unchanged as the sole payment-removal specification.

- [ ] **Step 1: Capture the historical reference inventory**

Run:

```bash
rg -ni '(wallet|stripe|x402|top.?up|credit.?cost|daily.?quota|meter(?:ed|ing)?)' docs/superpowers --glob '!docs/superpowers/specs/2026-07-21-remove-wallet-payments-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-wallet-payments.md'
```

Expected: matches in the single-owner plan/spec and remove-markets plan.

- [ ] **Step 2: Remove or rewrite every historical product claim**

Apply these exact boundaries:

- Single-owner plan/spec: remove Stripe callback exceptions, wallet cleanup/data retention, outbound x402 retention, credit transfers, and wallet tests.
- Remove-markets plan: remove instructions to preserve wallet/x402 and remove them from allowed remaining search matches; retain generic financial-news language.
- Preserve references to generic news/editorial finance, legal “payment” language, provider subscriptions, and this removal project's own design/plan.

- [ ] **Step 3: Verify historical cleanup**

Run:

```bash
rg -ni '(wallet|stripe|x402|top.?up|credit.?cost|daily.?quota|meter(?:ed|ing)?)' docs/superpowers --glob '!docs/superpowers/specs/2026-07-21-remove-wallet-payments-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-wallet-payments.md'
```

Expected: no Mu payment capability claims. Any generic-content match must be individually justified.

- [ ] **Step 4: Commit historical documentation cleanup**

```bash
git add docs/superpowers
git commit -m "docs: scrub historical payment claims"
```

### Task 9: Repository-Wide Removal Verification

**Files:**
- Modify only files identified by failed searches, tests, build, or vet.

**Interfaces:**
- Produces a repository with no wallet/payment implementation or product surface.

- [ ] **Step 1: Run the executable-code removal sweep**

Run:

```bash
rg -n -i '(mu/wallet|wallet\.|/wallet\b|stripe|x402|base wallet|usdc|top.?up|insufficient credits|credit.?cost|daily.?quota|WalletOp|QuotaCheck|ChargeQuota|ConsumeQuota|CheckQuota|payment required|billing)' . --glob '!docs/superpowers/specs/2026-07-21-remove-wallet-payments-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-wallet-payments.md'
```

Expected allowed matches only:

- `internal/data/remove_wallet_payments.go` and its tests, because they destroy legacy state.
- Generic finance/news/editorial fixtures and prompts.
- `mail/inbound_filter.go` spam/filter terms such as `stripe.com` and “payment overdue”.
- Provider-owned subscription guidance.
- `LICENSE` legal text.

Remove every other Mu product or implementation match. Do not remove standard-library crypto used for mail/auth.

- [ ] **Step 2: Verify deleted paths and obsolete dependency absence**

Run:

```bash
test ! -d wallet
test ! -e internal/cli/wallet.go
test ! -e internal/cli/x402.go
test ! -e home/pricing.go
test ! -e internal/app/html/wallet.png
test ! -e scripts/deploy/verify-x402.sh
test ! -e docs/WALLET_AND_CREDITS.md
test ! -e docs/X402.md
```

Expected: all commands exit 0.

Run: `go mod tidy`

Expected: no unintended dependency additions; payment-only dependencies, if any, disappear from `go.mod`/`go.sum`.

- [ ] **Step 3: Run the complete verification suite**

Run: `go test ./... -short`

Expected: PASS.

Run: `go build ./...`

Expected: PASS.

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 4: Inspect the final diff for accidental feature or security removal**

Run: `git status --short && git diff --stat && git diff`

Expected: only wallet/payment removal, destructive migration, direct-access adjustments, tests, and documentation cleanup. Confirm auth, CSRF, rate limiting, provider checks, SSRF checks, mail crypto, and generic finance/editorial content remain.

- [ ] **Step 5: Commit final cleanup if verification changed files**

If Step 1, `go mod tidy`, tests, build, or vet required changes:

```bash
git add -A
git commit -m "chore: finish wallet payment removal"
```

If the worktree is clean after prior commits, do not create an empty commit.
