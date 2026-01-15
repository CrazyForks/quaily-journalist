# Feature Plan: Cover Image Generation (2026-01-15)

## Goal
Generate an infographic-style cover image for each newsletter post using the Susanoo image API with model `gemini-2.5-flash` (16:9), then set `cover_image_url` in the Markdown frontmatter so Quaily uses it as the post cover.

## Notes / Open Items
- Susanoo image generation returns base64 data that expires (~20 minutes); we must decode and persist the image locally.
- The README does not state the base URL host; keep `susanoo.base_url` configurable.
- Confirm desired image dimensions/aspect ratio and whether the post language should be reflected in the infographic text.

## Proposed Plan
1) Lock the Susanoo image-generation contract
- Endpoint: `POST /images/generations?async=0` with header `X-SUSANOO-KEY: <api_key>`.
- Request body: `{ "model": "gemini-2.5-flash", "prompt": "...", "n": 1, "provider": "gemini", "gemini_options": { ... } }` (optional fields as needed).
- Response: `data.results[].b64_json` (base64-encoded image) and `data.expires` (about 20 minutes).
- Decode base64 (Gemini likely returns PNG) and convert to WebP before storing as `cover.webp`.

2) Add configuration for image generation
- Introduce a new config block (example name: `susanoo` or `image_generation`) with:
  - `base_url`
  - `api_key`
  - `model` (default `gemini-2.5-flash`)
  - Optional: `timeout`, `size`, `aspect_ratio`, `style`, `prompt_template`, `cache_ttl`
- Update `config.example.yaml` and README with the new block.

3) Implement a small image generation client
- Create `internal/imagegen` (or extend `internal/ai`) with an interface like:
  - `GenerateCover(ctx, prompt, outPath) (string, error)` returning `cover_image_url`.
- Implement a Susanoo client that:
  - Builds request payload per README spec.
  - Adds the required auth headers.
  - Decodes `b64_json`, converts PNG -> WebP, and writes `cover.webp` to disk.
  - Uses timeouts and structured logs.

4) Build a prompt for infographic covers
- Inputs: channel title, short summary, top item titles/nodes, language.
- Output: a concise prompt that requests an infographic layout, minimal text, clear hierarchy.
- Allow a config override for prompt template.

5) Integrate into newsletter generation
- Extend `internal/newsletter.Data` with `CoverImageURL`.
- Update `internal/newsletter/newsletter.tmpl` to conditionally include:
  - `cover_image_url: "..."` when present.
- In both `worker/newsletter_builder.go` and `cmd/generate.go`:
  - After summaries are computed, call image generation.
  - Set `CoverImageURL` on the template data.
  - If the call fails, log and continue without a cover.
  - Save the decoded image under `out/<channel>/<slug>/cover.webp` where `<slug>` is the markdown filename without `.md` (e.g., `daily-20251114/cover.webp`).
  - Upload the WebP to Quaily Attachment API and use `data.view_url` as `cover_image_url` when Quaily is configured.
  - Use relative `cover_image_url` as `<slug>/cover.webp` if upload is unavailable.
  - Optionally cache by `channel + period + prompt hash` to avoid repeats.

6) Publishing
- `internal/quaily/publish.go` already forwards frontmatter. No new publish logic needed.

7) Tests / QA
- Unit test for prompt builder output.
- Template test to assert `cover_image_url` appears only when set.
- HTTP client test with a stub server for success/failure/timeouts.

## Deliverables
- New config block and documentation updates.
- Susanoo image generation client.
- Cover image URL included in generated Markdown frontmatter.
- Tests for prompt and template behavior.
