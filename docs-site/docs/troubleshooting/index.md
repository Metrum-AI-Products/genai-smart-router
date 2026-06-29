---
title: Troubleshooting
doc_type: howto
---

# Troubleshooting

Use this section to triage customer-visible failures without exposing secrets or private deployment details. Start with the request ID, the caller-visible error code, and the affected model group.

From Troubleshooting, you might be looking for the canonical error catalog: see [Error Responses](/docs/reference/errors).

## First Checks

```bash
export ROUTER_BASE_URL="https://llm-api.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/version"
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

If `/readyz` fails, check license status, config load errors, database connectivity, and required runtime files before testing individual requests.

If `/v1/models` does not include the expected model group, inspect caller access policy and model-group configuration. `/v1/models` is the caller-facing source of truth for allowed groups.

## Triage Map

| Symptom | Likely area | Next page |
|---|---|---|
| One request failed, timed out, or was slow | Request attempts, trace events, upstream provider path, quota, context, or fallback | [Request Troubleshooting](/docs/troubleshooting/requests) |
| `/readyz` fails with `license-*` status | License file, expiry, feature gate, volume, instance scope, or clock state | [Licensing Troubleshooting](/docs/troubleshooting/licensing) |
| Caller gets `401` or `403` | Missing token, disabled key, model-group access, metrics/report authorization, or admin policy | [Request Troubleshooting](/docs/troubleshooting/requests) |
| Caller gets `429` | Router quota, traffic shaping, TPM/RPM, concurrency, license volume/window, or upstream provider limit | [Request Troubleshooting](/docs/troubleshooting/requests) |
| `/metrics` returns forbidden | Caller is not authorized for metrics admin | [Observability](/docs/operations/observability) |
| Admin reports unavailable | Admin authentication, Casbin policy, usage DB, or report feature license | [Admin Browser Reports](/docs/operations/admin-browser-reports) |

## Safe Support Packet

When escalating, include:

- router version and build timestamp from `/version`;
- exact UTC time window;
- request ID values;
- caller-visible error code and HTTP status;
- requested model group;
- client name and version when relevant;
- safe license status fields when licensing is involved;
- a redacted summary of the workflow.

Do not include raw router tokens, provider keys, token hashes, raw prompts, raw images, raw tool outputs, full production config, private hostnames, SSH details, private signing material, or full customer-specific license payloads.
