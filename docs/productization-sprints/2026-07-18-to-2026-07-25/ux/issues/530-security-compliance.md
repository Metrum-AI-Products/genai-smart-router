# #530 — Security & tenant isolation

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/530
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P9
- **Depends on:** #521 #524 #526 #527 #506
- **HTML mock:** [530-security-compliance.html](530-security-compliance.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Threat model | P9 | Surfaces list | Controls map | ADR |
| Pentest | External | Staging | Findings | Remediation |
| Sign-off | P9 | Exception register | Launch OK | Evidence |

## Alternate paths

- Open critical finding → no launch
- Metrics admin isolation regression

## Exact fields / payloads (fictional)

Checklist: identity, webhook, x402 facilitator, secrets, EKS network

## Validation & evidence

Authz matrix + scans + tabletop

## Expiry / teardown / rollback

Exception expiry dates required
