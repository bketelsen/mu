# CLI

`mu` runs the server with `mu --serve` and is also the owner CLI. CLI tool calls
use the same private MCP endpoint as other clients.

## Authenticate

Create a PAT at `/token` after owner login, then configure it locally:

```bash
mu login
mu config set token "$MU_TOKEN"
# or
export MU_TOKEN="your-owner-pat"
```

The token is stored in `~/.config/mu/config.json` with mode `0600`. Point the
CLI at your server with `mu config set url https://mu.example.com` or `MU_URL`.

## Use tools

```bash
mu news
mu news_search "ai safety"
mu chat "summarise today's markets"
mu agent "check my mail and list follow-ups"
mu weather_forecast --lat 51.5 --lon -0.12
```

Run `mu help` for the live tool catalog. Arguments map directly to MCP tool
parameters, and `--raw`, `--pretty`, and `--table` select output formatting.
All commands authenticate as the server owner; there is no local user selection.

## First run

Run `mu setup` to configure an AI provider, then start `mu --serve` and finish
owner setup in the browser. The setup flow is available only before the owner
exists. See [Installation](INSTALLATION.md).
