# Task 6 Report: Remove Every Account-Provisioning Path

## Status

Completed. Setup is the only Task 6 account creator. Web signup, invitations,
MCP signup, and Google account creation are removed. Google now resolves only a
verified email linked to the single owner. OAuth authorization validates the
client and redirect URI before authentication and issues the code for the
authenticated session account ID.

## TDD Record

RED command:

```text
go test ./internal/app ./internal/auth -run 'TestResolveGoogleOwner|TestOAuthAuthorize' -count=1
```

It failed as expected because `resolveGoogleOwner`, `loginForOAuth`, and
`createOAuthCode` did not exist.

GREEN command:

```text
go test ./internal/app ./internal/auth -run 'TestResolveGoogleOwner|TestOAuthAuthorize' -count=1
```

It passed after the owner-only Google resolver and OAuth seams/validation were
implemented.

## Scope Expansion

The brief's file list was not treated as a prohibition. These additional files
were required to remove executable invitation/signup callers, controls, and
dead links:

- `admin/admin.go`: removed the admin invitation link.
- `admin/console.go`: removed the invitation handler and `invite`/`invites`
  console commands, which otherwise referenced deleted invite storage APIs.
- `home/home.go`: replaced invitation and signup controls with account, login,
  and first-time setup controls.
- `chat/chat.go`, `internal/app/chat.go`, `internal/agents/agents.go`,
  `wallet/handlers.go`, `places/places.go`, `home/landing.go`, and
  `home/pricing.go`: replaced dead `/signup` CTAs with owner login or
  first-time setup wording.
- `chat/chat_test.go` and `home/chat_test.go`: updated affected CTA assertions.
- `internal/auth/auth.go`: removed obsolete MCP signup-token wording.
- `internal/api/mcp.go`: removed obsolete signup-rate-limit wording.

The three channel `autoCreateAccount` callers in Discord, Telegram, and
WhatsApp were not modified, as deferred to Task 7.

## Verification

Passed focused affected-package tests:

```text
go test ./internal/auth ./internal/app ./internal/setup ./internal/api ./admin ./home ./chat ./wallet ./places ./internal/agents -count=1
```

Passed full short suite once:

```text
go test ./... -short
```

Production provisioning self-review scan:

```text
rg -n 'auth\.Create\(|Name:\s*"signup"|/signup|request-invite|CreateInvite|findOrCreateGoogleAccount|auto-create account' --glob '*.go'
```

The scan contains only `internal/setup/setup.go`, test fixtures, and the three
deferred channel auto-create helpers. A second scan for web/invite/Google
provisioning symbols returned no files.

## Concerns

No captcha caller outside deleted signup and invite-request flows was found.
Existing OAuth tests remain compatible with strict client and redirect URI
validation.
