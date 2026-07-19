# #536 — Upstream connection onboarding

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/536
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P4
- **Depends on:** #535
- **HTML mock:** [536-upstream-onboarding.html](536-upstream-onboarding.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Draft connection | P4 | Adapter+URL+cred | Draft | Egress check |
| Probes | Worker | Synthetic | Evidence rows | Per skin |
| Select models | P4 | Passing only | Draft routing | #537 |

## Alternate paths

- Probe fail → bounded reason
- Never browser hot-path probes

## Exact fields / payloads (fictional)

adapter, base_url, credential public_id, skins claimed

## Validation & evidence

Scalar evidence only; no raw provider bodies

## Expiry / teardown / rollback

Revalidate on DNS/endpoint change
