# #527 — Release engineering gates

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/527
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P8
- **Depends on:** #507 #516-519
- **HTML mock:** [527-release-engineering.html](527-release-engineering.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Build | CI | Signed image | SBOM | Attestation |
| Stage | CI | Smokes | Promote/block | Evidence |
| Rollback | P8 | Prior digest | Health restore | GitOps |

## Alternate paths

- Failed smoke blocks prod
- Unsigned image rejected

## Exact fields / payloads (fictional)

digest, chart version, migration window

## Validation & evidence

Stage-to-prod rehearsal

## Expiry / teardown / rollback

N/A
