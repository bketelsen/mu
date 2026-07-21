# Task 8 Report

## Status

BLOCKED before production implementation.

## Completed Evidence

- Read the exact task brief and inspected the current auth, admin, wallet, app preference/control, and multi-user call-site surfaces.
- Established a clean baseline with `go test ./... -short`.
- Added the requested initial regression tests for owner writes, the operational admin dashboard, and removed wallet transfers.
- Ran the required RED command:

  ```text
  go test ./internal/auth ./admin ./wallet -run 'Test(OwnerCanAlwaysWrite|AdminDashboard|WalletTransferRemoved)' -count=1
  ```

  It failed as expected: `CanWrite` is undefined, the admin dashboard contains Users/Moderation/Blocklist routes, and `GET /wallet/transfer` returns 200.

## Blocker

The background-job requirement is cross-cutting and safety-critical: all account enumeration and persisted arbitrary account targets must be located, changed to resolve `auth.Owner()` at execution time, discarded on target mismatch, and individually regression-tested. The initial repository-wide search already returns more than 100 account-identity call sites and does not isolate scheduler work from ordinary per-owner partition keys.

Implementing the unrelated auth, UI, wallet, presence, federation, and domain deletions before that audit would leave an uncertain, uncommitted partial removal. The task instruction requires stopping rather than making unsafe partial changes.

## Proposed Split

1. **Task 8A: Owner-bound execution audit**: enumerate all background jobs and persisted account targets in blog, social, apps, stream, user, mail, wallet, and `main.go`; replace execution identity with `auth.Owner()`; discard stale targets; add focused scheduler regression tests.
2. **Task 8B: Auth and owner UI surface removal**: replace `CanPost` with `CanWrite`, remove account moderation/presence/blocking/preferences/MCP/admin-user surfaces, and update direct callers.
3. **Task 8C: Domain, wallet, and federation removal**: remove transfer APIs/UI while retaining historical transfer rendering, delete ActivityPub/WebFinger and profile routes, remove domain moderation reads/mutations, then run the required absence scan and full short suite.

## Commits

None. No production implementation was committed.

## Worktree Changes

The requested initial RED regression tests are present but intentionally uncommitted:

- `internal/auth/account_lifecycle_test.go`
- `admin/admin_test.go`
- `wallet/wallet_test.go`

## Task 8A

### Status

Completed.

### Evidence

- Added `auth.CanWrite`, updated central charged-write middleware to require the owner, and retained legacy posting APIs for later domain cleanup.
- Removed dashboard links and wallet routes for user administration and peer transfers; `GET /wallet/transfer` now falls through to 404.
- Removed block/unblock controls, preference storage, and MCP tools. Updated only the `apps` and `social` `app.IsBlocked` call sites so the deleted preference API no longer has domain consumers.
- Preserved historical `type:"transfer"` transaction display as generic incoming/outgoing credit text.
- `go test ./internal/auth ./internal/app ./internal/api ./admin ./wallet . -count=1` passed.
- `go test ./... -short` passed.

## Task 8A Admin Fix

### Status

Completed.

### Implementation

- Deleted `UsersHandler`, `ModerateHandler`, their account-specific rendering helpers, the new-account blog moderation helper, direct `/admin/users` and `/admin/moderate` registrations, and the blog moderation link.
- Reduced the admin console to operational commands. Removed account enumeration, account lookup, approval, ban, status clearing, arbitrary wallet lookup, credit grants, invites, and their help/prompt entries.
- Retained generic content flagging/deletion and restored the operational dashboard links, including explicit mail blocklist and spam filter controls.
- Added regression coverage for removed console commands, absent routes and blog moderation surface, and retained operational/mail dashboard links.

### TDD Evidence

- RED:

  ```text
  go test ./admin ./blog . -run 'Test(AdminDashboardContainsOnlyOperationalLinks|ConsoleRejectsLocalAccountCommands|ConsoleRetainsOperationalCommands|BlogDoesNotLinkToRemovedModeration|AdminRoutesExcludeLocalUserManagement)' -count=1
  ```

  Failed as expected because dashboard links were absent, local-account console commands still dispatched, and the removed route/link registrations remained.

- Additional RED:

  ```text
  go test ./blog -run TestBlogDoesNotLinkToRemovedModeration -count=1
  ```

  Failed as expected because `GetNewAccountBlogPosts` still existed.

### Test Evidence

```text
go test ./admin ./blog . -count=1
ok   mu/admin  0.055s
ok   mu/blog   0.008s
ok   mu        0.008s

go test ./... -short
ok   mu
ok   mu/admin
ok   mu/blog
ok   mu/wallet
ok   mu/weather
```

The full short suite completed successfully for all packages; packages without tests reported `[no test files]`.

### Review Evidence

- Route and handler scans find removed user-management identifiers only in regression-test forbidden lists.
- Console dispatch/help scan finds no removed local-account command cases or help entries.
