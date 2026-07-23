# Inspect coding evaluations

Issue #562 adds an explicitly invoked coding-evaluation lane. It is not part of `make`, `make test`, or any normal build because it needs a live model endpoint, an authenticated caller, Docker, and a bounded spend approval.

Run a caller-visible router group with protected environment variables (never put the token in a command line or checked-in file):

```sh
export EVAL_BASE_URL=https://router.example/v1 EVAL_API_KEY='…'
make eval-humaneval eval-report EVAL_SUITE=humaneval EVAL_MODEL=deployment-defined-group EVAL_MODEL_KIND=router-group EVAL_LIMIT=8
make eval-bigcodebench eval-report EVAL_SUITE=bigcodebench EVAL_MODEL=deployment-defined-group EVAL_MODEL_KIND=router-group EVAL_LIMIT=8
```

`EVAL_MODEL_KIND` is required policy context: use `router-group` for a caller-visible deployment group and `direct-baseline` only for an approved direct upstream reference. It is never inferred from a slash because group names are deployment-defined strings. `EVAL_API` selects the Inspect model provider (default `openai`) and prefixes `EVAL_MODEL`; `EVAL_REASONING` is passed as Inspect's `reasoning_effort` model argument. `EVAL_CONCURRENCY`, `EVAL_TIMEOUT`, `EVAL_INSPECT`, and `EVAL_POLICY` are explicit overrides. Limits are 1–200 and concurrency 1–16. Use the caller output-cap, dialect, tool mode/count, streaming state, and request-size bucket intended for promotion as separate evidence; a text-only OpenAI-style suite does not validate Responses, Messages, or bridge traffic.

The wrapper executes Inspect from an empty disposable directory. This prevents Inspect from auto-selecting this repository's Dockerfile as its code-execution sandbox. Each Make invocation creates an isolated timestamp/PID `EVAL_LOG_DIR`, shared by its explicitly requested suite and report targets; CI pins the run ID across separate Make calls. The exporter uses Inspect's supported log API and reads only headers and sample summaries into `inspect-aggregate.json`; only `evaluation-summary.json` and `.md` are safe to upload. They omit prompts, responses, schemas, raw logs, credentials, headers, and token hashes.

`make eval-ci-smoke` is a two-task router-group smoke for explicitly configured CI. Missing Inspect/Docker yields `skipped`; missing protected credentials or model access yields `blocked`; neither is a passing quality result. The protected scheduled/manual workflow installs both Inspect and its packaged task suite, runs bounded HumanEval and BigCodeBench in separate directories, and requires a protected sanitized baseline aggregate for each suite before it enforces `config/evaluation-policy.example.json`. Reports fail closed when an aggregate is partial, skipped, blocked, or otherwise incomplete. Promotion review remains human-controlled. Roll back by removing the candidate from the group or reducing its weight; never promote on this benchmark alone.

When the scheduled or protected manual CI workflow invokes `make eval-report`, it sets `EVAL_SAVE_CI_REPORT=true`. The target copies only the sanitized JSON aggregate and detailed Markdown summary to the versioned, committed report directory `docs/evaluation-reports/inspect/<timestamp>/`. The workflow commits that folder after a successful run. Timestamps are supplied by CI and preserve an immutable run history; do not put raw Inspect logs, prompts, outputs, headers, credentials, or token material there.
