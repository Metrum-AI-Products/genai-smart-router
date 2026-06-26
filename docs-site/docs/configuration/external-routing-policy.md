---
title: External Routing Policy Service
---

# External Routing Policy Service

Use `strategy: external` when a deployment wants routing decisions to come from a standalone policy service instead of from TypeScript running inside the router. Callers still request one deployment-defined model group. The router filters the group to eligible targets, applies any optional model-group contract, sends safe request context to the policy service, validates the returned target, and then calls the selected upstream.

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
      # Off by default. Set true only for a trusted policy service that is allowed
      # to receive raw/redacted request content.
      include_request: false
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

`on_error` defaults to `fail_closed`. Use `fallback` only when the group is allowed to use the normal configured target order if the policy service is unavailable or returns an invalid decision.

Policy URLs use HTTPS by default. Plain HTTP is accepted only for trusted loopback hosts such as `localhost`, `127.0.0.1`, and `::1`, or when `external_policy.allow_http: true` is explicitly set for a trusted non-local endpoint. Redirects are revalidated before they are followed; every hop must keep an allowed `http`/`https` scheme and an exact hostname from `allow_hosts`.

Hostname allowlisting is not a replacement for deployment network controls. Use firewall, service-mesh, or cloud egress policy for private-network and CIDR restrictions until native CIDR egress controls are added.

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
  "context": {
    "model": "adaptive",
    "dialect": "openai-chat",
    "estimatedTokens": 9,
    "textChars": 36,
    "messageCount": 1,
    "messageTextChars": 36,
    "imageCount": 0,
    "toolCount": 0,
    "hasTools": false,
    "hasStructuredOutput": false,
    "maxTokens": 128,
    "maxTokensField": "max_tokens",
    "temperatureSet": false,
    "stream": false,
    "reasoning": {
      "requested": true,
      "kind": "effort",
      "effort": "medium",
      "source": "openai_chat.reasoning_effort"
    }
  },
  "inputModalities": ["text"],
  "requirements": ["text", "max_tokens"],
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

By default, the policy request does not include raw prompt text, normalized message bodies, image URLs or base64 data, tool result text, tool schemas, or `request.raw`. Use the derived `context` object for routing signals such as prompt size, estimated token count, message count, image count, tool count, structured-output presence, explicit output cap, streaming flag, safe metadata key names, and normalized reasoning or thinking fields.

If a deployment needs a trusted policy service to inspect request content, set `external_policy.include_request: true`. That opt-in adds `request` and `text` fields to the policy body. When the model group has `pii_filter` enabled, those fields are built from the redacted request object; placeholder mappings stay in router memory for the current request and are not sent to the policy service. Without `pii_filter`, `include_request: true` can send raw prompts/messages, image references or data, tool schemas, and tool outputs to the external service.

`targets` contains only targets already eligible for the request shape. For example, image requests only include image-capable targets, tool requests only include compatible tool targets, reasoning or thinking requests only include compatible reasoning targets, and capped requests skip targets marked as not honoring max tokens. Each target includes safe capability metadata such as modalities, tool support, structured-output support, reasoning support, validation status, prices, and configured key identifiers.

When the group has a model-group contract, the policy body includes safe `contract` metadata and target `validation` metadata. `targets[]` is already filtered by the contract, and policy responses are validated against that eligible list. The policy service cannot select a contract-ineligible fallback.

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

The deployment examples include a runnable prompt-size policy service:

```bash
python3 examples/external-routing-policy/prompt_size_policy.py
```

It listens on `http://127.0.0.1:18090/route`, sends requests with `context.textChars <= 8000` to a `cheap` target, sends larger requests to a `heavy` target, and returns the same response schema shown above.

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
      include_request: false
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

Names such as `external-policy-demo`, `cheap`, and `heavy` are examples. Deployments can choose their own model group names and target metadata.

## Security And Operations

- Keep policy services inside trusted infrastructure because they receive request-shape context, caller metadata, pricing metadata, and target capability metadata. They receive prompt text, message content, tool schemas, image references or image data, and tool outputs only when `external_policy.include_request: true` is explicitly configured.
- Leave `include_request` off unless the policy service has the same trust boundary as the router and the deployment has approved content sharing. Prefer derived fields such as `context.textChars`, `context.estimatedTokens`, `context.imageCount`, and `context.toolCount`.
- Use exact `allow_hosts`; wildcard host allowlists are not supported.
- Use HTTPS for non-local policy services. Use `external_policy.allow_http: true` only for an approved trusted internal endpoint; loopback HTTP is reserved for local demos and sidecars.
- Treat redirects as policy-service egress: a redirect to a non-allowlisted hostname fails before the redirected service is reached.
- Put policy-service authentication in `external_policy.headers`, not in application requests.
- Keep `timeout_ms` low because routing happens before any upstream model call.
- Use `fail_closed` for sensitive routing policy. Use `fallback` only when the configured target order is an acceptable default.
- Treat `routing-policy-error` as a deployment/configuration issue. The response means the policy service failed, timed out, returned non-JSON, returned non-2xx, or selected an invalid target.

When decision telemetry is enabled, external-policy executions write safe scalar policy rows. Successful policy decisions record outcome, selected candidate index, fallback count, and safe class label. Fail-closed policy errors before selection record an execution row even when no routing-decision row exists, with policy kind, outcome, duration, eligible/all target counts, safe error class, and terminal error type. `on_error: fallback` records the configured fallback outcome and then normal routing/fallback telemetry explains the selected target. These rows do not store the policy request body, policy response JSON, prompt text, tool schemas, bearer tokens, provider keys, token hashes, raw URLs, policy headers, or full config.
