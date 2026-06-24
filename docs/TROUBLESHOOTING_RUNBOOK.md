# Troubleshooting Runbook

Use this runbook for production issues reported by users or monitoring.

## First Checks

Capture:

- exact timestamp in UTC;
- caller-visible error body;
- `X-Request-Id`;
- requested endpoint and model group;
- client type, such as Codex CLI, Claude Code, Warp, SDK, or custom app;
- whether the request included tools, images, streaming, or a low token cap.

Do not ask users for provider keys or raw router tokens.

## Health And Version

```bash
rtk curl -fsS https://llm-api-engg.metrum.ai/readyz
rtk curl -fsS https://llm-api-engg.metrum.ai/version
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose ps'
```

## Request ID Investigation

Use `X-Request-Id` to inspect relational usage tables:

- `request_usage`: terminal status, caller, selected target, token counts, cost fields, cache status.
- `request_attempts`: upstream provider/model attempts, status, duration, timeout/cancel flags.
- `request_trace_events`: routing decisions, fallback, cache, timeout, terminal failure.
- `request_errors`: sanitized terminal error class/message.

Diagnostic tables must not store raw prompts, raw image payloads, raw tokens, token hashes, provider keys, full upstream headers, or unsanitized upstream response bodies.

## Common Cases

### `no-eligible-target`

Check the requirements in the error body. The fix is normally a configuration change: enable or add a target in the requested model group that supports the requested dialect, tools, modalities, and max-token cap behavior.

### Timeout

Check upstream attempt durations, provider status, client timeout settings, router upstream timeout config, and whether the model is reasoning-heavy with too small a token budget. Run both direct upstream and router smokes with realistic `max_tokens`.

### Rate Limit Or Quota

Check rpm, tpm, concurrency, and daily/monthly/lifetime request and token caps. Use usage reports to confirm whether the block is expected. Keep caller policies consistent unless there is an explicit product reason for different tiers.

For token-budget rejections, inspect the caller's requested output cap as well as recent actual usage. The router reserves estimated input tokens plus `max_tokens`, `max_completion_tokens`, or `max_output_tokens` before upstream calls, and TPM, daily, monthly, and lifetime checks include other in-flight reservations. A small prompt can be rejected near a budget if it asks for a very large possible output. Failed or canceled upstream calls release the reservation, while successful calls reconcile to actual reported usage.

### Bad Image Analysis

Separate transport success from task quality. A model can remain active for general VLM routing even if it is not good enough for OCR-specific routing. For OCR routes, require exact-answer image smokes.

### Tool-Calling Failure

Validate the exact client dialect:

- Warp and many OpenAI-compatible agents use `/v1/chat/completions` tool passthrough.
- Codex CLI uses `/v1/responses`.
- Claude Code uses `/v1/messages`.

Run the appropriate real tool smoke and assert file contents, not only assistant text.

## Logs

Recent router logs:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose logs --tail=200 router'
```

Prefer DB traces for request-level details because logs should remain sanitized and compact.
