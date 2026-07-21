# Remove Markets Capability

## Goal

Remove Mu's dedicated markets capability completely. Mu will no longer fetch,
cache, display, expose, or route requests to live cryptocurrency, futures,
commodity, or currency price data.

Ordinary finance and market language remains valid in unrelated capabilities
such as news, web search, and editorial content. This work removes the product
capability, not the topic from Mu's vocabulary.

## Compatibility

The removed interfaces will not receive compatibility handlers, redirects, or
deprecation stubs. Requests to the old `/markets` route will follow normal
not-found behavior, and calls to removed tools will follow normal unknown-tool
behavior.

Persisted user home-card preferences may still contain the string `markets`.
The application will ignore that unknown card identifier naturally; no data
migration is required.

## Removal Scope

### Service and runtime

- Delete the `markets/` package and its tests.
- Remove startup loading and go-micro service registration.
- Remove the HTTP route, REST API description, MCP tool and card registration,
  diagnostics, health checks, and any market-specific background work.
- Remove market icons and static-cache entries.

### User interfaces and clients

- Remove the Markets navigation item, home card, card preferences, tooltips,
  mobile ordering, and dedicated CSS or JavaScript.
- Remove Discord and Telegram market commands and command metadata.
- Remove the app SDK's dedicated markets wrapper and market-specific app test
  parsing.

### Agent behavior

- Remove the Markets micro-agent, its routing keywords, permissions, tools, and
  progress messages.
- Remove dedicated market shortcuts, market-mover planning and fallback logic,
  market tool result formatting, and prompts that claim live price access.
- Keep generic routing to news or web search where those capabilities already
  handle finance-related questions; do not introduce a replacement market
  workflow.

### Composition layers

- Stop adding live prices to the home context, news digest input, blog opinion
  input, chat context, and admin diagnostics.
- Adjust surrounding prompts so they describe only the inputs still supplied.
- Remove stream and indexed-data variants only when they exist solely to carry
  output from the removed markets service.

### Documentation and examples

- Remove Markets from current product, architecture, service, API, MCP, client,
  and setup documentation.
- Remove examples that call the dedicated markets API or SDK.
- Preserve natural references to financial markets in news examples, editorial
  guidance, and fixtures when they do not claim Mu has a live markets service.

## Error Handling

No new error paths are introduced. Removed HTTP and tool interfaces use the
application's existing not-found and unknown-tool behavior. Composition layers
continue without a market-data section rather than substituting stale or
synthetic values.

## Verification

- Add or update focused tests so public tool lists, agent registries, home-card
  definitions, and client command definitions no longer expose Markets.
- Run `go test ./... -short`.
- Run `go build ./...`.
- Search for imports of `mu/markets`, dedicated routes and tool names, market
  card identifiers, client commands, and market assets.
- Review remaining uses of the words `market` and `markets`; each must be a
  legitimate generic finance/news reference rather than a remnant of the
  removed capability.

## Success Criteria

- The repository contains no `markets/` package or market-specific asset.
- The binary starts without market loading, fetching, caching, or registration.
- No UI, API, MCP, SDK, agent, Discord, or Telegram surface advertises or calls
  a dedicated markets capability.
- Digests and opinions work without live price inputs.
- Generic finance and market topics remain available through unrelated news and
  search capabilities.
- The short test suite and full build pass.
