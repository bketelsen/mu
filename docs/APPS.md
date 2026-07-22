# Apps

Apps are small private tools hosted on Mu, such as a timer, notes page, or
expense tracker. An app is HTML, CSS, and JavaScript with the Mu SDK injected by
the server. Apps run for the authenticated owner only.

## Create and run

Create an app at `/apps/new`, ask the owner agent to build one with
`apps_create` or `apps_build`, or edit an existing app at `/apps/{slug}/edit`.
Run it at `/apps/{slug}/run`.

```html
<script src="/apps/sdk.js"></script>
```

The SDK calls same-origin endpoints. Owner authentication is carried by the
session and never supplied by app JavaScript.

## Owner-scoped storage

`mu.store` is a key/value store partitioned by app and owner. `mu.db` stores
JSON collections in the same owner-scoped internal partitioning. The server
sets the internal owner identifier and rejects attempts to select a different
identity. These storage partitions are implementation boundaries, not a
multi-user sharing feature.

```javascript
await mu.store.set('prefs', { theme: 'dark' });
const prefs = await mu.store.get('prefs');

await mu.db.create('notes', { title: 'Idea', body: '...' });
const notes = await mu.db.list('notes');
```

Owner-only SDK calls include `mu.ai`, `mu.agent`, `mu.web.fetch`, weather, news,
video, search, blog, and app helpers. The server binds every
write to the owner.

## Security

- Apps require owner authentication.
- Storage and mutations are bound server-side to the owner and app partition.
- `mu.web.fetch` blocks private, loopback, link-local, and redirected internal
  destinations to reduce SSRF risk.
- App HTML, storage, and fetch requests have size and time limits.

See [MCP](/docs/mcp) for authenticated programmatic access.
