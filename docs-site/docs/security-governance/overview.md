---
title: Security And Governance
---

# Security And Governance

GenAI Smart Router centralizes caller access, provider credentials, admin access, telemetry boundaries, license enforcement, and optional content governance so applications and agent clients do not handle provider keys directly.

## Governance Layers

| Layer | Purpose |
|---|---|
| Caller tokens | Authenticate application and agent traffic, bind public caller metadata, and limit allowed model groups. |
| Model-group allow lists | Prevent callers from requesting groups outside their contract or project scope. |
| Quotas and budgets | Enforce RPM, TPM, concurrency, daily/monthly token limits, and spend controls before upstream calls. |
| Admin authentication | Protect browser/admin endpoints with Basic Auth or OIDC when enabled by the deployment. |
| Casbin authorization | Authorize admin/report/security/content actions by subject, object, and action. |
| Metrics isolation | Keep `/metrics` restricted to metrics-admin callers. |
| PII filtering | Redact configured text before target selection, cache-key generation, policy inputs, and upstream calls. |
| License enforcement | Gate licensed capabilities with safe status surfaces and caller-visible `license-*` errors. |

## Data Handling Boundaries

Public and hosted docs should use placeholder tokens, placeholder hosts, and sample model group names only. Operational diagnostics and reports must not expose raw provider keys, raw router tokens, token hashes, raw prompts, raw images, raw tool outputs, full upstream headers, full config files, private signing keys, or full license payloads.

## Related Pages

- [Admin Authentication](../configuration/admin-authentication)
- [Admin Authorization](../configuration/admin-authorization)
- [PII Filtering](../configuration/pii-filtering)
- [License-Protected Deployments](../licensing/)
- [Security And Trust](../evaluation/security-and-trust)
- [Deployment Security Assessment](../evaluation/deployment-security-assessment)
