# Inspect coding evaluations

Issue #562 adds an explicitly invoked coding-evaluation lane. It is not part of `make`, `make test`, or any normal build because it needs a live model endpoint, an authenticated caller, Docker, and a bounded spend approval.

Run a caller-visible router group with protected environment variables (never put the token in a command line or checked-in file):

```sh
export EVAL_BASE_URL=https://router.example/v1 EVAL_API_KEY='…'
make eval-humaneval eval-report EVAL_SUITE=humaneval EVAL_MODEL=deployment-defined-group EVAL_MODEL_KIND=router-group EVAL_LIMIT=8
make eval-bigcodebench eval-report EVAL_SUITE=bigcodebench EVAL_MODEL=deployment-defined-group EVAL_MODEL_KIND=router-group EVAL_LIMIT=8
```

`EVAL_MODEL_KIND` is required policy context: use `router-group` for a caller-visible deployment group and `direct-baseline` only for an exact model listed in `approved_direct_baselines` in the evaluation policy. It is never inferred from a slash because group names are deployment-defined strings. `EVAL_API` selects the Inspect model provider (default `openai`) and prefixes `EVAL_MODEL`; `EVAL_REASONING` is passed as Inspect's `reasoning_effort` model argument. `EVAL_CONCURRENCY`, `EVAL_TIMEOUT`, `EVAL_INSPECT`, and `EVAL_POLICY` are explicit overrides. Limits are 1–200 and concurrency 1–16. Use the caller output-cap, dialect, tool mode/count, streaming state, and request-size bucket intended for promotion as separate evidence; a text-only OpenAI-style suite does not validate Responses, Messages, or bridge traffic.

The wrapper executes Inspect from an empty disposable directory. This prevents Inspect from auto-selecting this repository's Dockerfile as its code-execution sandbox. Each Make invocation creates an isolated timestamp/PID `EVAL_LOG_DIR`, shared by its explicitly requested suite and report targets; CI pins the run ID across separate Make calls. The exporter uses Inspect's supported log API and reads only headers and sample summaries into `inspect-aggregate.json`; only `evaluation-summary.json` and `.md` are safe to upload. They omit prompts, responses, schemas, raw logs, credentials, headers, and token hashes.

Inspect cannot report router reasoning usage itself. To populate the reasoning-coverage section, an operator or protected CI job must separately export aggregate-only data from the router usage database for the exact evaluation window and caller/model-group filters, then set its owner-readable path as `EVAL_REASONING_COVERAGE_FILE` when running `make eval-report`. The supported exporter is `router-usage-report --reasoning-coverage-out`; it writes only totals plus provider/model/dialect attempt aggregates—never request IDs, caller IDs, prompt/response content, headers, endpoint hosts, or credentials.

```sh
# Run in the protected environment that can read the usage DB. Keep the output
# outside the repository and make it owner-readable only.
go run ./cmd/router-usage-report \
  --driver postgres --dsn "$ROUTER_USAGE_DSN" \
  --from 2026-07-23T10:00:00Z --to 2026-07-23T11:00:00Z \
  --resolved-group deployment-defined-group \
  --reasoning-coverage-out /protected/eval/reasoning-coverage.json

EVAL_REASONING_COVERAGE_FILE=/protected/eval/reasoning-coverage.json \
  make eval-report EVAL_SUITE=humaneval
```

If that explicit file is unavailable or malformed, reporting blocks rather than silently presenting unverified coverage. With no file configured, the report deliberately says `not reported`; it does not infer reasoning values from Inspect logs. Do not copy the protected exporter file into the repository or CI report directory—the report sanitizer copies only its allowlisted scalar fields into `evaluation-summary.*`.

`make eval-ci-smoke` is a two-task router-group smoke for explicitly configured CI. `make eval-ci-full` is the reusable protected full-lane command: it requires protected router inputs, one sanitized baseline aggregate per suite, and one sanitized request-time router-usage aggregate per suite. Router-group cost growth is calculated only from the latter's `stored_request_time_cost_usd` scalar, exported from persisted router usage reporting—not from evaluator estimates. It runs HumanEval and BigCodeBench in separate directories and creates timestamped sanitized reports. Empty or incomplete protected baselines fail closed, as do partial, skipped, blocked, or otherwise incomplete candidate aggregates. The workflow pins the reviewed Inspect harness versions and is only an environment adapter and trigger for this command. Promotion review remains human-controlled. Roll back by removing the candidate from the group or reducing its weight; never promote on this benchmark alone.

When the scheduled or protected manual CI workflow invokes `make eval-report`, it sets `EVAL_SAVE_CI_REPORT=true`. The target copies only the sanitized JSON aggregate and detailed Markdown summary to the versioned, committed report directory `docs/evaluation-reports/inspect/<timestamp>/`. The workflow commits that folder after a successful run. Timestamps are supplied by CI and preserve an immutable run history; do not put raw Inspect logs, prompts, outputs, headers, credentials, or token material there.
