# Domain Services

Each package below follows a common shape: `Load()` registers a
go-micro service and starts any background loops; an HTTP `Handler`
serves the package's page(s); a `Server` type (if present) exposes RPC
methods called via `service.Call`. Read `main.go`'s linear `Load()`
sequence to see exact startup order and cross-package wiring.

## news
RSS aggregation, article summaries/sentiment, indexed search, home
headlines. `Load()` registers `"news"`; `Server.Headlines`,
`Server.Read`, `Server.Search` are the RPC surface; `Handler` serves
`/news`. Storage: `internal/data` files (`feed.json`, rendered HTML)
plus the shared search index; home card via `internal/snapshot`.
Background: hourly/on-demand feed refresh, event-driven HN-comment
refresh, per-article AI summary generation
(`news.StartSentimentLoop()` started separately in `main.go`).
Integrates with `topics` for feed config, emits summary events consumed
by `chat`, feeds `search`/`recall`/`digest`.
Key files: `news/news.go`, `news/service.go`.

## mail
Owner-scoped inbox, SMTP receive/send, DKIM, spam filtering,
blocklist, encryption at rest, threading. `Load()` registers `"mail"`
(`Server.Search`, `Server.Inbox`); `Handler` serves `/mail`;
`StartSMTPServerIfEnabled()` (called separately in `main.go`) starts
the SMTP listener. Storage: `internal/data` JSON (messages encrypted,
spam filter, blocklist, whitelist, sent tracking). `mail.OnNewMail`
callback (wired in `main.go:269-276`) fans new-mail notifications out
to Discord/Telegram/WhatsApp. `recall` searches mail live and
owner-scoped rather than through the shared public index — mail bodies
never enter that index.
Key files: `mail/mail.go`, `mail/service.go`, `mail/smtp.go`.

## blog
Markdown microblog: posts, comments, tags, previews, AI auto-tagging,
daily AI-generated digest posts, a low-cadence "notes" self-blog loop.
`Load()` registers `"blog"` (`Server.Recent`); `Handler`,
`PostHandler`, `CommentHandler` serve HTTP. Storage: `internal/data`
(`blog.json`, `comments.json`) plus shared index and a snapshot-backed
preview card. Publishes `blog_updated` (consumed by `home`); receives
tag results from `chat`. `digest.PublishBlogPost` /
`digest.UpdateBlogPost` / `digest.FindTodayBlogDigest` callbacks are
wired in `main.go:317-342` so `news/digest` can create/update blog
posts without `blog` importing `digest`. `blog.StartOpinion()` and
`blog.StartNotes()` are started separately in `main.go` (disable notes
with `NOTES=off`).
Key files: `blog/blog.go`, `blog/service.go`.

## video
Curated YouTube-channel feed. `Load()` initializes the YouTube client,
registers `"video"` (`Server.Latest`), loads videos asynchronously;
`Handler` serves `/video`. Storage: embedded `channels.json` (curated
list) plus cached `internal/data/videos.json`, shared index, snapshot
card.
Key files: `video/video.go`, `video/service.go`.

## weather
Location-based forecast + optional pollen data. `Load()` registers
`"weather"` (`Server.Forecast`); `Handler` serves HTML/JSON `/weather`.
No server-side storage — caches live client-side (localStorage).
Key files: `weather/weather.go`, `weather/service.go`.

## search
Local indexed-content search + Brave web search + safe fetch/reader +
generated web topics. `Load()` registers `"search"` (`Server.Search`,
`Server.Fetch`); handlers: `Handler`, `WebHandler`, `FetchHandler`,
`ReadHandler`, `PreviewHandler`. Uses `internal/safefetch` for
untrusted URL fetches. `search.StartTopics()` (started separately in
`main.go`) regenerates web topics in the background. Used by the
`web_search`/`web_fetch` tools and by chat RAG.
Key files: `search/search.go`, `search/service.go`, `search/topics.go`.

## images
AI image generation, per-user galleries, a daily ambient home image.
`Load()` restores the daily image and starts its scheduler; exports
`Generate`/`Search`; `Handler`/`FileHandler` serve HTTP. Storage: daily
image metadata in `internal/data`; per-user generated images in
`internal/userdb` (`images/generated`). Background: daily scheduler
retries hourly until success, else wakes at 06:00 UTC. Backs the
`image_generate`/`image_search` tools registered in `main.go`.
Key files: `images/images.go`, `images/file.go`.

## apps
User-authored small HTML/JS apps ("mini-apps"): versioning, an SDK
(`apps/static/sdk.js` exposing `window.mu`), a public directory, and
AI-assisted app building/forking. `Load()` registers `"apps"`
(`Server.Build`, `Server.Search`, `Server.Read`); `Handler` serves
management, rendering, SDK endpoints, and JS execution (`apps_run`
tool). Storage: app definitions/version history in
`internal/data/apps.json`; each app's own SDK database calls go through
`internal/userdb` namespaced `apps/<slug>`. Publishes `apps_updated`
(consumed by `home`). `apps.AuthorNameFor` is wired in `main.go:284-289`
so app author display names are resolved server-side, never trusted
from a model. Uses `internal/safefetch` for any app-initiated URL
fetches; app HTML capped at 256 KB.

