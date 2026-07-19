# #526 — EKS provisioning & teardown

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/526
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P2, P8
- **Depends on:** #521 #507 #516-519
- **HTML mock:** [526-provisioning.html](526-provisioning.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Saga start | Worker | Funded tenant | op running | Idempotent steps |
| Activate | Worker | Config+grant | Smokes | Shared fleet |
| Ready | Console | — | Key reveal | Mark active |
| Cancel | P2/P6 | Confirm | Revoke then teardown | Retention hold |

## Alternate paths

- Fail mid-saga → compensate exact step
- Dedicated path adds DNS/TLS

## Exact fields / payloads (fictional)

op_id, desired_version, compensating_action

## Validation & evidence

Kind/EKS double-create; isolation suite

## Expiry / teardown / rollback

Cancel: revoke grants → drain → evidence → delete ephemeral
