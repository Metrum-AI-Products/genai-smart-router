# External Routing Policy Service Demo

This demo service implements prompt-size routing for GenAI Smart Router's `strategy: external`.

Run it locally:

```bash
python3 examples/external-routing-policy/prompt_size_policy.py
```

Configure a model group with:

```yaml
strategy: external
external_policy:
  url: http://127.0.0.1:18090/route
  allow_hosts: [127.0.0.1]
  timeout_ms: 500
  max_response_bytes: 65536
  on_error: fail_closed
```

The service receives safe routing context, eligible targets, caller metadata, pricing metadata, tool support, and input modality details. It does not receive raw router tokens, caller token hashes, or provider API keys.

This demo uses loopback HTTP, which the router treats as a trusted-local development exception. Non-local external policy services should use HTTPS, or set `external_policy.allow_http: true` only after deployment security review. Redirects are rechecked against the exact `allow_hosts` list before the router follows them.
