# MCP Server

Mu exposes the owner's private services through Model Context Protocol (MCP) at
`POST /mcp`. MCP clients authenticate as the owner with a PAT or session token.
There is no MCP account creation flow and no anonymous tool access.

## Configure a client

Create a PAT at `/token`, then configure an MCP client:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.example.com/mcp",
      "headers": {"Authorization": "Bearer YOUR_OWNER_PAT"}
    }
  }
}
```

Use the same token with the CLI, API, and other programmatic owner clients.

## Call a tool

```bash
curl -X POST https://mu.example.com/mcp \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"news_search","arguments":{"query":"AI safety"}}}'
```

`tools/list` returns the live tool catalog. Tools that read or change owner data
bind the internal owner ID on the server. That ID is an architectural namespace,
not a tool argument for choosing an account.
