# #521 — Control plane: accounts & entitlement

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/521
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P2, P6
- **Depends on:** #507
- **HTML mock:** [521-control-plane.html](521-control-plane.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Signup | P2 | Email+OIDC | pending_email | users row |
| Verify | P2 | Link click | pending_funding | audit |
| Org+tenant | P2 | Name+region | tenant draft | orgs/tenants |
| Key create | P2 | Name | Raw once | hash+public ID |
| Suspend | P6 | Reason | suspended | revoke grants |

## Alternate paths

- Cross-tenant read → 404/403
- Control plane down → router keeps last grant until expiry

## Exact fields / payloads (fictional)

`POST /v1/api-keys` → `{public_id, token_once}`; states: pending_email→…→canceled

## Validation & evidence

Transition matrix unit tests; key absent from DB plaintext

## Expiry / teardown / rollback

Cancel revokes keys; retain audit/ledger
