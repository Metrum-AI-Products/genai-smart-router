# #522 — Admission: entitlement + allowance + x402 handoff

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/522
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P3, P8
- **Depends on:** #521 #523 #540
- **HTML mock:** [522-admission.html](522-admission.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Within allowance | Router | Request | 200 + settle | Local grant reserve |
| Exhausted | Router | Request | 402 payment-required | No upstream |
| After x402 | Router | Retry+proof | 200 | #506 settle then upstream |

## Alternate paths

- Grant expired → deny
- Race N pods ≤ grant
- No Stripe in handler trace

## Exact fields / payloads (fictional)

Grant: in_remaining, out_remaining, price_book_version, expires_at

## Validation & evidence

Concurrency + fault injection tests in issue body

## Expiry / teardown / rollback

Fail closed at grant expiry/outbox cap
