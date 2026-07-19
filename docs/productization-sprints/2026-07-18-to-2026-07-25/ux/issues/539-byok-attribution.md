# #539 — BYOK cost attribution

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/539
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P4, P7
- **Depends on:** #521 #522 #523 #535 #536
- **HTML mock:** [539-byok-attribution.html](539-byok-attribution.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| View costs | P4 | Filters | Attribution table | Scalar usage |
| Reconcile | P7 | Run | Report | No provider invoices stored |

## Alternate paths

- BYOK must not draw pooled grant
- Unknown upstream cost labeled unknown

## Exact fields / payloads (fictional)

funding_source pooled|byok, estimate vs upstream billed

## Validation & evidence

Finance attribution review

## Expiry / teardown / rollback

N/A
