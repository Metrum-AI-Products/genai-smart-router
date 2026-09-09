# External Routing Policy Service Runbook

Use `strategy: external` when a model group delegates target selection to a deployment-owned web service.

Public customer-facing strategy ownership guidance lives in `docs-site/docs/routing/customer-controlled-routing.md`. Keep this runbook aligned with that page when changing external-policy behavior, security boundaries, proof requirements, or examples.

## Policy Design Checklist

- Define the workload and owner.
- Choose whether this belongs in one model group, multiple groups, or a separate router instance.
- Choose the strategy: `static`, `failover`, `weighted`, `dynamic_score`, `script`, `external`, or a contract-backed combination.
- Define eligible providers and models under `models.<group>.targets[]`; do not treat provider catalog entries as active routes.
- Document required API shapes, tool modes, modalities, reasoning controls, structured-output support, and max-token cap behavior.
- Define quality, cost, latency, throughput, error-rate, timeout, and fallback targets.
- Run direct upstream smokes for every provider/model/dialect/skin being claimed.
- Run router-level smokes through each caller API shape and negative no-eligible-target path.
- Run representative evaluation or proof for the workload.
- Define rollback: remove the target from affected groups, remove or tighten the capability metadata that made it eligible, isolate it behind a restricted smoke group, relax a contract only when the contract is too strict, switch strategy, or restore the previous config.

## Configuration

```yaml
models:
  adaptive:
    strategy: external
    external_policy:
      url: https://routing-policy.internal.example/route
      mode: shadow
      allow_hosts: [routing-policy.internal.example]
      timeout_ms: 500
      max_response_bytes: 65536
      headers:
        Authorization: ${ROUTING_POLICY_AUTH_HEADER}
      on_error: fail_closed
      include_request: false
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

The external routing policy service receives derived request context, caller
identity metadata (`id`, user, project, environment, public token ID, and
allowed groups), and only targets that remain eligible after request-shape and
model-group contract filtering. Eligible-target metadata includes provider,
model/model-ref, dialect, tier, weight, pricing, capability/validation fields,
configured key ID, API-key environment-variable name, and whether that
variable is configured. These fields are useful for policy but disclose
deployment inventory and pseudonymous caller identity, so the policy service
and transport are trusted infrastructure. `targets[].region` is not included.

By default the request does not include prompt text, message bodies, image
URLs/data, tool schemas, tool outputs, or `request.raw`; route on fields such
as `context.textChars`, `context.estimatedTokens`, `context.imageCount`, and
`context.toolCount`. Set `external_policy.include_request: true` only when the
service is trusted to receive request content. If the model group enables
`pii_filter`, that opt-in request mirror is built from the redacted request
object and placeholder mappings remain request-local. The service never
receives raw router tokens, caller token hashes, provider API key values, or
full router config.

`external_policy.mode` controls reversible activation. Omitted mode keeps the
backward-compatible `enforce` behavior. In `shadow`, the router calls the
policy, validates and records its recommendation, and still serves configured
eligible target order. Promote by changing reviewed config to `enforce`. Roll
back to `baseline` to preserve eligible configured order without calling the
policy. Shadow failures never affect the served target. All three modes still
require a valid `url` and matching `allow_hosts` at config validation time, so
promotion or rollback cannot activate an unreviewed destination.

Policy URLs should use HTTPS. Plain HTTP is accepted only for trusted loopback hosts such as `localhost`, `127.0.0.1`, and `::1`, or when `external_policy.allow_http: true` is explicitly configured for a trusted non-local endpoint. `allow_hosts` is exact-host matching, not a suffix or wildcard rule. Redirects are revalidated before each hop; a redirect to any host outside `allow_hosts`, including a loopback address that was not listed, fails before the redirected service is reached.

The router also validates destination IPs and ports. Non-loopback private and
link-local destinations are denied even when the hostname is allowlisted;
nonlocal HTTPS uses port 443 and HTTP uses port 80. For a private policy service,
use loopback in the same network namespace (for example a sidecar in the same
pod). A separate private service on a custom port is rejected. Deployment network
policy and firewall controls supplement these checks.

For outcome-trained target selection and its protected offline training CLI, see
[Learned Routing Policy](LEARNED_ROUTING_POLICY.md).

## Local Demo

Run the committed prompt-size policy service:

```bash
python3 examples/external-routing-policy/prompt_size_policy.py
```

The sample `external-policy-demo` group in `config.example.yaml` points at `http://127.0.0.1:18090/route`. It routes requests with `context.textChars > 8000` to targets tagged `tier: heavy` and shorter requests to targets tagged `tier: cheap`.

