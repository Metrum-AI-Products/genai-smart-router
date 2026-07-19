# #531 — Observability & on-call

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/531
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P8
- **Depends on:** Wave 2 + #527
- **HTML mock:** [531-observability-oncall.html](531-observability-oncall.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Dashboard | P8 | — | SLO panels | Metrics |
| Alert | System | Lag threshold | Page | Runbook |
| Game day | P8 | Inject fault | Recover | Timeline |

## Alternate paths

- Control-plane outage → grant-bounded traffic
- x402 facilitator health red

## Exact fields / payloads (fictional)

SLOs: signup, provision, webhook, grant, x402, reconcile

## Validation & evidence

Synthetic probes + game-day evidence

## Expiry / teardown / rollback

N/A
