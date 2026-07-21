# Task 8A Verification UI Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the owner-facing UI that presents email verification as a posting requirement while retaining email-address verification and Google linking.

**Architecture:** `RenderHTMLForRequest` will render supplied content without a posting-gate banner. The account email card and verification request/consume handlers stay in place, with their copy describing email confirmation rather than access to posting.

**Tech Stack:** Go, `net/http`, Go standard-library testing.

## Global Constraints

- Delete `VerifyBanner` and its injection into rendered pages.
- Remove owner-facing copy saying email verification, account age, approval, or account status is required to post.
- Preserve email settings, verification mechanics, and Google linking/authentication.
- Do not change domain `CanPost` or ban/new-account call sites; Task 8C owns those APIs.
- Commit the implementation as `app: remove account posting gate UI`.

---

### Task 1: Remove The Posting Gate

**Files:**
- Modify: `internal/app/app_test.go`
- Modify: `internal/app/app.go:756-793,1027-1068`
- Modify: `.superpowers/sdd/task-8-report.md`

**Interfaces:**
- Consumes: `RenderHTMLForRequest(title, desc, html string, r *http.Request) string`
- Produces: owner page rendering without posting-gate content; unchanged email-verification and Google-linking handlers.

- [ ] **Step 1: Write the failing rendering test**

Add an authenticated-owner request test that renders content through `RenderHTMLForRequest` and fails if the result contains either obsolete banner phrase:

```go
func TestRenderHTMLForRequestOmitsPostingGate(t *testing.T) {
	// Build an owner session request using the existing app test helper.
	out := RenderHTMLForRequest("Test", "Test", "<p>content</p>", request)
	for _, text := range []string{
		"Verify your email to post.",
		"unlock status updates, replies, comments and posts",
	} {
		if strings.Contains(out, text) {
			t.Errorf("rendered owner page contains obsolete posting gate text %q", text)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app -run TestRenderHTMLForRequestOmitsPostingGate -count=1`

Expected: FAIL because `RenderHTMLForRequest` prepends `VerifyBanner` for an unverified owner.

- [ ] **Step 3: Remove only the posting-gate UI**

Delete the `VerifyBanner` call from `RenderHTMLForRequest` and delete `VerifyBanner`. Update account-email card and verification-request copy so it invites the owner to add or confirm an address without claiming this unlocks posting. Do not alter token creation/consumption or Google code.

- [ ] **Step 4: Run focused and required tests**

Run: `gofmt -w internal/app/app.go internal/app/app_test.go && go test ./internal/app -count=1 && go test ./... -short`

Expected: PASS.

- [ ] **Step 5: Append evidence and commit**

Append the exact RED and GREEN commands/results to `.superpowers/sdd/task-8-report.md`, then run `git diff --check` and commit the implementation files with:

```bash
git add internal/app/app.go internal/app/app_test.go .superpowers/sdd/task-8-report.md docs/superpowers/plans/2026-07-21-task-8a-verification-ui-fix.md
```
