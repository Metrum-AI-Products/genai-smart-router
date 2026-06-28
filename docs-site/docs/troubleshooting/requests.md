---
title: Request Troubleshooting
---

# Request Troubleshooting

Every request returns an `X-Request-Id` header. Use that ID to join caller symptoms to logs, usage rows, upstream attempts, trace events, terminal errors, and browser report drilldown.

## 1. Capture The Caller View

Record these safe fields:

- UTC timestamp;
- request ID;
- HTTP status;
- router error code, if present;
- requested model group;
- API shape, such as Chat Completions, Responses, or Anthropic Messages;
- client name, such as Codex CLI, Claude Code, Cursor, or an internal service;
- whether the request used streaming, tools, images, or a large output cap.

Do not record raw prompts, image payloads, bearer tokens, provider keys, tool outputs, or full request bodies unless a governed content-capture process is explicitly enabled for the deployment.

## 2. Check Caller Access

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Expected: the requested model group appears in the response. If it is absent, the caller token is not allowed to use that group or the group is not configured.

Common access outcomes:

| Status | Meaning | Operator action |
|---|---|---|
| `401` | Missing, malformed, expired, or invalid caller token. | Issue or rotate the caller token. |
| `403` | Caller is authenticated but not authorized for the surface or model group. | Review caller access, admin policy, or metrics/report role. |
| `403 metrics-forbidden` | Ordinary caller attempted `/metrics`. | Use a metrics-admin token only for metrics scraping. |
| `403 reports-forbidden` | Ordinary caller attempted admin reports. | Use an authorized admin report identity. |

## 3. Check Quota And Token Admission

Router-side quota and admission failures usually return `429`. Distinguish them from upstream provider `429` attempts:

- Router quota failures appear as the terminal caller response.
- Upstream `429` attempts may be followed by fallback to another target.
- Large-context developer tools can exhaust TPM through in-flight reservations even when daily or monthly budget remains available.

Use usage reports or admin browser troubleshooting buckets for quota, TPM/RPM, concurrency, input-token, and max-token signals.

## 4. Check Upstream Attempts

For a slow or failed request, inspect:

- selected provider/model/dialect;
- upstream status code;
- upstream duration and TTFB;
- timeout or cancellation flags;
- retryability;
- fallback transitions;
- sanitized terminal error class.

If only one provider/model/dialect is failing, isolate that upstream before changing the broader model group. If all targets are failing, inspect shared config, network, license, database, or caller request shape.

## 5. Check Request Shape

Common request-shape causes:

- requested model group does not support the API skin used by the client;
- tool calls are sent to a target without validated tool support;
- image input is sent to a text-only target;
- output cap or context length exceeds target limits;
- a forced tool-choice shape is unsupported by the selected upstream;
- streaming behavior differs from the caller expectation.

Model-group contracts and provider catalog metadata should describe validated modalities, tools, dialects, pricing, and max-token behavior.

## 6. Verify Recovery

After a config, credential, quota, or upstream fix:

```bash
curl -fsS "$ROUTER_BASE_URL/readyz"

curl -fsS "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "replace-with-allowed-model-group",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 16
  }'
```

Then confirm the request appears in usage reports with the expected provider/model, status, latency, cost, and fallback state.
