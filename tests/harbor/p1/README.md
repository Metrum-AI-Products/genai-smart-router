# Harbor P1/P2 offline stubs (HARBOR-07..18)

Offline task stubs and independent verifiers for [issue #94](https://github.com/metrum-ai/router/issues/94)
and P2 gap closure in [#104](https://github.com/metrum-ai/router/issues/104).
These do **not** run live Harbor or providers in CI. They prove verifier integrity:
reference solutions pass, starter/no-op and decoy artifacts fail.

| ID | Focus |
|---|---|
| HARBOR-07 | Receipt/chart extract; text-only decoy excluded |
| HARBOR-08 | Image + text tool result; both modalities required |
| HARBOR-09 | Opaque reasoning preservation (protocol check) |
| HARBOR-10 | Late instruction near context/tool limits |
| HARBOR-11 | Continuity across simulated restart |
| HARBOR-12 | Eligible backup after pre-commit primary failure |
| HARBOR-13 | Quota exhaustion; no forged success |
| HARBOR-14 | Cancel/timeout; incomplete ≠ completed |
| HARBOR-15 | Structured JSON schema + refusal/truncation |
| HARBOR-16 | Concurrent caller nonce isolation |
| HARBOR-17 | Multi-file Python + C repair; held-out tests + protocol obs |
| HARBOR-18 | Agent canary; explicit supported/unsupported (no silent drop) |

Run:

```bash
python3 tests/harbor/p1/run_offline_tests.py
```

Missing credentials for a real agent run must be recorded as `blocked` / skipped evidence, never as a green pass. See `scripts/evaluate_workload_gate.py --promotion` for promotion gating (nonempty `fixed_model_controls`, missing baseline blocks).
