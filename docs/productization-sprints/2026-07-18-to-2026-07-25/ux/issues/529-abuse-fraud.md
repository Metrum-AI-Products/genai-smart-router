# #529 — Abuse & fraud controls

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/529
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P6, P9, P2
- **Depends on:** #521 #522 #524 #506
- **HTML mock:** [529-abuse-fraud.html](529-abuse-fraud.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Signup risk | System | Signals | Allow/deny trial | Risk policy |
| Kill switch | P6 | Tenant ID | Suspended | Revoke grants |
| Customer msg | P2 | — | Limit guidance | No rule leakage |

## Alternate paths

- Risk service outage → fail-safe paid-only
- False positive unsuspend

## Exact fields / payloads (fictional)

Safe reason codes only; no prompts

## Validation & evidence

Abuse fixtures; load test neighbor isolation

## Expiry / teardown / rollback

Appeal window documented
