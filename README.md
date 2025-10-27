# Quaily Journalist

This is a tool by [Quaily](https://quaily.com).

Collect, score, and publish Markdown newsletters from [V2EX](https://v2ex.com) and [Hacker News](https://news.ycombinator.com/news). Quaily Journalist runs small workers that poll sources (V2EX nodes, Hacker News lists derived from channel nodes), rank posts using a Hacker‑News‑like formula, store them in Redis, and periodically render channel‑specific Markdown files (daily/weekly) using a simple template.

Use it to generate a daily or weekly digest you can post, email, or archive.

## Live Channels (Chinese)

- [V2EX 日报](https://quaily.com/v2ex-daily)
- [V2EX 日报（生活版）](https://quaily.com/v2ex-daily-lifestyle)
- [V2EX 日报（投资版）](https://quaily.com/v2ex-daily-investment)

## Features

- V2EX collector by node with configurable poll interval
- Hacker News collector by list (top/new/best/ask/show/job)
- HN‑like time‑decayed scoring using replies and post age
- Redis storage with sensible TTLs and period ZSETs (daily, weekly)
- Channel builder per source with filters, min/top thresholds, and skip logic
- Markdown rendering via a text/template (easy to customize)
- AI-powered summaries (OpenAI) for item descriptions and a post summary
- CLI with `serve`, `generate`, and `redis ping`
- Configurable via YAML config only (no env overrides)
- Systemd‑friendly service unit example

## Quick Start

Prerequisites: Go 1.21+, Redis (local or remote).

1) Copy and edit `config.yaml` to your environment. At minimum, set Redis and pick V2EX nodes. To enable AI summaries, set the OpenAI section in the config file.

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

Optional: generate from a URL list file using Cloudflare Browser Rendering (Markdown endpoint). Provide one URL per line in a text file:

```bash
go run . generate <channel> -i urls.txt
```
This fetches each URL via Cloudflare Browser Rendering, keeps input order (no scores), and renders normally. Requires `cloudflare.account_id` and `cloudflare.api_token` in config.

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

Example (redacted) configuration (config file only; no env overrides):

```yaml
app:
  log_level: "info"

redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

openai:
  api_key: ""
  model: "gpt-4o-mini"
  base_url: ""  # optional, e.g., https://api.openai.com/v1

sources:
  v2ex:
    token: ""      # Optional
    base_url: "https://www.v2ex.com"
    fetch_interval: "10m"
  hackernews:
    base_api: "https://hacker-news.firebaseio.com/v0"
    fetch_interval: "10m"

cloudflare:
  # Cloudflare account ID used to build the fixed scrape endpoint URL.
  # Docs: https://developers.cloudflare.com/browser-rendering/rest-api/
  account_id: ""   # required
  api_token: ""    # Cloudflare API token with Browser Rendering permissions
  timeout: "20s"

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
      language: "English"  # Language used for AI outputs
      template:
        title: ""  # optional; default: "Digest of <channel> <YYYY-MM-DD>"
        preface: "Your daily V2EX highlights."
        postscript: "Brought to you by Quaily Journalist."
      # Template variables supported in template fields (title/preface/postscript):
      # - {.CurrentDate} -> YYYY-MM-DD (UTC)
```

## CLI

- `go run . --help` — show CLI help
- `go run . serve` — run service (collector + builders + scheduler)
- `go run . generate <channel>` — force‑generate today’s post for `<channel>` (writes `:output_dir/:channel/:frequency-YYYYMMDD.md` if at least `min_items` are available; ignores published/skip)
- `go run . generate <channel> -i urls.txt` — generate from a URL list file; fetches each URL via Cloudflare Browser Rendering Markdown endpoint, keeps input order (no scores)
- `go run . redis ping` — ping Redis using current config
- `go run . publish <markdown_path> <channel_slug>` — publish a rendered Markdown file to Quaily now
- `go run . send <path_or_slug> <channel_slug>` — deliver a Quaily post now; if `<path_or_slug>` is a file, reads its frontmatter `slug`, otherwise treats it as the slug directly

Make targets:

- `make build` — compile to `bin/quaily-journalist`
- `make test` — run unit tests
- `make vet` — static checks via `go vet`


## Output

- Files are UTF‑8 Markdown under `newsletters.output_dir/<channel>/`
- Daily slug format: `daily-YYYYMMDD.md` (e.g., `out/v2ex_daily_digest/daily-20251023.md`)
- Frontmatter includes `summary`, and the same summary appears near the top of content

## Quaily Publishing

- Create a new channel at [https://quaily.com](https://quaily.com)
- Get your API token from [Quaily Dashboard](https://quaily.com/dashboard/profile/apikeys#)
- Add a `quaily` block to `config.yaml` to enable auto‑publish after each render:

```yaml
quaily:
  base_url: "https://api.quaily.com/v1"
  api_key: "YOUR_TOKEN"
  timeout: "10s"
```

> the channel will be published to the channel matching the channel name. for example, if the channel is `v2ex-daily`, the post will be published to the Quaily channel with slug `https://quaily.com/v2ex-daily`.

 - The `serve` command publishes to Quaily right after writing each Markdown file, then delivers (sends) the post 5 seconds later. It uses the file’s frontmatter as Create Post parameters, adds the Markdown body as `content`, and uses the channel name as `channel_slug`. It then calls Create Post and Publish Post, followed by Deliver.
- Use `go run . publish <markdown_path> <channel_slug>` to manually publish a specific file.

## Run as a Service (systemd)

See Deployment for systemd setup and operations:

- Deployment and service setup: [Deployment.md](./Deployment.md)

## Development

- Architecture and internals: [Architecture.md](./Architecture.md)
