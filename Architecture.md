# Architecture

This document describes how Quaily Journalist is put together: the main components, data flow, Redis keys, and how to develop and test locally.

> This is a golang repository, always use Tab for code indentation.

## Overview

- Cobra CLI initializes Viper to load `config.yaml`.
- A Redis client is created via `internal/redisclient` and shared.
- Workers run in the background:
  - Collectors fetch items from sources (V2EX, Hacker News), compute HN‑like scores, and store results in Redis period ZSETs (daily/weekly) plus item JSON.
  - Builders run per channel, filter items by nodes and thresholds, render Markdown with a template, write files, and mark published/skip keys.
- Optional AI summaries (OpenAI) generate per‑item descriptions and a top summary used in frontmatter and near the top of the content.

## Components

- CLI and config
  - `main.go` boots Cobra commands under `cmd/` and loads config from YAML (no env overrides).
  - `internal/config` holds types and defaults; see README for config schema and example.
  - `internal/redisclient` creates a Redis connection from config.

- Collectors (sources)
  - V2EX (`worker/v2ex_collector.go`, `internal/v2ex`):
    - Polls the union of all V2EX nodes referenced across all channels.
    - Skips topics with zero replies.
    - Computes a score from replies and age (time‑decay) and `ZADD`s into period sets.
  - Hacker News (`worker/hn_collector.go`, `internal/hackernews`):
    - Derives HN lists to poll from the union of channel nodes (e.g., `top`, `new`, `best`, `ask`, `show`, `job`).
    - Scores using comment count and age; stores alongside V2EX in per‑period sets.

- Builder (`worker/newsletter_builder.go`)
  - Runs per channel. Filters the period ZSETs by the channel’s `source`/`nodes` and skip markers.
  - Enforces `min_items` and `top_n`.
  - Renders Markdown with `internal/newsletter` template and writes to `out/<channel>/` (`daily-YYYYMMDD.md`, etc.).
  - Marks published + skipped in Redis so repeated runs don’t duplicate work.

- Manager (`worker/manager.go`)
  - Starts collectors and builders with their configured intervals; coordinates shutdown.

- AI summaries (`internal/ai/openai.go`)
  - If `openai` is configured in `config.yaml`, item descriptions and a post summary are produced and injected into the template variables.

## Data Flow and Keys

- Collector (≈10m):
  - V2EX API → normalize → score → `ZADD news:source:v2ex:period:<YYYY-MM-DD>` and `SET news:item:v2ex:<id>`
  - Hacker News API → normalize → score → `ZADD news:source:hackernews:period:<YYYY-MM-DD>` and `SET news:item:hackernews:<id>`
- Builder (≈30m): `ZREVRANGE` → filter nodes/skip → threshold → template render → write file → mark published + skipped

Sample keys (for 2025‑10‑23):

- `news:item:v2ex:123456` — JSON of the topic (7‑day TTL)
- `news:source:v2ex:period:2025-10-23` — ZSET of IDs with scores
- `news:item:hackernews:4201337` — JSON of the HN item (7‑day TTL)
- `news:source:hackernews:period:2025-10-23` — ZSET of IDs with scores
- `news:published:v2ex_daily_digest:2025-10-23` — flag for published period
- `news:skip:v2ex_daily_digest:123456` — skip marker (e.g., 72h TTL)

## Directory Layout

```
cmd/                 # Cobra commands (root + subcommands)
internal/            # Non-public packages (config, clients, storage, newsletter, ai)
worker/              # Long-running workers (collector, builder, manager)
gears/               # Example assets (e.g., systemd unit)
config.yaml          # Application configuration (not committed)
main.go              # CLI entrypoint
Makefile             # Build/test helpers
README.md            # Project overview and user docs
Architecture.md      # This document
Deployment.md        # Deployment guide (systemd, run modes)
```

## Development & Testing

- Go 1.21+. Format with `go fmt ./...`.
- Indentation uses tabs (not spaces).
- Keep commands small and focused under `cmd/`; avoid cyclic dependencies.
- Run tests: `go test -v ./...` or `make test`.
- Static checks: `make vet`.
- Template rendering: test `internal/newsletter` with sample data.

## Security Notes

- Do not commit secrets. Place secrets in a local `config.yaml` at one of the supported search paths.
- Prefer config search paths and local untracked variants.
