# #541 — Auto-tune cheapest mix

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/541
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P10, P4
- **Depends on:** #392-394 #501 #528 #537 #521
- **HTML mock:** [541-auto-tune.html](541-auto-tune.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Job create | P10 | Dataset+min pass | Job queued | Central tuner |
| Search | Worker | Candidates | Best config | Evidence bundle |
| Activate | P4/P10 | Approve | Routing studio / canary | Rollback fingerprint |

## Alternate paths

- Fail quality → no activation
- Hot path never calls tuner

## Exact fields / payloads (fictional)

min_pass_rate, max_p95_ms, max_cost, candidate allowlist

## Validation & evidence

Synthetic dataset cheaper config proof

## Expiry / teardown / rollback

Job retention policy
