# Gemini Image API Reference

Current source: <https://ai.google.dev/gemini-api/docs/image-generation> and <https://ai.google.dev/api/interactions-api>. Re-check before changing model limits or request fields. Checked **July 19, 2026**.

## Preferred API

Use the Gemini Interactions API for current image-generation features:

```http
POST https://generativelanguage.googleapis.com/v1beta/interactions
x-goog-api-key: $GEMINI_API_KEY
Content-Type: application/json
```

Use the `GEMINI_API_KEY` environment variable. Do not print it.

Core request fields:

- `model`: use `gemini-3-pro-image` for Nano Banana Pro. The preview alias `gemini-3-pro-image-preview` was retired on **June 25, 2026** and no longer works.
- `input`: string or content blocks. Content blocks include text and image blocks.
- `response_format`: image output format object or an array such as `[{"type":"text"},{"type":"image"}]`.
- `generation_config`: model controls such as `temperature`, `top_p`, `seed`, `stop_sequences`, `thinking_level`, `thinking_summaries`, and `max_output_tokens`.
- `tools`: tool declarations, including `{"type":"google_search"}`.
- `previous_interaction_id`: continue a prior image interaction.
- `response_modalities`: requested modalities such as `text` and `image`.
- Other top-level fields include `system_instruction`, `stream`, `store`, `background`, `cached_content`, `environment`, `service_tier`, and `webhook_config`.

## Models

- Nano Banana Pro / Gemini 3 Pro Image: `gemini-3-pro-image`. Premium model for complex visual tasks, professional asset production, advanced localization, brand consistency, precision creative control, 4K output, Google Search grounding, and complex multi-turn editing. GA release date: **May 28, 2026**. Knowledge cutoff: **January 2025**. Input token limit: 65,536. Output token limit: 32,768. Pricing: ~$0.134/image (1K/2K), ~$0.24/image (4K); Batch API cuts cost ~50%.
- Nano Banana 2 / Gemini 3.1 Flash Image: `gemini-3.1-flash-image`. Faster generalist image model. 2K: ~$0.101/image; 4K: ~$0.15/image.
- Nano Banana 2 Lite / Gemini 3.1 Flash Lite Image: `gemini-3.1-flash-lite-image`. Fastest and cheapest Gemini image model; ~$0.034/image at 1K only. Not optimized for multiple references or sequential editing.
- Nano Banana / Gemini 2.5 Flash Image: `gemini-2.5-flash-image`. Legacy Nano Banana series model. ~$0.039/image.

Nabapro defaults to `gemini-3-pro-image` intentionally. Use model presets only when a lower-cost or Flash-specific capability is desired.

All generated images include a SynthID watermark.

## Input Blocks

Text block:

```json
{"type": "text", "text": "Create a detailed image..."}
```

Image block:

```json
{"type": "image", "data": "BASE64_ENCODED_IMAGE", "mime_type": "image/png"}
```

Image blocks may also use `uri` instead of `data` where supported:

```json
{"type": "image", "uri": "https://example.com/reference.png", "mime_type": "image/png"}
```

Supported image input MIME types in the Interactions API reference include `image/png`, `image/jpeg`, `image/webp`, `image/heic`, `image/heif`, `image/gif`, `image/bmp`, and `image/tiff`. Image input blocks also support a `resolution` hint: `low`, `medium`, `high`, or `ultra_high`.

Gemini 3 image workflows support up to 14 total reference images. Nano Banana Pro supports high-fidelity reference use for up to 6 object images, up to 5 character-consistency images, and up to 3 style-reference images within that total.

Video block:

```json
{"type": "video", "uri": "https://www.youtube.com/watch?v=...", "mime_type": "video/mp4"}
```

Video-to-image is documented for `gemini-3.1-flash-image`. Public YouTube URLs can be passed directly. For large local video files, use the Files API and pass the resulting URI via `--video-json`.

Observed live API behavior: YouTube URLs or local videos with audio tracks can fail with `Audio input modality is not enabled for this model`, even when using `gemini-3.1-flash-image`. For local videos, strip audio before upload. For YouTube URLs, use a local silent/no-audio video or Files API video URI if the public URL fails.

When a user specifically asks to make a YouTube URL work despite that failure, an optional workaround is to use `uvx yt-dlp` to download a video-only or no-audio MP4, then pass that local file with `--video --strip-video-audio`. Do not require `yt-dlp` for normal image work.

## Output Format

Image response format:

```json
{
  "type": "image",
  "mime_type": "image/jpeg",
  "aspect_ratio": "16:9",
  "image_size": "4K"
}
```

Supported image output fields:

- `type`: always `image`.
- `mime_type`: `image/jpeg`.
- `aspect_ratio`: `1:1`, `2:3`, `3:2`, `3:4`, `4:3`, `4:5`, `5:4`, `9:16`, `16:9`, `21:9`, `1:8`, `8:1`, `1:4`, `4:1`.
- `image_size`: `512`, `1K`, `2K`, `4K`. Use uppercase `K`; lowercase values can be rejected. Nano Banana Pro supports up to 4K.

Current size limits:

- `gemini-3-pro-image`: `1K`, `2K`, `4K`
- `gemini-3.1-flash-image`: `512`, `1K`, `2K`, `4K`
- `gemini-3.1-flash-lite-image`: `1K`

Text+image interleaved output:

```json
[
  {"type": "text"},
  {"type": "image", "aspect_ratio": "16:9", "image_size": "2K"}
]
```

Do not rely only on SDK convenience properties such as `output_image` for interleaved output. Iterate through returned `steps[].content[]` and save every image block.

## Google Search Grounding

Use:

```json
{"type": "google_search"}
```

Optional `search_types` values include `web_search`, `image_search`, and `enterprise_web_search`. Use grounding when the prompt depends on current weather, recent events, stock charts, maps, factual visual details, or current reference imagery.

`image_search` grounding is documented for `gemini-3.1-flash-image`. Save and surface `google_search_result.search_suggestions` and citation metadata when present; image-search grounding has display requirements.

## Thinking Artifacts

Gemini 3 image models can return `thought` steps with text and interim image summaries. Save these separately when present, but do not rely on them as final output. `gemini-3.1-flash-image` supports `generation_config.thinking_level` values `minimal` and `high`.

## Multi-Turn Editing

Use `previous_interaction_id` with a follow-up prompt:

```json
{
  "model": "gemini-3-pro-image",
  "input": "Change the poster language to Spanish. Do not alter layout or imagery.",
  "previous_interaction_id": "v1_...",
  "response_format": {"type": "image", "aspect_ratio": "16:9", "image_size": "2K"}
}
```

Keep the returned interaction ID in the final answer when further edits are likely.

## Batch Jobs

Use the Gemini Batch API for large image-generation runs. This skill's CLI is optimized for interactive foreground generation, editing, and iteration.
