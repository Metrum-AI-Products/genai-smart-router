---
title: Security And Governance
doc_type: explanation
---

# Security And Governance

GenAI Smart Router centralizes caller access, provider credentials, admin access, telemetry boundaries, license enforcement, and optional content governance so applications and agent clients do not handle provider keys directly.

## Governance Layers

| Layer | Purpose |
|---|---|
| Caller tokens | Authenticate application and agent traffic, bind public caller metadata, and limit allowed model groups. |
| Model-group allow lists | Prevent callers from requesting groups outside their contract or project scope. |
| Quotas and budgets | Enforce RPM, TPM, concurrency, traffic shaping, daily/monthly token limits, and spend controls before upstream calls. |
| Admin authentication | Protect browser/admin endpoints with Basic Auth or OIDC when enabled by the deployment. |
| Casbin authorization | Authorize admin/report/security/content actions by subject, object, and action. |
| Metrics isolation | Keep `/metrics` restricted to metrics-admin callers. |
| PII filtering | Redact configured text before target selection, cache-key generation, policy inputs, and upstream calls. |
| License enforcement | Gate licensed capabilities with safe status surfaces and caller-visible `license-*` errors. |

## Data Handling Boundaries

Examples use placeholder tokens, placeholder hosts, and sample model group names. Operational diagnostics and reports expose request IDs, caller labels, selected provider/model, status, timing, token, cost, and sanitized error fields so administrators can investigate without sharing credentials or customer content.

## Related Pages

- [Admin Authentication](../configuration/admin-authentication)
- [Admin Authorization](../configuration/admin-authorization)
- [PII Filtering](../configuration/pii-filtering)
- [License-Protected Deployments](../licensing/)
- [Security And Trust](../evaluation/security-and-trust)
- [Deployment Security Assessment](../evaluation/deployment-security-assessment)
