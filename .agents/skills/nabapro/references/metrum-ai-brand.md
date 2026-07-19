# Metrum AI Brand Guide

Source of truth: [Metrum AI website](https://www.metrum.ai/). Refresh from the official site before production work when possible.

## Logo

- **White logo** (dark backgrounds): `assets/metrum-logo-white.png` (2244×405 RGBA)
- **Inverted logo**: download from the official site for light backgrounds or use a white-background version processed separately
- Preserve aspect ratio; do not redraw or recreate as text
- Keep clear space around the logo
- Use on title frames, closing frames, and hero visuals; avoid on content-heavy frames where it competes

## Color Palette

| Role | Value |
| --- | --- |
| Background | Near-black or black |
| Primary text | White |
| Secondary text | Restrained gray |
| Accent gradient | `#FF3132 → #FE005F → #EE0089 → #CC28AF → #9948CB → #465CDA` |

Do not introduce unrelated accent colors.

## Typography

| Role | Font | Notes |
| --- | --- | --- |
| Display headlines | **Geist** | Light or regular weight, tight tracking |
| Body copy | **Poppins** | Explanatory labels and body text |
| Technical labels | **Geist Mono** | Small uppercase, metadata, timestamps, CTAs; modest letter spacing |

Load fonts from the official site or use licensed local equivalents. Do not substitute a different visual family when a Metrum asset is available.

## Composition

- Thin technical rules, subtle grid/topology cues
- Square or minimally rounded UI elements
- Deliberate empty space
- Restrained motion: gentle opacity fades, short line reveals, data-flow motion
- Avoid: bouncy motion, cartoon illustration, glossy 3D, rounded consumer-app cards, generic template styles

## Generating Metrum-branded images with Nabapro

The `--brand` flag (default `metrum`) automatically includes the bundled white logo as a style reference and applies Metrum design language.

```bash
# Default — Metrum branded
uv run skills/nabapro/scripts/nabapro.py \
  --prompt "A cinematic product photo of a futuristic AI dashboard, dark background, red-to-blue gradient accents, technical grid overlay, Geist font text" \
  --aspect-ratio 16:9 --image-size 4K \
  --output-dir ./nabapro-output

# Custom brand — pass your own logo as style reference
uv run skills/nabapro/scripts/nabapro.py \
  --brand /path/to/client-logo.png \
  --prompt "..." \
  --output-dir ./output

# No brand — fully custom
uv run skills/nabapro/scripts/nabapro.py \
  --brand none \
  --prompt "..." \
  --output-dir ./output
```

## Prompting for Metrum styling

The model needs explicit visual direction even with a style reference. Always include these elements in the prompt when Metrum branding is desired:

- **Background**: dark or near-black
- **Text colors**: white primary, restrained gray secondary
- **Accents**: red-to-blue gradient (`#FF3132 → #465CDA`)
- **Typography**: Geist (headlines), Geist Mono (labels)
- **UI style**: thin rules, subtle grid/topology cues, square elements, generous whitespace
- **Avoid**: glossy 3D, cartoon illustrations, rounded cards, bouncy or playful aesthetics

Example:

```
"A hero image for a product launch page. Dark near-black background with a subtle diagonal grid overlay. A glowing red-to-blue gradient accent line sweeps from left to right. The UI element shown is a minimal square dashboard card with white text reading 'ANALYTICS' in Geist Mono. Thin white technical rules frame the composition. Clean, premium, technical aesthetic. Do not use rounded corners, cartoon style, or glossy 3D effects."
```

Override with `--brand none` and adjust the prompt for a different brand system.
