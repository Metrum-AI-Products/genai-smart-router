# #533 — Docs, legal, support

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/533
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P1, P2, P6
- **Depends on:** Customer-visible blockers
- **HTML mock:** [533-docs-legal-support.html](533-docs-legal-support.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Pricing page | P1 | — | Plan disclosure | Approved copy |
| Support intake | P2 | Topic+ID | Ticket | Template |
| Error docs | P3 | Error type | Remediation | D11 map |

## Alternate paths

- Unapproved price claim blocked
- Stale prepaid copy fails QA

## Exact fields / payloads (fictional)

ToS, AUP, privacy, refund, SLA, status, pricing/allowance/x402

## Validation & evidence

docs-qa + usability to first request

## Expiry / teardown / rollback

Policy version captured at signup
