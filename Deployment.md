# Deployment

This guide covers running Quaily Journalist locally and as a service.

## Prerequisites

- Go 1.21+
- Redis (local or remote)

## Build and Run

- Build a local binary:

```bash
make build
./bin/quaily-journalist --help
```

- Run from source during development:

```bash
go run . --help
go run . redis ping
go run . serve
```

## Configuration

Quaily Journalist reads `config.yaml` from one of:

- `./config.yaml` (project root)
- `$HOME/.config/quaily-journalist/config.yaml`
- `./configs/config.yaml`

You can also pass `--config /path/to/config.yaml`.

See the README for a full example and field descriptions.

Cloudflare (for URL-list generation)

- To use `generate -i urls.txt`, configure:
  - `cloudflare.account_id`: your Cloudflare account ID.
  - `cloudflare.api_token`: API token with Browser Rendering permissions.

## Run as a Service (using systemd)

An example unit file is provided at `gears/quaily-journalist.service.example`.

1) Copy and edit the unit file, updating fields required
2) Ensure your `config.yaml` is accessible from one of the search paths (or pass `--config`).
3) Enable and start:

```bash
sudo systemctl enable quaily-journalist
sudo systemctl start quaily-journalist
sudo journalctl -u quaily-journalist -f
```

## Operating

- Generate on demand (ignores published/skip):

```bash
go run . generate <channel>
```

- Output files are UTF‑8 Markdown under `newsletters.output_dir/<channel>/` with daily slugs like `daily-YYYYMMDD.md`.

For Quaily publish integration, see the README section “Quaily Publishing”.
