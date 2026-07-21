# Task 8B: Background Execution Identity Audit

## Rule applied

Background work that uses an account ID as a human identity now calls
`auth.RunForOwner` at execution. The helper calls `auth.Owner()` then runs only
when the captured target equals `owner.ID`. Missing owners, legacy targets, and
multi-account legacy state are no-ops. The check happens before external model
calls, private-memory writes, notifications, wallet credits, or dedup state.

## Changed background-human candidates

| Candidate | Classification | Result |
| --- | --- | --- |
| `main.go:185` `mail.OnNewMail` callback | Background human notification; carries mail recipient and sends private email summary | Changed: bind callback to `auth.RunForOwner`; notifications use `owner.ID`. |
| `main.go:325` status `user.AIReplyHook` | Background human agent run; queries with recipient identity and can read private mail, wallet, and memory | Changed: bind to `auth.RunForOwner` before ban check, agent call, and system response. |
| `main.go:356` stream `stream.AIReplyHook` | Background human agent run; same private context and cost risk | Changed: bind to `auth.RunForOwner` before ban check and agent call. |
| `agent/run.go:20` `extractMemory` | Deferred human memory extraction; sends captured prompt to background model and writes owner memory | Changed: bind to `auth.RunForOwner` before model request and write. |
| `wallet/stripe.go:307` checkout webhook | Deferred persisted external target; credits account from checkout metadata | Changed: bind to `auth.RunForOwner` before dedup persistence and credit. |
| `wallet/stripe.go:355` subscription invoice webhook | Deferred persisted external target; credits account from invoice metadata | Changed: bind to `auth.RunForOwner` before dedup persistence and credit. |

## Shared execution helper and tests

| Candidate | Classification | Result |
| --- | --- | --- |
| `internal/auth/auth.go:146` `RunForOwner` | Owner-bound execution helper | Added. Calls `Owner()` at execution and drops non-current targets. |
| `internal/auth/owner_test.go:50` | Regression test | Added first and observed compile failure before helper implementation. Verifies current owner executes, no owner is a no-op, and a stale legacy target is discarded. |

## Reviewed and unchanged candidates

| Area | Candidate | Classification | Reason unchanged |
| --- | --- | --- | --- |
| `main.go` | MCP gateway startup, memory monitor, HTTP server, health-check fanout | System/background infrastructure | No account target or owner-private access. |
| `agent/micro` | `Orchestrate` parallel agent calls | Request-scoped parallel work | Uses the authenticated request's account ID synchronously to produce that request's response; not a later job. |
| `agent` | Flow/session records | Storage partition keys | Persisted records are listed and deleted only through request-scoped owner handlers; no later executor consumes their `AccountID`. |
| `apps` | HTML app generation service and code-run records | Request-scoped identity/storage partition | `BuildRequest.AccountID`, `AuthorID`, and code-run author fields are assigned for the current authenticated request; cleanup does not act as an author. |
| `apps/apps.go:718` app moderation | Background system moderation | Uses author ID only to moderate/ban; it does not impersonate the author or read owner-private stores. |
| `apps/run.go:31` code-run cleanup | System cleanup | Deletes expired temporary records without account-author actions. |
| `blog/blog.go:158` tag subscription | System event consumer | Updates a public post by post ID, not an account identity. |
| `blog/blog.go:225,957,1098` indexing and re-indexing | System indexing | Public content indexing only; captured author is metadata, not an execution identity. |
| `blog/blog.go:974,1230,1709,1772` tag generation and moderation | System classification | Operates on public post/comment content; no owner-private state or impersonation. |
| `blog/notes.go:84` notes loop | Allowed system identity | Publishes public editorial notes as `app.SystemUserID`; it does not read owner-private state. |
| `blog/opinion.go:65` opinion loops | Allowed system identity | Publishes public editorial work as `app.SystemUserID`; engagement loop is disabled and neither loop runs as an owner. |
| `news/digest/digest.go:46,85,125` digest scheduler | Allowed system identity | Publishes a public digest as `app.SystemUserID`; gathers public feeds/markets/video only. |
| `mail/mail.go:1139` compose autocomplete | Authenticated request-scoped | `GetAllAccounts` is only a compose-page datalist in the active request, not a background enumeration. |
| `mail/mail.go:1553` `OnNewMail` goroutine | Background dispatcher | Carries target only into the now owner-bound callback above. |
| `mail/smtp.go:842` SMTP server and `:1005` rate-limit cleanup | System infrastructure/cleanup | No account target or private owner access. |
| `social` | Post/thread/reply handlers | Authenticated request-scoped | Author IDs come from the current session; no goroutine, scheduler, or deferred account executor found. |
| `stream/handlers.go:105` post and `:108` moderation | Request-scoped/system moderation | The mention callback is now bound in `main.go`; moderation does not impersonate the author. |
| `user/user.go:101,176` presence broadcaster and WebSocket reader | Connection-scoped presence | Captured user ID only updates the live connection's presence; no private-store or human-impersonating operation. |
| `user/user.go:484` status moderation | System moderation | Uses target only for flagging/banning, not for human actions or private reads. |
| `wallet` | Wallet maps, spend limits, x402 calls, Stripe session creation | Request-scoped identity/storage partition | Account IDs originate in current authenticated handlers and execute inline. Webhook execution is covered above. |
| `client/discord` | Gateway heartbeat, inbound messages, link codes, notification lookup | Transport/request-scoped | Inbound actions already require the sole linked owner. Notifications are reached only from the owner-bound mail callback. |
| `client/telegram` | Inbound messages, links, notification lookup | Transport/request-scoped | Inbound actions already require the sole linked owner. Notifications are reached only from the owner-bound mail callback. |
| `client/whatsapp` | Inbound messages, links, notification lookup | Transport/request-scoped | Inbound actions already require the sole linked owner. Notifications are reached only from the owner-bound mail callback. |

## Explicitly allowed system identity uses

`app.SystemUserID` remains only for non-login authorship: daily news digests,
editorial notes/opinions, and `@micro` response posts. The response posts are
authored by the system but their preceding human-context agent work is now
owner-bound. The editorial/digest jobs access public service data only and do
not impersonate the owner or access owner-private stores.

## Search coverage

Reviewed goroutines, timers, ticker loops, callback registrations, event
subscriptions, webhook handlers, queue/job terminology, persisted account-ID
records, `auth.GetAllAccounts`, `auth.Owner`, and `app.SystemUserID` across
`main.go`, `agent`, `agent/micro`, `apps`, `blog`, `mail`, `news/digest`,
`social`, `stream`, `user`, `wallet`, and `client`.
