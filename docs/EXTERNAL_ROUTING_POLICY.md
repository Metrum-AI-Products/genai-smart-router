# External Routing Policy Service Runbook

Use `strategy: external` when a model group delegates target selection to a deployment-owned web service.

## Configuration

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

The external routing policy service receives prompt/message context, safe caller metadata, eligible targets, pricing metadata, tool support, modalities, and max-token requirements. If the model group enables `pii_filter`, the policy request is built from the redacted request object, including `request.raw`; placeholder mappings remain request-local and are not sent. The service must be treated as trusted infrastructure. It never receives raw router tokens, caller token hashes, provider API keys, or full router config.

Policy URLs should use HTTPS. Plain HTTP is accepted only for trusted loopback hosts such as `localhost`, `127.0.0.1`, and `::1`, or when `external_policy.allow_http: true` is explicitly configured for a trusted non-local endpoint. `allow_hosts` is exact-host matching, not a suffix or wildcard rule. Redirects are revalidated before each hop; a redirect to any host outside `allow_hosts`, including a loopback address that was not listed, fails before the redirected service is reached.

The router-level control is hostname and scheme based. Use deployment network policy, firewall rules, or service-mesh egress policy for private-network and CIDR restrictions until native CIDR egress controls are added.

## Local Demo

Run the committed prompt-size policy service:

```bash
python3 examples/external-routing-policy/prompt_size_policy.py
```

The sample `external-policy-demo` group in `config.example.yaml` points at `http://127.0.0.1:18090/route`. It routes prompts over 8,000 characters to targets tagged `tier: heavy` and shorter prompts to targets tagged `tier: cheap`.

## Validation Checklist

- Validate YAML with `rtk go test ./internal/router -run 'ExternalRoutingPolicy|Config'`.
- Confirm policy service auth is configured through `external_policy.headers`, not source code.
- Confirm `allow_hosts` contains exact hostnames only.
- Confirm non-local policy URLs use HTTPS unless `external_policy.allow_http: true` was explicitly approved.
- Confirm redirects to non-allowlisted hosts fail and do not reach the redirected service.
- Run a router smoke for a short prompt and a long prompt, then check selected upstream model.
- Run an image or tool request when the group supports VLM/tool traffic and confirm the policy payload contains only eligible targets.
- Confirm errors are clear: policy timeout, non-2xx, invalid JSON, and invalid target should return `502 routing-policy-error` unless `on_error: fallback` is explicitly configured.

## Production Rollout

- Add the policy-backed group catalog-only or with a private caller first.
- Keep policy egress on HTTPS. If plaintext HTTP is required for trusted internal infrastructure, document the reason for `external_policy.allow_http: true`.
- Keep `timeout_ms` small, typically 200-500 ms.
- Prefer `on_error: fail_closed` for policy-sensitive traffic.
- Use `on_error: fallback` only when the configured target order is explicitly approved as the default policy.
- After rollout, monitor `request_usage.error_class`, `request_trace_events`, selected provider/model, latency, and class labels.
- Document rollback as either disabling the policy group, switching the group back to `weighted`/`static`, or setting `on_error: fallback` if approved.
