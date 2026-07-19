# #540 — Plan catalog

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/540
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P1, P2, P7
- **Depends on:** #521
- **HTML mock:** [540-plan-catalog.html](540-plan-catalog.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Finance edit | P7 | SKU+units+bands | plan_versions row | activation audit |
| Customer picker | P2 | Select SKU | Checkout Price ID | server-side map |

## Alternate paths

- Invalid overlapping bands rejected
- v2 activation does not rewrite in-flight v1 grants

## Exact fields / payloads (fictional)

plan_starter_mix_v3: $49/mo, 50M/10M tokens, groups default/fast/high, x402 bands…

## Validation & evidence

Catalog validation unit tests; Finance sign-off before prod numbers

## Expiry / teardown / rollback

Version stamp on grants/quotes; historical never reprices
