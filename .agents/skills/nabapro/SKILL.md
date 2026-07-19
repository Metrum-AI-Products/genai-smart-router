---
name: nabapro
description: Use this skill when the user needs to generate, edit, or iterate on images with Google's Gemini image-generation API (Nano Banana Pro). Trigger on requests for product visuals, brand/campaign imagery, text-in-image rendering, reference-based editing, style transfer, video-to-image, multi-turn refinement, or any Gemini image workflow. Applies Metrum AI branding by default unless the user provides another brand.
---

# Nabapro

Use `gemini-3-pro-image` (Nano Banana Pro) as the default model. This skill bundles a CLI that wraps the Gemini Interactions API. Use `--dry-run` to inspect the request JSON before any API call.

## Brand

By default nabapro injects the Metrum AI logo and a detailed brand style prompt into every generation. This gives images the correct look — dark backgrounds, red-to-blue gradient accents, Geist/Geist Mono typography, thin technical rules, subtle grid cues.

Read [`references/metrum-ai-brand.md`](references/metrum-ai-brand.md) for the full brand guide. The bundled logo is at `assets/metrum-logo-white.png`.

Control with `--brand`:

| Value | Behavior |
| --- | --- |
| `metrum` (default) | Injects brand style prompt + logo as object+style reference |
| `none` / `off` | No brand — fully custom |
| `/path/to/logo.png` | Uses your logo as object+style reference |
| `--no-brand-style` | Suppresses all brand injection even when `--brand` is set |

When the user requests a different brand or no brand, respect their choice. Pass `--brand none` and describe the desired brand in the prompt.

## Workflow

1. **Clarify requirements** — Final use, aspect ratio, exact text, brand constraints, whether edits must preserve identity/layout. Default to Metrum if brand is not mentioned.
2. **Research (if needed)** — For factual/current visuals enable Google Search grounding with `--google-search --search-types web_search,image_search` and model preset `flash`.
3. **Draft the prompt** — Highly specific: subject, scene, camera/design language, materials, lighting, exact text (in quotes), constraints, negative instructions. For Metrum branding, reference the brand guide; for custom work, describe the aesthetic explicitly. The model needs explicit visual direction even with a style reference.
4. **Run with `--dry-run` first** — Inspect request JSON before any paid API call.
5. **Run with `nabapro.py`** — Use `--previous-interaction-id` for follow-ups. Save all returned images, thought/citation/search artifacts. Return file paths and the interaction ID.
6. **Never print** the `GEMINI_API_KEY`.

## Quick Start

```bash
uv run skills/nabapro/scripts/nabapro.py --help

# Default: Metrum-branded image
uv run skills/nabapro/scripts/nabapro.py \
  --prompt "A hero image for a product launch page with a glowing red-to-blue gradient accent line." \
  --aspect-ratio 16:9 --image-size 4K \
  --output-dir ./nabapro-output --dry-run

# No brand — fully custom look
uv run skills/nabapro/scripts/nabapro.py \
  --brand none \
  --prompt "A minimalist Apple-style product shot, white background..." \
  --aspect-ratio 16:9 --image-size 4K \
  --output-dir ./nabapro-output

# Editing an existing image
uv run skills/nabapro/scripts/nabapro.py \
  --prompt "Overlay this logo on a tech dashboard background." \
  --image ./existing-design.png \
  --aspect-ratio 4:5 --image-size 2K \
  --output-dir ./nabapro-output
```

Set `--model-preset flash` for high-throughput or video-to-image. Use `--previous-interaction-id` for edits.

## Key constraints

- **Video-to-image** requires `--model-preset flash`. If API rejects with "Audio input modality not enabled", retry with `--strip-video-audio`.
- **Multi-turn** — Keep the returned interaction ID; pass `--previous-interaction-id`.
- **Model IDs**: `gemini-3-pro-image-preview` and `gemini-3.1-flash-image-preview` were **retired June 25, 2026**. Use only GA model IDs.
- **Image rights** — Verify rights to uploaded images. Avoid deceptive/infringing transformations.

## Reference

- `scripts/nabapro.py` — Full CLI options via `--help`
- `references/gemini-image-api.md` — Model limits, request/response schemas, and API options
- `references/metrum-ai-brand.md` — Metrum AI brand guide (colors, typography, composition, logo usage, prompting examples)
- `assets/metrum-logo-white.png` — Bundled white logo for style+object injection
