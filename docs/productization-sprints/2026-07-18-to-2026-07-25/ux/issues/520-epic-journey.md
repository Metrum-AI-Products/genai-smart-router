# #520 — Epic journey board

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/520
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** P1–P11 (index)
- **Depends on:** All launch-blocking children
- **HTML mock:** [520-epic-journey.html](520-epic-journey.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
| Epic board | Orchestrator | Open child links | Status per wave | Milestone checklist |
| Go/No-Go | Product+Finance+Sec | Sign evidence | Launch decision | Evidence index |

## Alternate paths

- Blocked NEEDS-HUMAN decision → stop wave
- Child red CI → no #534

## Exact fields / payloads (fictional)

Wave table; PR links; evidence paths under `ux/`.

## Validation & evidence

Every launch-blocking issue has MD+HTML; E2E #534 green.

## Expiry / teardown / rollback

N/A — epic tracking only.
