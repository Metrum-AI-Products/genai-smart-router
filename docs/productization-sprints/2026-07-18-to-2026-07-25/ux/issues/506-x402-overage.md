# #506 — x402 overage payment

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/506
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P3, P7, P9
- **Depends on:** #522 #540
- **HTML mock:** [506-x402-overage.html](506-x402-overage.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Challenge | Router | No proof | 402 + PAYMENT-REQUIRED | quote frozen |
| Pay | Client | Sign payment | Retry headers | Facilitator |
| Settle | Router | Valid proof | 200 + settlement hdr | One upstream + ledger |

## Alternate paths

- Replay → payment-invalid
- Facilitator down → payment-unavailable
- Upstream fail after settle → support ref

## Exact fields / payloads (fictional)

Quote id `q_…`, amount `0.020000`, asset/network fictional, expiry 120s

## Validation & evidence

Fake facilitator + test-network smoke; Chat/Responses/Messages

## Expiry / teardown / rollback

Quote TTL; no auto refund
