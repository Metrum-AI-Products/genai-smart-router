# #524 — Stripe subscription checkout

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/524
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P2, P7
- **Depends on:** #521 #523 #540
- **HTML mock:** [524-stripe-subscription.html](524-stripe-subscription.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Checkout | P2 | Confirm plan | Stripe hosted | Session + idempotency |
| Webhook | Stripe | invoice.paid | Entitlement active | Fulfillment worker |
| Portal | P2 | Manage PM | Portal session | No card in Metrum DOM |

## Alternate paths

- Redirect without webhook → no entitlement
- Duplicate event → one fulfillment

## Exact fields / payloads (fictional)

mode=subscription; success_url confirms; Price from catalog only

## Validation & evidence

Stripe CLI E2E; SAQ-A boundary scan

## Expiry / teardown / rollback

Cancel/refund via Portal → async entitlement revoke