### apps/micro
Constrained AI micro-app generator: the model produces a validated
JSON `Spec` (tracker, checklist, or counter) which is then rendered
deterministically — the model never writes raw HTML for these.
Exports `Generate`, `Render`, `Spec`, `Field`, `Counter`,
`Spec.Validate`. No storage or service registration of its own.
Key files: `apps/micro/generate.go`, `apps/micro/spec.go`,
`apps/micro/render.go`.

### apps/static
Browser-side runtime only. `sdk.js` implements `window.mu` (bridges
weather/news/video/blog/chat/search/apps/AI/agent and per-app storage
endpoints back to the server) and the framework's `window.app`;
`sdk.css` styles framework-generated apps.

## github
GitHub repos/issues/PRs/threads via the GitHub API. `Load()` registers
`"github"` (`Server.Repositories`, `Server.Repository`,
`Server.Search`, `Server.Issue`); `Handler` is an admin GitHub
workspace. Token comes from `internal/settings`. `github.Load()` /
`github.RegisterTools()` are both called from `main.go` early
(`main.go:241-242`).
Key files: `github/github.go`, `github/service.go`, `github/handler.go`.

## chat
WebSocket/HTTP chat rooms with LLM/RAG answers, topic summaries, and
async tagging of articles/notes/posts. `Load()` does **not** register a
go-micro service (no cross-package RPC surface); `Handler` serves
`/chat`. Storage: `internal/data/chat_summaries.json` plus in-memory
room/message state. Background: event subscribers for summary/tag
requests, a summary worker, initial topic-summary generation, idle-room
cleanup, per-room LLM replies. Consumes `topics` config; publishes AI
summaries to `news` and tags to `blog`.
Key file: `chat/chat.go`.

## stream
Append-only operational/conversational timeline (user, agent, system,
news events) — the platform's "console". `Load()` only logs restored
state; `Publish`/`PostUser`/`PostAgent`/`PostSystem` are the write API;
`Handler`/`FragmentHandler` serve HTTP. Storage:
`internal/data/stream.json`, in-memory with a 500-event cap. POST
starts async moderation and — for `@micro` mentions — invokes
`stream.AIReplyHook`, wired in `main.go:370-384` to run
`agent.Query(owner.ID, prompt)` and post the answer back via
`stream.PostAgent`.
Key files: `stream/stream.go`, `stream/handlers.go`.

## topics
Shared persisted topic configuration: each topic pairs an RSS feed URL
with a chat-summary prompt. `Load() error`, `Snapshot`, `Create`,
`Update`, `Delete`, `Subscribe` — no HTTP handler of its own (edited via
`admin.TopicsHandler`). Storage: its own atomic `topics.json` under
`internal/data.Dir()`. Consumed live by both `news` (feed list) and
`chat` (summary prompts).
Key file: `topics/topics.go`.

## recall
Cross-source, model-ready search: combines the shared public index
(news/blog/video) with a strictly account-scoped live mail search. No
`Load()` — `recall.Server` is registered directly as `"recall"` by
`main.go:515-517` (not via a package-level `Load()`). Deliberately
keeps mail bodies out of the public shared index; imports `mail`
directly to search it live per-caller.
Key file: `recall/recall.go`.

## home
Configurable dashboard/home cards with a card cache and independent
per-card fragment loading. `Load()` builds cards from an embedded
`cards.json`; `Handler`, `CardHandler`, `RefreshHandler` serve HTTP.
No storage of its own — subscribes to `blog_updated`/`apps_updated` and
renders cards sourced from `agent`, `apps`, `blog`, `chat`, `news`,
`video`, `weather`, `images`, and (per-viewer) `mail`. Each card render
runs in a goroutine with a 3-second timeout so one slow source can't
block the whole dashboard.
Key file: `home/home.go`.

## docs
Embedded Markdown documentation catalog + renderer (this is the
*product* `docs/` served at `/docs` — distinct from `yeti/`, which is
this AI-context documentation). `Load()` is a no-op; `Handler` serves
`/docs` and cataloged slugs; `WhitepaperHandler` serves `/whitepaper`
and generates/caches a PDF at `/whitepaper.pdf`. All top-level
`docs/*.md` files are `//go:embed`ed, but only files in the catalog are
directly routable.
Key files: `docs/docs.go`, `docs/whitepaper.go`, `docs/pdf.go`.

## admin
Owner-only operations UI: environment/settings editor (`/admin/env`),
logs (`/admin/log`), API call log (`/admin/api`), email log
(`/admin/email`), AI usage (`/admin/usage`), diagnostics, console,
topics editor, mail spam-filter/blocklist management, content deletion
across any type, server update/restart. No `Load()` or go-micro
service — reads/writes `internal/settings`, `internal/app` logs/
metrics, `internal/data` deleters, and delegates to `mail`/`topics`/
`apps` directly.
Key files: `admin/admin.go`, `admin/env.go`, `admin/topics.go`,
`admin/delete.go`.
