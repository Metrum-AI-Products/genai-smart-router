# #528 — Model canary ops

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/528
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P10
- **Depends on:** #505 #7 #507 #531
- **HTML mock:** [528-canary.html](528-canary.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Propose | P10 | Target+evidence | Draft canary | Config set |
| Shadow | System | Traffic sample | Metrics | No response change |
| Promote/rollback | P10 | Approve | Weights | Auto rollback gates |

## Alternate paths

- Quality breach → auto rollback
- Prosumer not enrolled → no experimental

## Exact fields / payloads (fictional)

exposure %, shape gates, SLO thresholds

## Validation & evidence

Fault inject each rollback trigger

## Expiry / teardown / rollback

Canary expiry timestamp required
