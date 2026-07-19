# Shipped: API & CLI developer journey

**Personas:** P3

**HTML:** [api-and-cli.html](api-and-cli.html)

## Flow

1. Obtain key (hosted console once, or self-host token-gen).
2. `GET /v1/models` → allowed groups only.
3. Chat Completions, Responses, Anthropic Messages.
4. Codex CLI (`wire_api=responses`) / Claude Code (`ANTHROPIC_AUTH_TOKEN`, unset API key).

## Error remediation

| Error | Meaning | Next |
| --- | --- | --- |
| entitlement-inactive | No subscription | Subscribe / Portal |
| payment-required | Allowance exhausted | x402 pay + retry |
| payment-invalid | Bad/expired proof | Fresh challenge |
| quota / RPM | Traffic shape | Wait / raise limits |
| upstream-* | Provider path | request_id triage |
