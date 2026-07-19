# #532 — AWS Marketplace / AMI

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/532
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P1 procurement, P5
- **Depends on:** #521 #523 #527 #533
- **HTML mock:** [532-marketplace.html](532-marketplace.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Subscribe | Buyer | Offer | Register | Control plane link |
| AMI launch | P5 | Instance role | Bootstrap | BYOK+license |

## Alternate paths

- Fast-follow; does not replace Stripe+x402 hosted

## Exact fields / payloads (fictional)

product code, dimensions, private offer id

## Validation & evidence

Sandbox buyer flow; no hot-path metering

## Expiry / teardown / rollback

Unsubscribe stops entitlement per contract
