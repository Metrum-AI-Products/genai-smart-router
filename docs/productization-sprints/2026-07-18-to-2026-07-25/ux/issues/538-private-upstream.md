# #538 — Private upstream connectivity

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/538
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P4, P9
- **Depends on:** #536
- **HTML mock:** [538-private-upstream.html](538-private-upstream.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Request private | P4 | Host+profile | network_review | Ops review |
| Activate | P8 | Approve egress | active | NetworkPolicy |

## Alternate paths

- SSRF ranges rejected
- Public shared tenancy: approved public only at launch

## Exact fields / payloads (fictional)

canonical URL, egress profile, region class

## Validation & evidence

Connectivity tests; no arbitrary proxy

## Expiry / teardown / rollback

DNS change → revalidation
