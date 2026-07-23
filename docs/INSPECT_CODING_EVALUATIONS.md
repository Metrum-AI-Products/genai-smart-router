# Inspect coding evaluations

Issue #562 adds an explicitly invoked coding-evaluation lane. It is not part of `make`, `make test`, or any normal build because it needs a live model endpoint, an authenticated caller, Docker, and a bounded spend approval.

Run a caller-visible router group with protected environment variables (never put the token in a command line or checked-in file):

```sh
export EVAL_BASE_URL=https://router.example/v1 EVAL_API_KEY='…'
make eval-humaneval EVAL_MODEL=deployment-defined-group EVAL_LIMIT=8
make eval-bigcodebench EVAL_MODEL=deployment-defined-group EVAL_LIMIT=8
make eval-report EVAL_LOG_DIR=tmp/inspect-evals
```

`EVAL_API` selects the Inspect model provider (default `openai`) and prefixes an unqualified `EVAL_MODEL`; `EVAL_REASONING` is passed as Inspect's `reasoning_effort` model argument. `EVAL_CONCURRENCY`, `EVAL_TIMEOUT`, `EVAL_INSPECT`, and `EVAL_POLICY` are explicit overrides. Limits are 1–200 and concurrency 1–16. Use the caller output-cap, dialect, tool mode/count, streaming state, and request-size bucket intended for promotion as separate evidence; a text-only OpenAI-style suite does not validate Responses, Messages, or bridge traffic.

The wrapper executes Inspect from an empty disposable directory. This prevents Inspect from auto-selecting this repository's Dockerfile as its code-execution sandbox. Raw Inspect logs remain in the ignored `EVAL_LOG_DIR`; only `evaluation-summary.json` and `.md` are safe to upload. They omit prompts, responses, schemas, raw logs, credentials, headers, and token hashes.

`make eval-ci-smoke` is a two-task router-group smoke for explicitly configured CI. Missing Inspect/Docker yields `skipped`; missing protected credentials or model access yields `blocked`; neither is a passing quality result. The protected scheduled/manual workflow runs bounded HumanEval and BigCodeBench, compares aggregates using `config/evaluation-policy.example.json`, and requires human promotion review. Roll back by removing the candidate from the group or reducing its weight; never promote on this benchmark alone.
