# Markets Removal Final-Fix Report

## Scope

Restored the four negative regression guards required by Tasks 1, 3, and 4
of `docs/superpowers/plans/2026-07-21-remove-markets.md`:

- `internal/api/api_test.go`: `TestEndpointsExcludeMarkets` rejects endpoint
  path `/markets` and endpoint name `Markets`.
- `client/discord/rich_test.go`: `TestSlashCommandsExcludeMarkets` rejects the
  `markets` slash command.
- `apps/templates_test.go`: `TestTemplatesExcludeMarketsCapability` rejects
  `markets` and `portfolio` templates and `mu.markets` template HTML calls.
- `home/home_test.go`: `TestCardConfigExcludesMarkets` unmarshals embedded
  `cards.json` and rejects card ID/type `markets` and link `/markets`.

The positive tests introduced by the audit review (`TestEndpointsIncludeNews`,
`TestSlashCommandsIncludeNews`, and `TestDashboardTemplateExists`) were
replaced because they do not independently protect the Markets-removal
contract. No production code changed.

Task 6's dedicated-reference audit now excludes Go test files with
`--glob '!**/*_test.go'`. The historical design spec remains unchanged; the
exclusion allows intentional negative-literal guards while retaining the audit
for production, configuration, and documentation remnants.

## Test-First Note

The four restored tests were run immediately after restoration against the
already-removed capability. They passed, as required for the current state.
A red phase was not possible without reintroducing a removed production
surface, which is outside the requested scope and would invalidate the audit.

## Commands And Results

All commands below ran from the repository root and exited 0 unless noted.

```text
go test ./internal/api -run TestEndpointsExcludeMarkets -count=1
ok   mu/internal/api  0.005s

go test ./client/discord -run TestSlashCommandsExcludeMarkets -count=1
ok   mu/client/discord  0.006s

go test ./apps -run TestTemplatesExcludeMarketsCapability -count=1
ok   mu/apps  0.007s

go test ./home -run TestCardConfigExcludesMarkets -count=1
ok   mu/home  0.008s

go test ./internal/api ./client/discord ./apps ./home -short
ok   mu/internal/api      (cached)
ok   mu/client/discord    (cached)
ok   mu/apps              (cached)
ok   mu/home              (cached)

test ! -d markets && test ! -e internal/app/html/markets.svg && test ! -e internal/app/html/markets.png && ! rg -n '"mu/markets"|markets\.|markets_list|/markets\b|mu\.markets|market_[A-Za-z<]|Markets Agent|MarketsHTML|TopMovers|GetAllPriceData' . --glob '!docs/superpowers/specs/2026-07-21-remove-markets-design.md' --glob '!docs/superpowers/plans/2026-07-21-remove-markets.md' --glob '!**/*_test.go' && ! rg -n 'piquette/finance-go|psanford/finance-go' go.mod go.sum
(no output; all absence checks passed)

go test ./... -short
(exit 0; all tested packages passed, with only internal/agents and internal/testutil reporting no test files)
```

## Self-Review

- Verified each restored guard matches the approved plan's intended assertion
  and failure message.
- Confirmed `encoding/json` is used to parse the embedded card configuration,
  not a duplicate fixture.
- Confirmed the templates guard scans every registered template using
  `strings.Contains`.
- Confirmed the audit exclusion is limited to `_test.go`; it does not exclude
  production, config, or ordinary documentation files.
- Ran `gofmt` on all changed Go tests and `git diff --check`; no formatting or
  whitespace errors were reported.
- Inspected the final diff: it contains only the requested test guards and
  audit-plan adjustment, plus this report. No production code changed.

## Concerns

None. The deliberate test-file audit exclusion means dedicated-remnant
literals are permitted only in Go regression tests; the four runtime/template/
card guards cover the intended surfaces directly.
