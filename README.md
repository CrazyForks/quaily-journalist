# Quaily Journalist

Collect, score, and publish Markdown newsletters from V2EX topics. Quaily Journalist runs a small worker that polls V2EX nodes, ranks posts using a Hacker‑News‑like formula, stores them in Redis, and periodically renders channel‑specific Markdown files (daily/weekly) using a simple template.

Use it to generate a daily or weekly digest you can post, email, or archive.

## Features

- V2EX collector by node with configurable poll interval
- HN‑like time‑decayed scoring using replies and post age
- Redis storage with sensible TTLs and period ZSETs (daily, weekly)
- Channel builder per source with filters, min/top thresholds, and skip logic
- Markdown rendering via a text/template (easy to customize)
- CLI with `serve`, `generate`, and `redis ping`
- Configurable via YAML and `QUAILY_*` environment variables
- Systemd‑friendly service unit example

## Quick Start

Prerequisites: Go 1.21+, Redis (local or remote).

1) Copy and edit `config.yaml` to your environment. At minimum, set Redis and pick V2EX nodes.

2) Verify Redis connectivity:

```bash
go run . redis ping
```

3) Start the service (collector + builders):

```bash
go run . serve
```

This writes newsletters under `out/<channel>/` when the builder has enough items. For a one‑off render (ignoring published/skip), run:

```bash
go run . generate <channel>
```

## Installation

Build a local binary:

```bash
make build
./bin/quaily-journalist --help
```

Or run directly from source during development:

```bash
go run . --help
```

## Configuration

Quaily Journalist reads `config.yaml` from one of:

- `./config.yaml` (project root)
- `$HOME/.config/quaily-journalist/config.yaml`
- `./configs/config.yaml`

You can also pass `--config /path/to/config.yaml`.

Example (redacted) configuration:

```yaml
app:
  log_level: "info"

redis:
  addr: "127.0.0.1:6379"
  username: ""
  password: ""    # Prefer env var QUAILY_REDIS_PASSWORD
  db: 0

sources:
  v2ex:
    token: ""      # Optional; or use env QUAILY_SOURCES_V2EX_TOKEN / QUAILY_V2EX_TOKEN
    base_url: "https://www.v2ex.com"
    fetch_interval: "10m"

newsletters:
  output_dir: "./out"
  channels:
    - name: "v2ex_daily_digest"
      source: "v2ex"
      nodes: ["crypto", "solana", "create"]
      frequency: "daily"
      top_n: 20
      min_items: 5
      item_skip_duration: "72h"
      preface: "Your daily V2EX highlights."
      postscript: "Brought to you by Quaily Journalist."
```

### Environment Variables

All config keys support env overrides via Viper with prefix `QUAILY_` and dot‑to‑underscore mapping. Examples:

- `QUAILY_REDIS_ADDR`, `QUAILY_REDIS_USERNAME`, `QUAILY_REDIS_PASSWORD`, `QUAILY_REDIS_DB`
- `QUAILY_SOURCES_V2EX_TOKEN`, `QUAILY_SOURCES_V2EX_BASE_URL`, `QUAILY_SOURCES_V2EX_FETCH_INTERVAL`
- `QUAILY_NEWSLETTERS_OUTPUT_DIR`

Convenience bindings also exist:

- `QUAILY_V2EX_TOKEN` (same as `sources.v2ex.token`)
- `QUAILY_V2EX_NODES` (comma‑separated list used to seed node selection)

## CLI

- `go run . --help` — show CLI help
- `go run . serve` — run service (collector + builders + scheduler)
- `go run . generate <channel>` — force‑generate today’s post for `<channel>` (writes `:output_dir/:channel/:frequency-YYYYMMDD.md` if at least `min_items` are available; ignores published/skip)
- `go run . redis ping` — ping Redis using current config

Make targets:

- `make build` — compile to `bin/quaily-journalist`
- `make test` — run unit tests
- `make vet` — static checks via `go vet`

## Run as a Service (systemd)

An example unit file is provided at `configs/quaily-journalist.service.example`. Update the `WorkingDirectory` and `ExecStart` to match your deployment, and set environment variables for secrets.

```bash
sudo systemctl enable quaily-journalist
sudo systemctl start quaily-journalist
sudo journalctl -u quaily-journalist -f
```

## Output

- Files are UTF‑8 Markdown under `newsletters.output_dir/<channel>/`
- Daily slug format: `daily-YYYYMMDD.md` (e.g., `out/v2ex_daily_digest/daily-20251023.md`)

## Architecture

- Cobra initializes Viper, loads `config.yaml` and env overrides.
- Commands create a Redis connection via `internal/redisclient`.
- Workers:
  - Collector: polls union of `channels[].nodes` for source `v2ex`, skips zero‑reply topics, computes score, stores into ZSETs per period (daily `YYYY‑MM‑DD` UTC and weekly) with item JSON and TTL.
  - Builder (per channel): filters by nodes and skip markers, enforces `min_items`/`top_n`, renders via `internal/newsletter/newsletter.tmpl`, writes file, and marks published + skipped.

## Data Flow and Keys

