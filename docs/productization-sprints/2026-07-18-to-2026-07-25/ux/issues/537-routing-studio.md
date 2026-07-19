# #537 — Routing studio

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/537
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P4
- **Depends on:** #7 #521 #536
- **HTML mock:** [537-routing-studio.html](537-routing-studio.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Draft group | P4 | Targets+weights | Validation | Server checks |
| Approve | Owner | Reauth | Activate | Version+rollback |

## Alternate paths

- Invalid weights rejected
- v1 static/weighted only

## Exact fields / payloads (fictional)

group name, targets, weights, shape envelope

## Validation & evidence

Simulation uses synthetic templates only

## Expiry / teardown / rollback

Rollback to prior version fingerprint
