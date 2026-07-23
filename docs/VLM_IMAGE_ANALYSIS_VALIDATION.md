# VLM Image-Analysis Validation

## Purpose

This runbook covers **image analysis only**: OCR, screenshot and document
understanding, and image-bearing agent tool calls. It does not configure image
generation, image editing, video generation, or any provider-hosted image tool.

The router treats an image route as a request-shape contract. A candidate is
eligible only when its exact provider/model/API skin has passed the relevant
tests. Provider marketing claims and success on a text request are not enough.

## Client contracts

### Codex and OpenAI Responses clients

Codex-style clients can send a `POST /v1/responses` request with an `input`
message containing `input_text` and `input_image` content parts. For agent
workloads, validate all of the following together:

- image URL and data-URL forwarding;
- forced function tools and correctly shaped `function_call` results;
- `stream: true` SSE completion and tool-call events;
- exact caller `max_output_tokens` forwarding, including rejection or
  pre-selection filtering for unsupported caps;
- realistic coding-agent tool schemas and output budgets.

### Claude Code and Anthropic Messages clients

Claude Code gateway traffic uses `POST /anthropic/v1/messages`. Native vision
requests use `image` content blocks with a URL or base64 source, while tool
workloads require valid `tool_use` / `tool_result` continuation behavior.
Validate image input, client tools, streaming, requested `max_tokens`, and any
configured Anthropic-to-upstream translation separately. A target that passes
Responses is not automatically eligible for Claude Code.

## Required routing gates

Every active image-analysis target must set:

```yaml
request_shape_support:
  required_input_modalities: [image]
  supported_inbound_dialects: [openai-responses]
```

Use `min_requested_output_tokens` when the provider is only reliable at a
realistic requested output budget. The router must skip the target for smaller
explicit caps; it must never silently increase the caller's cap. Image-only
targets must be absent from ordinary text selection.

## Local smoke groups

`config.example.yaml` provides caller-restricted static groups:

| Group | Target | Disposition |
| --- | --- | --- |
| `image-analysis-smoke-gpt54` | Direct OpenAI GPT-5.4 | Validated reference target |
| `image-analysis-smoke-grok45` | Direct xAI Grok 4.5 | Validated at 512+ output tokens |
| `image-analysis-smoke-claude46` | OpenRouter Claude Sonnet 4.6 | Validated fallback at 512+ output tokens |
| `image-analysis-smoke-minimax-m3` | Direct MiniMax M3 | Staging canary only |

Run the local test with protected provider variables already present in the
environment:

```bash
scripts/local_image_analysis_smoke.sh
```

The script starts a temporary `dev_no_license` router, sends a synthetic receipt
as an OpenAI Responses image request with a forced function tool, and repeats
the request with SSE streaming. It writes no credentials, prompts, images, or
responses to the repository.

## Local evidence (2026-07-23)

The timestamped execution record, including the unsuccessful synthetic-fixture
attempt, is in the [evaluation report](VLM_IMAGE_ANALYSIS_EVALUATION_REPORT_2026-07-23.md).

| Target | Image OCR + forced tool | Streaming tool call | Result |
| --- | --- | --- | --- |
| GPT-5.4 | Passed | Passed | Eligible reference target |
| Grok 4.5 | Passed | Passed | Eligible only with a 512-token minimum |
| Claude Sonnet 4.6 via OpenRouter Responses | Passed | Passed | Eligible only with a 512-token minimum |
| MiniMax M3 | Protocol passed; OCR was inconsistent on repeat | Passed | Keep staging-only; do not assign production image weight |

The MiniMax result is intentionally not generalized to other MiniMax skins.
Revalidate OpenAI Chat, OpenAI Responses, and Anthropic Messages independently.

## Promotion checklist

1. Run direct upstream text, image OCR, forced tool, streaming, and cap-bucket
   tests for the exact account/model/skin.
2. Run the same tests through its static local smoke group.
3. Verify safe usage rows: provider, model, dialect, image counters, status,
   attempts, fallback state, and request-time costs.
4. Verify a text-only negative test does not select the image-only target.
5. Add a sanitized production-derived fixture before production rollout.
6. Start at low weight behind the current validated image route, monitor error,
   latency, OCR/tool outcome, and cost, and restore the prior config on breach.

## Non-goals

- No `v1/images/*` routing.
- No generated-image model activation.
- No raw prompts, images, image URLs, tool schemas, tool output, API keys, or
  caller tokens in logs, reports, fixtures, or commits.
