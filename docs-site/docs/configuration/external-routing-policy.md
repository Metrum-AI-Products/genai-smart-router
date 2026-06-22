---
title: External Routing Policy Service
---

# External Routing Policy Service

Use `strategy: external` when a deployment wants routing decisions to come from a standalone policy service instead of from TypeScript running inside the router. Callers still request one deployment-defined model group. The router filters the group to eligible targets, sends safe request context to the policy service, validates the returned target, and then calls the selected upstream.

This is useful when the routing policy should be developed, tested, deployed, and observed as its own service. For smaller local rules, see [TypeScript Routing Policy](./routing-typescript).

## Admin Setup

Configure a model group with `strategy: external`, a policy URL, an exact host allowlist, and the targets the policy may choose from:

```yaml
models:
  adaptive:
    strategy: external
    external_policy:
      url: https://routing-policy.internal.example/route
      allow_hosts: [routing-policy.internal.example]
      timeout_ms: 500
      max_response_bytes: 65536
      headers:
        Authorization: ${ROUTING_POLICY_AUTH_HEADER}
      on_error: fail_closed
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

`on_error` defaults to `fail_closed`. Use `fallback` only when the group is allowed to use the normal configured target order if the policy service is unavailable or returns an invalid decision.

Callers continue to use the model group name:

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "adaptive",
    "messages": [{"role": "user", "content": "Summarize this note in one sentence."}],
    "max_tokens": 128
  }'
```

## Policy Request

The router sends a JSON `POST` body to the policy service:

```json
{
  "group": "adaptive",
  "text": "Summarize this note in one sentence.",
  "inputModalities": ["text"],
  "requirements": ["text", "max_tokens"],
  "request": {
    "model": "adaptive",
    "messages": [{"role": "user", "content": "Summarize this note in one sentence."}],
    "max_tokens": 128
  },
  "caller": {
    "id": "team-prod",
    "user": "team",
    "project": "product",
    "environment": "prod",
    "tokenId": "rtr_metrum_team_product_prod_k20260621",
    "allow": ["adaptive"]
  },
  "targets": [
    {
      "provider": "baseten",
      "model": "openai/gpt-oss-120b",
      "modelRef": "gpt-oss-120b",
      "dialect": "openai-chat",
      "tier": "cheap",
      "weight": 70,
      "inputPricePerMillionUsd": 0.10,
      "outputPricePerMillionUsd": 0.50,
      "toolSupport": {"openaiChat": ["tools", "tool_choice"]},
      "inputModalities": ["text"],
      "outputModalities": ["text"],
      "keyId": "baseten-primary",
      "apiKeyEnv": "BASETEN_API_KEY",
      "keyConfigured": true
    }
  ],
  "now": "2026-06-21T12:00:00Z"
}
```

`targets` contains only targets already eligible for the request shape. For example, image requests only include image-capable targets, tool requests only include compatible tool targets, and capped requests skip targets marked as not honoring max tokens.

The router does not send raw router tokens, token hashes, provider API keys, or full deployment config.

## Policy Response

Return one target decision:

```json
{
  "targetIndex": 1,
  "fallbackIndexes": [0],
  "classLabel": "prompt-size:heavy",
  "metadata": {"reason": "large prompt"}
}
```

The router accepts:

- `targetIndex`: zero-based index into the request's `targets` array.
- `target`: selector such as `{ "provider": "baseten", "model": "openai/gpt-oss-120b" }`.
- `fallbackIndexes` or `fallbacks`: optional fallback order.
- `classLabel`: optional label stored in logs and usage records.

Returned targets are validated against the eligible target list. A policy service cannot select an unconfigured provider/model, a target outside the requested group, or a target filtered out for tools, modalities, or token-cap behavior.

## Tested Demo Service

The repository includes a runnable prompt-size policy service:

```bash
python3 examples/external-routing-policy/prompt_size_policy.py
```

It listens on `http://127.0.0.1:18090/route`, sends short prompts to a `cheap` target, sends prompts over 8,000 characters to a `heavy` target, and returns the same response schema shown above.

The reference config includes an example group named `external-policy-demo`:

```yaml
models:
  external-policy-demo:
    strategy: external
    external_policy:
      url: http://127.0.0.1:18090/route
      allow_hosts: [127.0.0.1]
      timeout_ms: 500
      max_response_bytes: 65536
      on_error: fail_closed
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

Names such as `external-policy-demo`, `cheap`, and `heavy` are examples. Deployments can choose their own model group names and target metadata.

## Security And Operations

- Keep policy services inside trusted infrastructure because they receive prompt text, message context, tool schemas, image references or image data, caller metadata, pricing metadata, and target capability metadata.
- Use exact `allow_hosts`; wildcard host allowlists are not supported.
- Put policy-service authentication in `external_policy.headers`, not in application requests.
- Keep `timeout_ms` low because routing happens before any upstream model call.
- Use `fail_closed` for sensitive routing policy. Use `fallback` only when the configured target order is an acceptable default.
- Treat `routing-policy-error` as a deployment/configuration issue. The response means the policy service failed, timed out, returned non-JSON, returned non-2xx, or selected an invalid target.
