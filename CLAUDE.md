# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Quaily Journalist is a Go-based newsletter aggregation service that collects, scores, and publishes Markdown newsletters from V2EX and Hacker News. It runs background workers that poll sources, rank posts using a time-decayed scoring formula, store them in Redis, and periodically render channel-specific Markdown files.

## Common Commands

```bash
# Run from source (development)
go run . --help
go run . serve                    # Start service (collectors + builders)
go run . generate <channel>       # Force-generate today's newsletter
go run . generate <channel> -i urls.txt  # Generate from URL list via Cloudflare
go run . redis ping               # Test Redis connectivity
go run . publish <path> <slug>    # Publish markdown to Quaily
go run . send <path|slug> <slug>  # Deliver a Quaily post

# Testing
go test -v ./...

# Format code
go fmt ./...
```

## Architecture

### Data Flow
```
Sources (V2EX/HN APIs)
    ↓ (10-min intervals)
Collectors → compute HN-like scores → Redis ZSETs + JSON items
    ↓ (30-min intervals)
Builders → filter by nodes/thresholds → AI summarize → render Markdown
    ↓
Output files (out/<channel>/daily-YYYYMMDD.md)
    ↓ (optional)
Quaily API → publish + deliver
```

### Key Directories
- `cmd/` - Cobra CLI commands (serve, generate, publish, send, redis)
- `worker/` - Background workers (v2ex_collector, hn_collector, newsletter_builder, manager)
- `internal/` - Non-public packages:
  - `config/` - Configuration structs
  - `storage/` - Redis operations (CRUD, published flags, skip markers)
  - `v2ex/`, `hackernews/` - Source API clients
  - `ai/` - OpenAI summarization
  - `newsletter/` - Markdown template rendering
  - `scrape/` - Cloudflare Browser Rendering client
  - `quaily/` - Quaily API client for publishing

### Redis Key Patterns
- `news:source:<source>:period:<YYYY-MM-DD>` - ZSET of item IDs with scores
- `news:item:<source>:<id>` - JSON item data (7-day TTL)
- `news:published:<channel>:<date>` - Published flag
- `news:skip:<channel>:<id>` - Skip marker (configurable TTL)

### Scoring Formula
```go
score = (replies - 1) / (hours_since_created + 2)^1.8
```

## Configuration

Config loaded from (in order): `./config.yaml`, `$HOME/.config/quaily-journalist/config.yaml`, `./configs/config.yaml`, or `--config` flag.

Key sections: `redis`, `openai`, `quaily`, `cloudflare`, `sources` (v2ex, hackernews), `newsletters.channels`.

## Code Style

- **Use tabs for indentation** (not spaces)
- Keep commands small and focused under `cmd/`
- Avoid cyclic dependencies between packages
