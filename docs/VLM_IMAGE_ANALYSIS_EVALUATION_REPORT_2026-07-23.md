# VLM Image-Analysis Evaluation Report — 2026-07-23

## Scope and verdict

This is the evidence record for image **analysis**, not image generation. It
evaluates whether image-bearing, tool-using OpenAI Responses traffic can be
routed safely through the configured image-only targets. It does not activate
`/v1/images`, image editing, video generation, or provider image-generation
tools.

**Verdict:** the GPT-5.4, Grok 4.5, and OpenRouter Claude Sonnet 4.6 routes
passed the recorded OpenAI Responses image/tool checks at a realistic
512-token output budget. MiniMax M3 passed protocol and streaming checks but
had inconsistent OCR on repetition, so it remains a staging-only candidate.

## Test contract

The evaluation used a synthetic public receipt image and a forced function
tool. The expected tool argument identified the receipt merchant as `Rite
Aid`. The tests exercised:

- OpenAI Responses `input_text` plus `input_image` forwarding;
- forced function-tool generation;
- server-sent-event streaming completion and tool-call events;
- a realistic `max_output_tokens: 512` cap;
- router request-shape filtering, so image-only targets cannot receive
  text-only traffic.

No raw prompts, image URLs, response bodies, credentials, router tokens, or
tool schemas are retained in this report.

## Evaluation matrix

| Target and route skin | Direct upstream | Local router: image + forced tool | Local router: streaming | OCR outcome | Disposition |
| --- | --- | --- | --- | --- | --- |
| Direct OpenAI GPT-5.4 / OpenAI Responses | Passed | `200`, function call observed | `200`, function call and completed SSE observed | Correct | Validated reference target |
| xAI Grok 4.5 / OpenAI Responses | Passed | `200`, function call observed | `200`, function call and completed SSE observed | Correct | Eligible only with 512-token minimum |
| OpenRouter Claude Sonnet 4.6 / OpenAI Responses | Passed | `200`, function call observed | `200`, function call and completed SSE observed | Correct | Eligible only with 512-token minimum |
| Direct MiniMax M3 / OpenAI Responses | Passed for protocol | `200`, function call observed | `200`, function call and completed SSE observed | Inconsistent on repeated OCR run | Staging-only; no production image weight |

`200` means the protected local smoke received a successful HTTP response;
the tool and OCR columns are separate evidence. A transport success alone is
not a quality pass.

## Cap and compatibility findings

| Target | Finding | Routing consequence |
| --- | --- | --- |
| Grok 4.5 | At lower requested output caps, the upstream returned substantially more output than requested; the 512-token run behaved acceptably. | Keep `min_requested_output_tokens: 512`; never raise a caller cap silently. |
| OpenRouter Claude Sonnet 4.6 | Lower-cap probing did not establish reliable OCR; 512-token tool and streaming checks passed. | Keep the 512-token minimum. |
| Kimi K3 | OpenAI Chat and Anthropic Messages passed earlier compatibility probes; native OpenAI Responses returned `404`. | Exclude it from Responses image routes; do not infer cross-skin compatibility. |
| MiniMax M3 | A repeated OCR result was not reliable despite successful request/tool/stream protocol behavior. | Keep cataloged only in the static staging smoke group until quality evidence is repeatable. |

## Router and production evidence

The router gate requires image-bearing request shapes for the image-only
targets. The following production checks were completed after rollout:

- Ten image/tool requests completed successfully in each of the
  `big-coder` and `big-coder-latest` groups.
- Five text-only requests completed successfully in each group.
- Safe usage readback found no selection of an image-only target for a
  text-only request.
- Safe selection readback observed GPT-5.4, Grok 4.5, and Claude Sonnet 4.6
  as image-target selections. The check used only provider/model/dialect and
  request-shape counters, not request content or credentials.

The production rollout evidence is summarized on
[issue #556](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/556#issuecomment-5054717532).

## Nano Banana Pro fixture attempt

Nano Banana Pro was used only to attempt generation of a **synthetic test
receipt**; it is not part of the router's product surface or routing plan.
The first attempt was stopped after an account-budget problem. After the
account was reported funded, a 2K retry remained pending for more than five
minutes without writing an output file and was stopped to avoid an unbounded
paid request. A smaller 1K retry was started, but the work was interrupted
before it returned.

Therefore there is **no generated fixture artifact and no Nano Banana OCR
evaluation result**. Do not describe that fixture as tested or commit one
until a completed asset is visually inspected, its text is readable, and it
passes the local-router forced-tool and streaming checks above.

## Reproducible follow-up

1. Generate or choose a synthetic, non-sensitive receipt image.
2. Visually inspect it and record only expected scalar fields (for example,
   merchant name) in a test-side assertion; do not commit raw prompts,
   private images, or API data.
3. Run `scripts/local_image_analysis_smoke.sh` against every static smoke
   group with the image fixture.
4. Require HTTP success, a valid function call, a completed stream, and a
   correct OCR assertion on repeated runs before promotion.
5. Repeat the production image and text-only negative checks at low routing
   weight, then retain only safe scalar usage evidence.

## Linked material

- [Validation runbook](VLM_IMAGE_ANALYSIS_VALIDATION.md)
- [Local smoke script](../scripts/local_image_analysis_smoke.sh)
- [Production rollout tracking issue #556](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/556)
