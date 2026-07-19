# #525 — Prosumer console

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/525
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P2, P3
- **Depends on:** #521 #522 #523 #524 #506 #540
- **HTML mock:** [525-prosumer-console.html](525-prosumer-console.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Home | P2/P3 | — | Allowance+x402 tiles | Scoped APIs |
| Keys | P2 | Create/rotate | Token once | Control plane |
| Billing | P2 | Portal link | Stripe Portal | #524 |
| Support | P3 | Correlation ID | Case created | Safe IDs only |

## Alternate paths

- Exhausted → payment-required panel
- Suspended → remediation

## Exact fields / payloads (fictional)

Nav: Home, Keys, Usage, Allowance, x402, Billing, Support

## Validation & evidence

Playwright states; no secret in DOM

## Expiry / teardown / rollback

Cancelled org read-only history
