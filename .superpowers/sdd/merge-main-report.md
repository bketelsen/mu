# Merge Main Report

## Resolution

- Preserved the reviewed private single-owner baseline: startup migration and
  backup, default-deny `app.Private` middleware, setup-only owner creation,
  owner-bound channels and background work, private documentation, and
  outbound-only x402.
- Preserved current `main` GitHub service: `github` package, go-micro service,
  owner/admin-gated route and tools, agent routing, home card/navigation asset,
  tests, and `GITHUB_TOKEN` configuration documentation.
- Preserved all Markets removals. The `markets` package and assets remain
  deleted; no route, tool, service load, API endpoint, card, or runtime import
  remains. Historical removal plans under `docs/superpowers/` are intentionally
  retained.
- Resolved documentation as private-owner product documentation and added the
  GitHub service and `GITHUB_TOKEN`; deleted stale `docs/SERVICES.md`.

## Conflict Decisions

- `main.go`: kept the branch's migration, owner-bound callbacks, and private
  middleware; retained GitHub import, service/tool registration, and `/github`
  route; removed Markets wiring.
- `internal/api/api.go`: kept owner-authenticated MCP and outbound-only x402
  text; removed Markets endpoint and added GitHub to the tool description.
- `internal/agents/agents.go`, Discord, and home: retained owner-only behavior,
  removed Markets copy, and included GitHub where service inventory is shown.
- Docs: retained the branch's owner-model semantics while incorporating current
  main's GitHub configuration. No public signup, federation, profile, or
  inbound-payment descriptions were restored.

## Verification

Executed successfully after staging the resolution:

```text
gofmt -w main.go internal/api/api.go client/discord/link_test.go
go test ./... -count=1
go vet ./...
go build ./...
git diff --cached --check
test ! -d markets
! rg -n '"mu/markets"|markets\.Load|markets\.Handler|markets_list|Path:\s*"/markets"' --glob '*.go' --glob '!docs/superpowers/**'
! rg -n 'Signup\(|InviteHandler|RequestInvite|/presence|/\.well-known/webfinger|InboxHandler|OutboxHandler|X402ContextKey' main.go internal --glob '*.go'
```

All commands exited successfully. The Markets scan excludes historical
superpowers documents only; runtime Go code is clean.