For observed-signal scoring, conversation pins, cache-hit exclusion, and serving-target fallback attribution, run the adaptive reference:

```bash
python3 scripts/run_adaptive_signal_policy_demo.py
# or: make adaptive-signal-policy-demo
```

That example is a trusted deployment-owned policy service, not built-in router state and not provider-backed quality evidence.

## Validation Checklist

- Validate YAML with `rtk go test ./internal/router -run 'ExternalRoutingPolicy|Config'`.
- Confirm policy service auth is configured through `external_policy.headers`, not source code.
- Confirm `allow_hosts` contains exact hostnames only.
- Confirm non-local policy URLs use HTTPS unless `external_policy.allow_http: true` was explicitly approved.
- Confirm redirects to non-allowlisted hosts fail and do not reach the redirected service.
- Run a router smoke for a short prompt and a long prompt, then check selected upstream model.
- Run an image or tool request when the group supports VLM/tool traffic and confirm the default policy payload contains only derived counts/requirements plus eligible target metadata, not image URLs/data, tool schemas, or tool outputs.
- If `external_policy.include_request: true` is approved, confirm the policy payload is redacted as expected for groups with `pii_filter` and document why the external service may receive request content.
- Confirm errors are clear: policy timeout, non-2xx, invalid JSON, and invalid target should return `502 routing-policy-error` unless `on_error: fallback` is explicitly configured.
- Confirm caller cancellation reaches the policy request. One concurrency-safe policy HTTP client is created per external-policy group at startup and reused; an unset timeout defaults to 500 ms.
- With decision telemetry enabled, confirm policy success, fail-closed error, and configured `on_error: fallback` requests write safe `request_policy_executions` rows and do not store policy request/response JSON, prompt text, tool schemas, provider keys, token hashes, policy headers, or full config.
- In `shadow`, confirm `request_policy_executions.outcome=shadow_recommended`, a `shadow_recommended_candidate` routing signal, and a routing decision for the baseline target actually served.

## Production Rollout

- Add the policy-backed group with a private caller and `mode: shadow` first.
- Keep policy egress on HTTPS. If plaintext HTTP is required for trusted internal infrastructure, document the reason for `external_policy.allow_http: true`.
- Keep `timeout_ms` small, typically 200-500 ms.
- Prefer `on_error: fail_closed` for policy-sensitive traffic.
- Use `on_error: fallback` only when the configured target order is explicitly approved as the default policy.
- After rollout, monitor `request_usage.error_class`, `request_trace_events`, `request_policy_executions`, selected provider/model, latency, and class labels.
- Promote with `mode: enforce` only after shadow evidence passes. Roll back policy influence with `mode: baseline`; switching to `weighted`/`static` remains a broader policy rollback.

## Outcome-Calibrated Reference

`examples/external-routing-policy/outcome_calibrated_policy.py` demonstrates a
deployment-owned calibration loop for task classes with different quality and
cost requirements. It has no router-side control plane and never writes active
configuration. The reference uses explicit exemplar classes, runs each
synthetic case through a restricted external-policy group, records reviewer
JSONL outcomes, rejects candidates below a class quality gate, and emits a
reviewable policy profile plus target-weight YAML patch.

The live service uses `external_policy.include_request: true` and an
OpenAI-compatible embeddings endpoint to match trusted redacted request content
to approved exemplars. The existing policy request already supplies the
eligible targets, model/provider identifiers, pricing, capabilities, request
shape, and safe caller metadata required for selection; do not add provider
keys, token hashes, or full config to the policy contract. Unknown or ambiguous
requests select the profile's designated strong default, not a cheap candidate.

The three committed cases are a workflow demonstration, not promotion evidence.
Use a deployment-owned larger dataset and retain the default minimum reviewed
sample gate before applying a generated patch. Keep response-bearing reviewer
JSONL in the trusted deployment boundary and never commit captured production
content. Roll back by restoring the last approved profile and target weights or
by disabling the external group.

Run `make outcome-calibrated-synthetic-demo` for the repeatable local
regression fixture. It uses fake embeddings and mock upstreams, writes an
ignored bundle with the profile, review-only YAML patch, and router test log,
and must never be treated as provider-quality evidence.

For provider-backed evidence, deploy the reference with a secret-held OpenAI
embedding key and `text-embedding-3-small`, collect real candidate responses,
verify each outcome, calibrate the profile, then run
`scripts/run_outcome_calibrated_live_demo.py` against a dedicated staging group.
Require a private policy request header and authenticated audit endpoint. Use a
unique `--run-id` so a cached response cannot skip policy selection.
