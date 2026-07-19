# #523 — Double-entry ledger

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/523
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P7, P8
- **Depends on:** #507 #521
- **HTML mock:** [523-ledger.html](523-ledger.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Subscription credit | Worker | Webhook event | Balanced postings | journal+postings |
| Allowance grant | Worker | Period start | Grant liability | allowance accounts |
| Settle usage | Outbox | request_id | Debit allowance / x402 | idempotent |

## Alternate paths

- Duplicate settlement → one posting
- Unbalanced insert rejected

## Exact fields / payloads (fictional)

Accounts: sub_revenue, allowance_liability, x402_settlement, provider_cost_memo

## Validation & evidence

10k replay twice → one set; SQL debit=credit

## Expiry / teardown / rollback

Commercial retention independent of diagnostics purge
