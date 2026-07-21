# GitHub Copilot Provider

Run Mu's agent, chat, and background AI on a GitHub Copilot subscription
instead of pay-per-token API keys. Copilot's API speaks the OpenAI
chat-completions wire format and serves **both** model families — Anthropic
(`claude-*`) and OpenAI (`gpt-*`) — through one endpoint, so a single Copilot
Pro/Business/Enterprise seat covers everything Mu does with an LLM.

## How it works

Copilot authentication is two-tiered:

1. A **long-lived GitHub OAuth token** (`gho_...`), obtained once via GitHub's
   device flow from a Copilot-enabled app. This is the only secret you store,
   as `COPILOT_GITHUB_TOKEN`. A classic PAT will **not** work.
2. A **short-lived Copilot bearer token** (~30 minutes), which Mu exchanges,
   caches, and refreshes automatically on every request. The exchange also
   returns the correct API base URL for your plan (Business/Enterprise use a
   different domain), so nothing else needs configuring.

When `COPILOT_GITHUB_TOKEN` is set, Copilot becomes the gateway for every
model call: the interactive agent/chat, the native tool-calling agent, the
planner, background summaries, tagging, and routing. Atlas-specific models
(`deepseek-*`, `qwen*`) still route to Atlas Cloud if you also have an
`ATLAS_API_KEY`.

## Setup

### Guided (recommended)

```bash
mu setup          # pick "4) GitHub Copilot"
```

This walks you through the device flow (open a URL, enter a code), verifies
the token, lists the models your subscription actually offers, and lets you
pick chat/background models. Then start the server:

```bash
mu --serve
```

Alternatively, the web first-run wizard at `/setup` accepts a pasted GitHub
OAuth token under "GitHub Copilot (subscription)".

### Manual

```bash
export COPILOT_GITHUB_TOKEN=gho_xxxxxxxxxxxx
mu --serve
```

Or set it later from `/admin/env` (AI section) — no restart needed.

## Environment variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `COPILOT_GITHUB_TOKEN` | yes | — | Long-lived GitHub OAuth token from a Copilot-enabled app (device flow). Enables the provider. |
| `COPILOT_CHAT_MODEL` | no | `claude-sonnet-4.5` | Interactive queries: agent, chat, streaming. |
| `COPILOT_BACKGROUND_MODEL` | no | `gpt-4.1` | High-volume background work: planning, summaries, tagging, routing. |
| `COPILOT_PREMIUM_MODEL` | no | chat model | The agent's "Best" tier. |

All are read env-first, then from the settings store (`/admin/env`).

## Choosing models

List what your subscription offers with `mu setup` (option 4), or query the
API directly. Model ids are used verbatim — e.g. `claude-sonnet-4.5`,
`gpt-4.1`, `gpt-5`, `o4-mini`.

Copilot serves models through two API shapes: `/chat/completions` and, for
newer OpenAI (Codex-generation) models, the Responses API (`/responses`)
only. Mu handles this automatically — it reads each model's
`supported_endpoints` from the Copilot model catalog and speaks the right
wire format, falling back and remembering if the API reports
`unsupported_api_for_model`. Any model your subscription lists is usable.

**Premium request budgets matter.** On Copilot plans, `gpt-4.1` (and other
"included" models) bill at a **0× premium-request multiplier** — effectively
unlimited — while `claude-*` models consume premium requests (1× each) from a
monthly allowance. Mu's background callers (news summaries, auto-tagging,
agent planning, routing) are high-volume, which is why
`COPILOT_BACKGROUND_MODEL` defaults to `gpt-4.1`. Keep it on a 0× model unless
you know your allowance can absorb it.

## What still needs other keys

- **Image generation** (`/images` prompt-to-image) requires `ATLAS_API_KEY` —
  Copilot has no image-generation endpoint.
- Everything non-AI (search, YouTube, mail, payments) uses its own keys as
  before; see [Environment Variables](/docs/environment-variables).

## Notes and caveats

- The provider identifies itself with VS Code client headers, which is how
  every third-party Copilot API integration works; GitHub's terms frame
  Copilot as an editor product, so keep usage personal and volumes sane.
  Heavy automated traffic on an individual seat is the pattern that gets
  accounts rate-limited or flagged.
- Usage is tracked under the `copilot` provider in `/admin` with a cost of $0
  (subscription-billed); token counts are still recorded.
- If both `COPILOT_GITHUB_TOKEN` and `ANTHROPIC_API_KEY` are set, Copilot
  wins. Unset the Copilot token to go back to direct Anthropic.
