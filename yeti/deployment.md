# Deployment

Covers Docker, `install.sh`, systemd socket activation, and CI/CD.

## Single binary

`main.go` dispatches on the first flag: any argument matching `--serve`,
`-serve`, `--serve=...`, or `-serve=...` is treated as server mode
(`isServerMode`, `main.go:1374-1385`); everything else is handed to
`internal/cli`, which authenticates and talks to `/mcp` over HTTP and
never touches server state. This means the exact same compiled binary
is the server, the CLI, and (via MCP) effectively an API client.
**Caveat:** only `--serve`/`-serve` (or `--serve=true`) actually starts
the server — `--serve=false` is still detected as server mode by
`isServerMode`, but the subsequent `if !*ServeFlag` check
(`main.go:198-201`) then prints `--serve not set` and returns without
running the server *or* falling back to the CLI.

## Docker

- `Dockerfile` — multi-stage Go 1.25 Alpine build producing one
  `/usr/local/bin/mu` executable. Runtime image sets `DATA_DIR=/data`,
  exposes `8080` (web), `8081`, and `2525` (SMTP), mounts `/data`, runs
  `mu --serve`. **`DATA_DIR` is currently not read by the app** —
  `internal/data.Dir()` (`internal/data/data.go:75-78`) always resolves
  to `$HOME/.mu/data` regardless of `DATA_DIR`, so unless the
  container's `$HOME` is also `/data` (it isn't by default — root's
  home in the Alpine image), data is actually written to
  `/root/.mu/data`, not the declared `/data` volume. This is a known
  drift between the Docker packaging and the code; treat `DATA_DIR` as
  aspirational/no-op until `internal/data` is updated to honor it.
- `docker-compose.yml` — builds the local Dockerfile, publishes host
  `8080`, uses a named persistent volume `mu-data` at `/data`.
  Documents optional env-based admin/AI-provider configuration and
  includes a commented-out optional Ollama sidecar (reachable via
  `host.docker.internal:11434/v1`). Restart policy `unless-stopped`.

## `install.sh`

Installs to `~/.local/bin/mu` by default (`MU_BIN_DIR`/
`MU_INSTALL_DIR` override). If Go is available, shallow-clones
`micro/mu` and builds locally; otherwise downloads the matching
platform/arch release binary (amd64/arm64, Linux/macOS — matches the
naming produced by the release workflow: `mu-<os>-<arch>`). Adds the
binary directory to shell PATH where appropriate, creates the data
directory, and directs the operator to `mu setup` then `mu --serve`.

## Systemd socket activation (zero-downtime restarts)

`serveListener` (`main.go:1356-1368`): if the process was started via
systemd socket activation (`LISTEN_PID` matches the current PID and
`LISTEN_FDS >= 1`), Mu adopts the inherited listening socket (fd 3,
`SD_LISTEN_FDS_START`) instead of binding its own. This lets the kernel
keep queuing/accepting connections on the held socket across a binary
restart during redeploy, so a reverse proxy in front of it sees a
moment of latency rather than a connection refusal (502). Without
activation, it just binds `*AddressFlag` normally. Docker/compose
deployment does **not** itself configure socket activation — this path
is specific to the systemd-managed production deployment described in
`.github/workflows/deploy.yml`.

## CI/CD (`.github/workflows/`)

- `test.yml` — on push/PR to `main`: Go 1.25 with module cache,
  enforces `gofmt`, runs `go vet ./...`, short tests, race-detector
  short tests, and a root build.
- `deploy.yml` — on push to `main`: SSHes to the production host
  (user `mu`, port `61194`), pulls `main`, loads `~/.env`, runs
  `go install`, attempts the zero-downtime socket-activation setup
  script, then restarts the `mu` systemd service.
- `release.yml` — on `v*` tags or manual dispatch: cross-compiles
  static Linux/macOS binaries for amd64/arm64 and publishes them as
  GitHub Release assets named `mu-<os>-<arch>` (the names `install.sh`
  expects).

## Data directory and backups

All persistent state lives under one directory: `~/.mu/data`, resolved
by `internal/data.Dir()` (`internal/data/data.go:75-78`) as
`$HOME/.mu/data` — this is hardcoded and does **not** honor `DATA_DIR`
(see the Docker caveat above). `internal/data.Backup` produces atomic,
timestamped sibling-directory backups; some but not all startup
migrations back up before mutating (`migrateRemoveSocial` does;
`migrateWalletPayments` and `migration.RemovePlaces` do not — see
`main.go:200-229`). Operators should back up this entire directory
before upgrading — see `docs/INSTALLATION.md` for the full
migration/backup story (legacy migration retains the backup, uses the
oldest admin as owner, or resets an instance with no admin).
