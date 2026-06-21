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
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

The external routing policy service receives prompt/message context, safe caller metadata, eligible targets, pricing metadata, tool support, modalities, and max-token requirements. It must be treated as trusted infrastructure. It never receives raw router tokens, caller token hashes, provider API keys, or full router config.

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
- Run a router smoke for a short prompt and a long prompt, then check selected upstream model.
- Run an image or tool request when the group supports VLM/tool traffic and confirm the policy payload contains only eligible targets.
- Confirm errors are clear: policy timeout, non-2xx, invalid JSON, and invalid target should return `502 routing-policy-error` unless `on_error: fallback` is explicitly configured.

## Production Rollout

- Add the policy-backed group catalog-only or with a private caller first.
- Keep `timeout_ms` small, typically 200-500 ms.
- Prefer `on_error: fail_closed` for policy-sensitive traffic.
- Use `on_error: fallback` only when the configured target order is explicitly approved as the default policy.
- After rollout, monitor `request_usage.error_class`, `request_trace_events`, selected provider/model, latency, and class labels.
- Document rollback as either disabling the policy group, switching the group back to `weighted`/`static`, or setting `on_error: fallback` if approved.
