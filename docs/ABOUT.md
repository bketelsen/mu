# About Mu

Mu is a private, single-owner home server for news, mail, search, weather,
GitHub, video, and an AI agent. It is one Go binary that you run and control.

## One owner

The first-run setup creates Mu's only owner. After setup, no additional local
accounts can be created. Every web page, service, API, MCP, and A2A request is
private and resolves to that owner after authentication.

Use a password, passkey, linked Google identity, Personal Access Token (PAT),
OAuth client, CLI token, API token, MCP client, or A2A client to access the same
owner data. Internal account IDs remain implementation details used to scope
storage and never represent separate local users.

## Services

Mu's agent works across private services including RSS news, web search,
GitHub, weather, video, mail, notes, apps, and the home dashboard.
The system is designed to serve its owner rather than advertising, engagement,
or profile discovery.

## Payments

Credits meter configured external and AI costs. Card top-ups add credits to the
owner wallet. x402 is an outbound owner capability: the agent can pay a remote
x402-enabled service within configured spend limits. Incoming payment never
authenticates a request or bypasses owner authentication.

See [Installation](/docs/installation) to create the owner and run Mu.
