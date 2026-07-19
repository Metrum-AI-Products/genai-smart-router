# #535 — BYOK vault

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/535
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P4
- **Depends on:** #521
- **HTML mock:** [535-byok-vault.html](535-byok-vault.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Add key | P4 | Secret once | public_id | Secrets Manager write |
| Rotate | P4 | New secret | Overlap then revoke | Audit |

## Alternate paths

- Control plane cannot read secret back
- Validation_failed state

## Exact fields / payloads (fictional)

provider kind, name, secret (write-only)

## Validation & evidence

Secret absent from Postgres; RBAC+reauth

## Expiry / teardown / rollback

Revoke stops upstream in bounded window
