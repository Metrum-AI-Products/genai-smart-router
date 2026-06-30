---
title: Providers And Models
doc_type: explanation
---

# Providers And Models

Providers and model catalogs describe what upstream endpoints exist, how to authenticate to them, what model IDs they serve, which request shapes they have validated, and how usage should be priced. Catalog metadata does not send traffic by itself. Traffic starts only when a cataloged model is referenced under a caller-visible model group in `models.<group>.targets[]`.

Provider examples in public docs are validation patterns. A provider, account, region, or model may be unavailable in a particular deployment until entitlement, pricing, request shape, direct upstream behavior, and router-level behavior are validated.

Before active routing, validate entitlement for the exact provider key, account/project/region, model ID, dialect, and request shape. Provider `401`, `403`, and `404` responses can mean invalid credentials, missing entitlement, policy/privacy restriction, region/project restriction, or model access denial. Keep those targets catalog-only or in a restricted smoke group until the exact shape passes.

## Catalog, Smoke Group, Active Target

| State | Meaning | Typical Use |
| --- | --- | --- |
| Catalog-only | The provider model is described under `providers.*.models.*` but no active group target references it. | Discovery, future rollout, unavailable entitlement, failed or incomplete validation. |
| Smoke group | A dedicated model group routes only to the candidate target and is limited to operators or test callers. | Direct-router validation before production weight. |
| Active target | One or more caller-visible groups reference the target with a positive weight or priority. | Production traffic after workload acceptance gates pass. |
| Disabled or zero weight | The target remains documented but receives no ordinary traffic. | Rollback, incident isolation, or temporary entitlement issues. |

## Onboarding Workflow

1. Verify current provider documentation for endpoint base URL, authentication, model IDs, context limits, modalities, tool support, structured-output support, streaming behavior, usage reporting, and pricing.
2. Add environment placeholders for provider credentials where applicable. Public examples should use placeholders such as `${PROVIDER_API_KEY}` and must not include raw keys.
3. Add provider catalog metadata with `base_url`, `dialect`, auth settings, `key_id`, served model ID, modalities, pricing fields, pricing source/date, and safe capability notes.
4. Run direct upstream smokes for every API skin and capability that will be claimed.
5. For coding-agent or retrieval-heavy groups, run a synthetic large-payload smoke with representative message count, tool schemas, tool outputs, and explicit output caps. Record `context_tokens` and optional `request_shape_support` limits before broad activation.
6. Add a smoke-only model group and run the same requests through the router.
7. Confirm usage, request-time price inputs, calculated cost, latency, upstream attempts, fallback status, estimated tokens, context headroom, eligibility skips, and safe decision telemetry are recorded.
8. Add the target to production groups conservatively after workload acceptance passes.
9. Update public and operator docs when the new provider, capability, API behavior, or rollout process is user-visible.

For the detailed reference process, see [Add A Provider Or Model](../reference/add-provider-model) and [Model Metadata](../reference/model-metadata).

## Provider Catalog Metadata

```yaml
providers:
  hosted_openai_compatible:
    base_url: https://provider.example.com/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${PROVIDER_API_KEY}
    api_key_env: PROVIDER_API_KEY
    key_id: provider-primary
    models:
      balanced-text:
        model: provider/model-id
        input_modalities: [text]
        output_modalities: [text]
        input_price_per_million_usd: 0.20
        output_price_per_million_usd: 0.80
        pricing_notes: Keep pricing source and update-date evidence in config.example.yaml alongside deployment validation notes and provider-specific billing caveats.
        honors_max_tokens: true
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
        request_shape_support:
          max_estimated_input_tokens: 120000
          max_requested_output_tokens: 8192
          max_tool_schema_bytes: 100000
          supports_large_coding_agent_payloads: true
```

Keep routing weights out of provider catalogs. Weights belong only under model groups:

```yaml
models:
  provider-smoke:
    strategy: static
    targets:
      - provider: hosted_openai_compatible
        model_ref: balanced-text
```

## Validation By API Skin

Validate each skin independently. A model that passes one request surface is not automatically compatible with another. Tool-bearing traffic requires explicit `tool_support` metadata for the exact skin, including Anthropic Messages `client_tools`.

| API Skin | Validate |
| --- | --- |
| OpenAI Chat Completions | Non-streaming text, streaming if used, `max_tokens` or `max_completion_tokens` caps, tool calls, forced `tool_choice`, `response_format`, usage fields, and provider error shape. |
| OpenAI Responses | Text, streaming if used, `max_output_tokens`, function tools, `text.format`, usage fields, and client compatibility for Responses-based agents. |
| Anthropic Messages | Text, streaming if used, `max_tokens`, client tools, image blocks if claimed, thinking controls if claimed, and Anthropic-compatible clients. |
| Vision or image input | Real image request with a realistic output budget, not only a tiny token cap. For OCR routes, require the expected OCR answer. |
| Reasoning or thinking | Direct and router-level requests for the exact reasoning field, budget or effort control, streaming behavior, and max-token interaction. |
| Structured outputs | Direct and router-level schema requests for the exact dialect; run combined tool plus structured-output smokes when both are claimed. |
| Max-token caps | Tiny cap requests such as `max_tokens: 1` where the caller contract depends on cap forwarding. |

