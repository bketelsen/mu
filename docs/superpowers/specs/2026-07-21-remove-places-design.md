# Remove Places Design

## Goal

Remove Places as a Mu domain capability. The package, runtime service, routes, tools, agent behavior, app SDK, UI, pricing, dependencies, and current product documentation will no longer expose or describe Places.

Upgrading installations will also delete Places runtime data and Places wallet transaction records. Existing wallet balances and quota totals will not change.

## Scope

The removal includes:

- The complete `places/` package and its tests.
- Places startup loading, go-micro registration, and HTTP routes.
- REST catalog entries, MCP tools, CLI handling, and tool tests.
- Agent prompts, allowlists, routing, rendering, formatting, and the Places micro-agent.
- Places navigation, assets, CSS, status reporting, pricing, and current product copy.
- The `mu.places` app SDK methods and the Place Explorer template.
- Places wallet costs and active operation definitions.
- Places-specific dependencies that become unused.
- Stored Places caches, saved searches, database files, and wallet transaction history.

The removal does not include:

- `GOOGLE_API_KEY`, because Weather also uses it.
- Google API dependencies still required by Video or other services.
- Generic uses of the English word "place" that do not identify the Places service.
- Rewriting historical implementation plans or specifications under `docs/superpowers/`.
- Compatibility routes, MCP tombstones, SDK shims, or a deprecation period.
- Refunds or changes to current wallet balances and quota totals.

## Architecture

Places will be removed atomically. No production capability will remain available after the change. Old HTTP requests will receive normal not-found behavior, unknown MCP tools will be rejected through the existing tool lookup behavior, and custom apps using `mu.places` will fail normally because the SDK method and routes no longer exist.

A small versioned startup migration in `internal/migration/places.go` will be the only temporary legacy component. It will remove persisted Places state once and record completion in `places_removal_migration.json` with version `1`. Keeping cleanup outside `places/` permits deletion of the domain package while still supporting upgrades from installations that used it.

## Removal By Surface

### Runtime

- Remove the `mu/places` import from `main.go`.
- Remove `places.Load()` and `/places` route registration.
- Delete the go-micro Places service with the rest of the package.
- Do not register replacement routes or services.

### Agent

- Remove `places_search` and `places_nearby` from prompts and tool allowlists.
- Remove Places answer-guard classification, progress labels, result cards, text formatting, and map-link helpers.
- Remove Places from native service inventories and general agent descriptions.
- Remove the Places micro-agent and its keyword routing.
- Remove or update tests that cover deleted Places-specific behavior.

Weather and web search remain independent tools. Neither will be presented as a compatibility replacement for Places.

### API And CLI

- Remove Places endpoints from the REST API catalog.
- Remove the static Places MCP tool definitions and wallet operation metadata.
- Remove CLI argument normalization that exists only for `places_search`.
- Update inventory and dispatch tests to assert that Places is absent.

### Apps

- Remove `mu.places.search` and `mu.places.nearby` from both the embedded SDK and `apps/static/sdk.js`.
- Remove the Place Explorer built-in template and its registry entry.
- Preserve the Weather template's city input without Places by using Nominatim geocoding directly, matching the approach already used by the Weather page, and then calling `mu.weather` with coordinates.
- Do not add compatibility behavior for installed or custom apps that call `mu.places`.

### Wallet

- Remove `CostPlacesSearch`, `CostPlacesNearby`, `OpPlacesSearch`, and `OpPlacesNearby` from active wallet behavior.
- Remove Places rows from pricing, quota, and operation-cost output.
- Remove or update wallet tests that treat Places as an active operation.
- Keep the literal legacy operation names private to the removal migration so it can identify old transaction records.

### UI And Static Assets

- Remove the Places navigation link.
- Remove Places asset preload and private-asset declarations.
- Delete `places.svg` and `places.png`.
- Delete the Places-specific CSS block.
- Remove Places pricing and service-status entries.

