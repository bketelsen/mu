# Final Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve the final security, identity, backup durability, and API-surface review findings without changing approved single-owner or outbound x402 behavior.

**Architecture:** Keep channel command redaction as one pure internal-app helper used immediately before every channel message log. Enforce the reserved `micro` login identity at validation, setup, and persistence boundaries. Make backup commits durable on Unix by syncing copied files, the temporary directory before rename, and its parent afterward.

**Tech Stack:** Go standard library, existing internal auth/data/app packages, Go tests.

## Global Constraints

- Preserve the plan-mandated first-restart normalization backup when the migration marker is absent.
- Do not reintroduce multi-owner behavior or inbound x402 authentication bypasses.
- Use failing tests before every production behavior change.

---

### Task 1: Protect Channel Credentials

**Files:**
- Modify: `internal/app/app.go`, `client/discord/discord.go`, `client/telegram/telegram.go`, `client/whatsapp/whatsapp.go`
- Test: `internal/app/app_test.go`

- [ ] Add failing tests proving link and unlink command text is redacted while ordinary messages remain visible.
- [ ] Run `go test ./internal/app -run TestRedact -count=1` and verify failure.
- [ ] Implement a pure shared redaction helper, replace channel message log arguments with it, and remove WhatsApp raw-body logging.
- [ ] Run the focused test and verify success.

### Task 2: Reserve Micro For The Agent

**Files:**
- Modify: `internal/auth/username.go`, `internal/auth/auth.go`, `internal/setup/setup.go`
- Test: `internal/auth/username_test.go`, `internal/auth/owner_test.go`, `internal/setup/setup_test.go`, `internal/auth/migration_test.go`

- [ ] Add failing tests for validation, direct account creation, setup rendering, and migration retaining a valid owner over `micro`.
- [ ] Run focused auth/setup tests and verify failure.
- [ ] Validate before provider persistence in setup, reject in `Create`, and retain the valid migration survivor.
- [ ] Run focused auth/setup tests and verify success.

### Task 3: Make Backups Durable And Remove Owner Deletion API

**Files:**
- Modify: `internal/data/backup.go`, `internal/data/backup_test.go`, `internal/auth/auth.go`, `agent/memory_extract_test.go`, `wallet/stripe_test.go`
- Test: `internal/data/backup_test.go`

- [ ] Add a failing sync-error test using a package-private seam.
- [ ] Run `go test ./internal/data -run TestBackup -count=1` and verify failure.
- [ ] Sync copied files and Unix directory metadata around rename, propagating failures and retaining cleanup. Remove exported `auth.Delete` and refactor tests to avoid owner removal.
- [ ] Run focused data, agent, and wallet tests and verify success.

### Task 4: Verify And Report

**Files:**
- Create: `.superpowers/sdd/final-review-fix-report.md`

- [ ] Run focused auth/setup/channel/data/agent/wallet tests.
- [ ] Run `go test ./... -short -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check`.
- [ ] Self-review secret logging and reserved identity paths, write the evidence report, and commit `security: protect single-owner credentials`.