- Collector (≈10m): V2EX API → normalize → score → `ZADD news:source:v2ex:period:<YYYY-MM-DD>` and `SET news:item:v2ex:<id>`
- Builder (≈30m): `ZREVRANGE` → filter nodes/skip → threshold → template render → write file → mark published + skipped

Sample keys (for 2025‑10‑23):

- `news:item:v2ex:123456` — JSON of the topic (7‑day TTL)
- `news:source:v2ex:period:2025-10-23` — ZSET of IDs with scores
- `news:published:v2ex_daily_digest:2025-10-23` — flag for published period
- `news:skip:v2ex_daily_digest:123456` — skip marker (e.g., 72h TTL)

## Directory Layout

```
cmd/                 # Cobra commands (root + subcommands)
internal/            # Non-public packages (config, v2ex client, storage, newsletter)
worker/              # Long-running workers (collector, builder, manager)
configs/             # Examples (e.g., systemd unit)
config.yaml          # Application configuration
main.go              # CLI entrypoint
Makefile             # Build/test helpers
README.md            # This file
```

## Development

- Go 1.21+. Format with `go fmt ./...`
- Keep commands small and focused under `cmd/`
- Avoid cyclic dependencies between packages
- Use tabs for indentation

## Testing

- Standard `testing` package; run `go test -v ./...`
- Focus on config parsing and Redis interactions (use a test Redis or mocks)
- Validate template rendering in `internal/newsletter` with sample data

## Security Notes

- Do not commit secrets. Use env overrides like `QUAILY_REDIS_PASSWORD`, `QUAILY_SOURCES_V2EX_TOKEN`
- Prefer config search paths and local untracked variants

## Troubleshooting

- Config not found: pass `--config /path/to/config.yaml` or place it under a search path
- Redis auth/connection errors: validate `QUAILY_REDIS_*` and network access
- No newsletter generated: ensure `min_items` is met; reduce `min_items` or wait for more posts; use `generate` to force a render
- Empty sections: check `nodes` are correct for V2EX and that the collector interval elapsed

---

## Repository Guidelines

The following guidelines apply to contributors and automated agents working in this repository.

### Project Structure & Module Organization
- `main.go`: CLI entrypoint using Cobra.
- `cmd/`: Cobra commands (root and subcommands). Example: `cmd/serve.go`, `cmd/redis_ping.go`.
- `worker/`: Long-running workers (e.g., `v2ex_collector.go`, `newsletter_builder.go`).
- `internal/config/`: Viper-backed configuration types and defaults.
- `internal/v2ex/`: Minimal V2EX API client.
- `internal/storage/`: Redis-backed storage utilities.
- `internal/redisclient/`: Redis client factory.
- `internal/newsletter/`: Text/template renderer and embedded template.
- `config.yaml`: App, sources, newsletters (channels), and Redis settings.

### Build, Test, and Development Commands
- `go run . --help` — show CLI help.
- `go run . serve` — run service (workers + scheduler).
- `go run . generate <channel>` — force-generate today’s post for the channel (overwrites `:output_dir/:channel/:frequency-YYYYMMDD.md`).
- `go run . redis ping` — ping Redis using config.
- `make build` — compile to `bin/quaily-journalist`.
- `make test` — run unit tests (add `_test.go` files).
- `make vet` — basic static checks via `go vet`.

### Coding Style & Naming Conventions
- Go 1.21+. Format with `gofmt` (or `go fmt ./...`).
- Use package-scoped files: `internal/...` for non-public code.
- Names: packages lower-case (`redisclient`), files `snake_case.go`, tests `*_test.go`.
- Prefer small, focused commands under `cmd/`. Avoid cyclic deps between packages.
- Indentation uses tabs, not spaces.

### Testing Guidelines
- Framework: standard `testing` package.
- File naming: mirror source with `_test.go`. Example: `internal/config/config_test.go`.
- Run locally: `go test -v ./...`. Aim for meaningful coverage on config parsing and Redis interactions (use a test Redis or mocks).
- Template rendering: test `internal/newsletter` rendering with sample data.

### Commit & Pull Request Guidelines
- Commits: imperative mood and scoped (e.g., `cmd: add redis ping command`).
- PRs: include summary (What/Why), linked issues, and CLI output when helpful (e.g., ping result). Keep PRs small and focused.

### Security & Configuration Tips
- Do not commit secrets. Use env overrides: `QUAILY_REDIS_ADDR`, `QUAILY_REDIS_PASSWORD`, etc.
- Config search paths: project root, `$HOME/.config/quaily-journalist/`, `./configs/`.
- Example `config.yaml` provided; create a local variant if needed.
- Channels live under `newsletters.channels[]` with fields: `name`, `source`, `nodes`, `frequency`, `top_n`, `min_items`, `output_dir`, `item_skip_duration`, `preface`, `postscript`.

### Service (systemd)
- Example unit: `configs/quaily-journalist.service.example`.
- Ensure the binary path and working directory match your deployment.
- Newsletters are written to `newsletters.output_dir/<channel_name>/:frequency-YYYYMMDD.md` (UTF‑8).