### Configuration And Dependencies

- Remove only the Google Places status check.
- Retain `GOOGLE_API_KEY` in settings, the admin environment editor, and environment documentation because Weather consumes it.
- Run `go mod tidy` after deleting the package. `github.com/asim/quadtree` should be removed if no remaining package imports it.
- Retain Google API modules still used by Video.

### Product Copy And Documentation

- Remove Places from current service lists, architecture documentation, API descriptions, agent descriptions, and blog-generation prompts.
- Leave historical documents under `docs/superpowers/plans/` and `docs/superpowers/specs/` unchanged as archival records.

## Upgrade Migration

### Ordering

The removal migration will run from `main.go` immediately after the single-owner migration and before `data.Load()` or any domain service starts. Wallet package state is loaded by package initialization before `main`, so the migration can filter its in-memory transaction map safely. Startup will not continue if the removal migration returns a real error.

### Steps

1. Read `places_removal_migration.json` and inspect its integer `version` field.
2. If the version is `1`, return without modifying data.
3. Remove transactions whose operation is `places_search` or `places_nearby` from every owner's in-memory wallet history and persist the filtered transaction map.
4. Leave wallet balances and daily quota usage unchanged.
5. Delete `places_saved.json`.
6. Delete the `places/` city-cache directory.
7. Delete `places.db`, `places.db-wal`, and `places.db-shm`.
8. Write the completion marker only after every cleanup step succeeds.

The wallet package will expose a narrow operation-filtering function for the migration. `internal/migration/places.go` will coordinate that function with fixed-path filesystem cleanup and marker persistence; it will not manipulate wallet mutexes or internal maps directly.

### Failure Behavior

Cleanup is idempotent. Missing files, missing directories, and already-filtered transactions count as success. If persistence or filesystem cleanup fails for another reason, startup fails and the completion marker remains unwritten. The next startup retries all steps safely.

If a process stops after some cleanup steps but before writing the marker, the next run repeats the migration. Re-filtering transactions and deleting absent files must remain safe.

The marker and migration are temporary legacy code. A comment will identify that they can be removed after all supported installations have upgraded through this release.

## Testing

### Migration Tests

- Deletes saved searches, city-cache files, the SQLite database, and its sidecars.
- Filters both legacy Places operation types from all owners' transaction histories.
- Preserves unrelated transactions, wallet balances, and quota totals.
- Succeeds when all Places artifacts are already absent.
- Writes the marker only after successful cleanup.
- Becomes a no-op after the marker records the current version.
- Leaves a failed migration retryable.

### Surface Tests

- REST and MCP inventories do not advertise Places.
- Agent registries, service lists, prompts, and allowlists exclude Places.
- App SDK output does not include `mu.places`.
- The Place Explorer template is absent.
- Navigation, status, pricing, and wallet operation output do not include Places.
- Weather template city lookup no longer depends on Places.
- Removed routes and tools follow existing not-found or unknown-tool behavior.

### Repository Checks

Search for the following identifiers after implementation:

- `places_search`
- `places_nearby`
- `/places`
- `mu.places`
- `CostPlaces`
- `OpPlaces`
- `mu/places`

Matches are allowed only in the removal migration and archival documents. Generic non-service uses of "place" are allowed.

Run:

```bash
go mod tidy
go test ./... -short
go vet ./...
go build ./...
```

Review the module diff after `go mod tidy` to ensure it removes only dependencies made unused by this change.

## Completion Criteria

The work is complete when:

- Mu builds and starts without importing, loading, or routing to the Places package.
- No current user, API, MCP, CLI, agent, app SDK, pricing, status, or documentation surface advertises Places.
- Old Places calls receive standard unavailable behavior with no compatibility shim.
- Upgrade cleanup removes all known Places files and transaction records without changing balances or quota totals.
- Places-only code, tests, assets, and dependencies are gone.
- The repository checks pass.