Do not claim tool support, image support, reasoning support, structured outputs, or max-token cap behavior from marketing copy alone. Use provider docs as discovery input, then promote only after direct upstream and router-level evidence passes for the exact provider, model ID, account, dialect, and skin.

Some upstream models expose more than one compatible API skin. Configure those as separate provider entries when the deployment validates them separately. For example, a deployment can expose MiniMax `MiniMax-M3` as an OpenAI Chat skin for Cursor-style traffic, a MiniMax OpenAI Responses skin for Codex-style traffic, and a MiniMax Anthropic Messages skin for Claude-compatible traffic. Each skin keeps its own dialect, tool metadata, reasoning notes, smoke group, and rollback path.

## Upstream Payload Controls

The router owns provider-side persistence policy for OpenAI-compatible passthrough. Caller-supplied provider `metadata` is stripped, and OpenAI Chat/Responses passthrough sends `store: false` upstream only when the resolved target sets `force_store_false: true`. OpenAI Chat passthrough also uses target metadata such as `output_token_field` to choose `max_tokens` or `max_completion_tokens`. Tool schemas and structured-output schemas still pass through to compatible targets, but their serialized size contributes to token-budget admission.

Large coding-agent payload support is not implied by a model name, a context-window claim, or an ordinary text smoke. Validate the exact provider/model/dialect/account with realistic request bytes, tool schema size, requested output cap, and router translation path. If a target is useful for small requests but not validated for large agent payloads, keep it in smoke groups or configure explicit `request_shape_support` limits such as `max_request_bytes`, `max_estimated_input_tokens`, `max_tool_schema_bytes`, or `supports_large_coding_agent_payloads: false`.

Use a sanitized synthetic fixture for this validation, not captured customer prompts. The repository includes `scripts/large_payload_chat_smoke.py` for OpenAI Chat-compatible targets; it builds safe filler messages plus representative tool schemas, prints only scalar shape metrics, and can test either a direct upstream endpoint or a router model group. For example, Fireworks Chat validation rerun on 2026-06-30 used a 524 KB fixture with 24 tools, about 50 KB of serialized tool schemas, and about 91K prompt tokens before keeping the tested Chat route marked as large-agent validated. Production reruns require a deployment-owned smoke group and a safe existing caller token; missing production caller or report access should be recorded as a blocker rather than worked around with copied secrets.

Upstream HTTP redirects are not followed. A 301, 302, 303, 307, or 308 response is treated as an upstream failure instead of replaying the prompt, image, tool, or schema payload to the redirect target. Successful upstream response bodies are bounded by `server.upstream.max_response_bytes` before decode or synthesized streaming.

## Hosted And Private Upstreams

Hosted providers, OpenAI-compatible aggregators, private vLLM or SGLang services, and enterprise-hosted model gateways all use the same catalog and model-group concepts. The operational differences are mostly in endpoint ownership, network controls, pricing or chargeback source, and validation responsibility.

| Upstream Type | What To Verify |
| --- | --- |
| Hosted provider | Account entitlement, region, billing, model ID, provider quota, current pricing, usage fields, and provider-specific schema limits. |
| Aggregator | Exact routed model ID or suffix, provider allow list, account routing behavior, pass-through support for tools/images/structured outputs, and billing metadata. |
| Private/self-hosted | Served model ID from `/models`, chat template or parser settings, tool-call parser behavior, context limits, GPU capacity, network isolation, and internal chargeback. |

For private OpenAI-compatible services, see [Self-Hosted Upstreams](../configuration/self-hosted-upstreams).

## Rollout And Rollback

Start with catalog-only metadata, then use a smoke group with restricted caller access. Move to active groups only after representative workload tests pass and reports show expected usage, cost, latency, and fallback behavior.

Rollback should usually be a config-only change:

- remove the target from affected groups;
- move traffic to a failover-safe target;
- tighten or remove `request_shape_support` metadata if context/request-size validation was wrong;
- remove an unsafe capability label such as `structured_outputs`, `image`, or tool support;
- keep the catalog entry with dated notes when the model still exists but is not safe for active traffic.

After rollback, run `/readyz`, a caller request for the affected group, and a usage/report check that confirms another eligible target was selected.
