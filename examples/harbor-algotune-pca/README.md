# Harbor Agentic Coding Case Study

This example compares Metrum AI Router model groups on Harbor's `aider/polyglot_python_two-bucket` agentic coding task. The canonical task command is:

```bash
harbor run -t aider/polyglot_python_two-bucket
```

The workflow runs both Codex CLI and Claude Code through the router across deployment-defined model groups. The checked-in defaults `default`, `fast`, `small`, `medium`, `high`, and `big-coder` are examples from the historical case-study deployment, not product-required group names.

Current production Harbor runs use one reusable Harbor caller token with access to all deployed model groups. The usage DB and reports still separate provider calls, input tokens, output tokens, latency, throughput, cache behavior, and fallback behavior by client, requested model group, timestamps, and run results. The older per-`{agent, model_group}` token generator is retained for isolated local or one-off investigations where separate caller identities are required.

## Prerequisites

Install the **pinned** Harbor baseline (AGENT-01). Do not use unbounded
`latest` inside a required reproducibility run. Canary versions are reported
separately; see `tests/harbor/pins.py`.

```bash
uv tool install 'harbor==0.13.2'
```

Pinned agent CLI baselines for required runs:

| Client | Pin |
|---|---|
| Harbor | `0.13.2` |
| Codex CLI (`@openai/codex`) | `0.153.4` |
| Claude Code | `2.1.220` |

Install and verify the local agent CLIs:

```bash
codex --version
claude --version
```

Offline adapter contract tests (no live Harbor / providers):

```bash
make harbor-adapter-test
```

Build the local router tools if they are not already present:

```bash
make build
```

## Configure Router Token

For production Harbor runs, use the reusable Harbor caller token from the production host token file and pass it as `HARBOR_ROUTER_TOKEN`. Do not print the token in logs or commit it.

```bash
cd examples/harbor-algotune-pca
export HARBOR_ROUTER_TOKEN="<raw reusable Harbor router token>"
export ROUTER_BASE_URL="https://<router-host>"
export CASE_ID="case-$(date -u +%Y%m%dT%H%M%SZ)"
./run_case_study.sh
```

Collect production usage for the reusable Harbor caller by filtering on the configured caller project/environment and the run time window:

```bash
cd /opt/smart-llmrouter/compose
dsn="$(sed -n 's/^ROUTER_USAGE_DB_DSN=//p' .env | tail -n 1)"

docker compose exec -T router /app/bin/metrum-ai-router-usage-report \
  --driver postgres \
  --dsn "$dsn" \
  --caller-project harbor \
  --caller-environment prod \
  --from "<case-start-utc>" \
  --to "<case-end-utc>" \
  --out /app/logs/harbor-agentic-usage.md
```

## Legacy Test Tokens

For isolated local or one-off investigations, generate one token per `{agent, model_group}`:

```bash
cd examples/harbor-algotune-pca
CASE_ID="case-$(date -u +%Y%m%dT%H%M%SZ)" ./generate_tokens.sh
```

Generated files are written under `generated/$CASE_ID/`:

```text
tokens.env     raw bearer tokens for the runner; do not commit or share
callers.yaml   caller config blocks to register with the router
manifest.tsv   agent/model_group/token_id mapping for audit
```

Register the caller entries from `callers.yaml` in the router config before running the case study. Each generated token allows exactly one model group. The router stores only token hashes and logs only the public `token_id`.

Avoid this path for routine production Harbor runs. If temporary production callers are required for an isolated investigation, follow the normal production config process:

1. Back up `/opt/smart-llmrouter/compose/config/config.yaml`.
2. Append the generated caller blocks with structured YAML tooling.
3. Run `sudo docker compose config >/dev/null`.
4. Restart the router container.
5. Verify `https://<router-host>/readyz`.

After the report is collected, remove the temporary caller blocks from production config and restart the router again. Historical usage remains queryable by `token_id`.

## Run The Matrix

Hosted production example with the reusable Harbor token:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<case-id>"
export ROUTER_BASE_URL="https://<router-host>"
export HARBOR_ROUTER_TOKEN="<raw reusable Harbor router token>"
./run_case_study.sh
```

Local router example:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<case-id>"
export ROUTER_BASE_URL="http://127.0.0.1:18080"
export HARBOR_ROUTER_TOKEN="<raw reusable or local router token>"
./run_case_study.sh
```

Useful overrides:

```bash
export AGENTS="codex,claude-code"
export MODEL_GROUPS="small,big-coder"
export DRY_RUN=1
```

## Evaluate The Outcome Gate

`workload_gate_matrix.json` is a safe **smoke/example** matrix. It records the task, verifier, reward rule, client matrix, deployment-defined model groups, outcome dimensions, and thresholds for pass rate, reward, p95 latency, cost per successful task, error rate, and fallback rate. Its `fixed_model_controls` list is intentionally empty so documentation examples cannot produce a promotion pass. Copy the matrix, fill nonempty fixed-model controls (direct provider and/or fixed router model IDs), and raise `min_attempts_per_cell` before using `--promotion`.

After `run_case_study.sh` writes `runs/$CASE_ID/results.tsv`, evaluate the outcome gate locally:

```bash
cd examples/harbor-algotune-pca
python3 ../../scripts/evaluate_workload_gate.py \
  --matrix workload_gate_matrix.json \
  --results "runs/$CASE_ID/results.tsv" \
  --out-json "reports/$CASE_ID/workload-gate.json" \
  --out-md "reports/$CASE_ID/workload-gate.md"
```

Promotion mode (requires nonempty `fixed_model_controls` and present baseline cells; missing baseline, missing task, or insufficient sample size exits `2` as **blocked**, not pass):

```bash
python3 ../../scripts/evaluate_workload_gate.py \
  --promotion \
  --matrix workload_gate_matrix.json \
  --results "runs/$CASE_ID/results.tsv" \
  --out-json "reports/$CASE_ID/workload-gate.json" \
  --out-md "reports/$CASE_ID/workload-gate.md"
```

If a safe usage-report JSON export is available, include it so the gate report shows selected upstream distribution, request IDs, stored request-time cost, latency, status, and fallback correlation:

```bash
python3 ../../scripts/evaluate_workload_gate.py \
  --matrix workload_gate_matrix.json \
  --results "runs/$CASE_ID/results.tsv" \
  --usage-json "reports/$CASE_ID/usage-rows.json" \
  --out-json "reports/$CASE_ID/workload-gate.json" \
  --out-md "reports/$CASE_ID/workload-gate.md"
```

CI and local development can validate the gate logic without Harbor or live provider access:

```bash
python3 scripts/evaluate_workload_gate_test.py
python3 tests/harbor/p1/run_offline_tests.py
```

The gate exits `1` when thresholds fail and `2` when promotion requirements are incomplete (`blocked`), unless `--no-fail` is supplied for exploratory reporting. Its outputs omit raw router tokens, token hashes, provider keys, authorization headers, raw prompts, images, and tool outputs.

For a different Harbor task, override `HARBOR_TASK`, `HARBOR_ARTIFACTS`, and `EXTRA_INSTRUCTION_PATHS` together so the captured artifact and task-specific self-check instructions match the task being evaluated.

The tested default is:

```bash
export HARBOR_TASK="aider/polyglot_python_two-bucket"
export HARBOR_ARTIFACTS="/app/two_bucket.py"
export EXTRA_INSTRUCTION_PATHS="examples/harbor-algotune-pca/two-bucket-verification.md"
```

The runner creates `runs/$CASE_ID/results.tsv` and one log file per run.

### Codex Behavior

For each Codex run, the script creates an isolated `CODEX_HOME` containing:

```toml
model = "<model-group>"
model_provider = "metrum-ai-router"

[model_providers."metrum-ai-router"]
name = "Metrum AI Router"
base_url = "<router-base-url>/v1"
env_key = "METRUM_ROUTER_KEY"
wire_api = "responses"
```

The token is supplied through `METRUM_ROUTER_KEY`.

### Claude Code Behavior

For each Claude Code run, the script sets:

```bash
ANTHROPIC_BASE_URL="<router-base-url>"
ANTHROPIC_AUTH_TOKEN="<generated-router-token>"
```

The script explicitly unsets `ANTHROPIC_API_KEY` for Claude Code router traffic.

## Collect A Report

For a local SQLite usage DB:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<generated-case-id>"
export ROUTER_USAGE_DB_DRIVER=sqlite
export ROUTER_USAGE_DB_PATH="../../usage.sqlite"
./collect_report.sh
```

For production Postgres, run from the deployment host or from an environment that can reach the DB:

```bash
cd /opt/smart-llmrouter/compose
dsn="$(sed -n 's/^ROUTER_USAGE_DB_DSN=//p' .env | tail -n 1)"

docker compose exec -T router /app/bin/metrum-ai-router-usage-report \
  --driver postgres \
  --dsn "$dsn" \
  --caller-project harbor-algotune-pca \
  --caller-environment "<generated-case-id-lowercase>" \
  --out /app/logs/harbor-agentic-usage.md
```

From this repository, the helper has the same filters:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<case-id>"
export ROUTER_USAGE_DB_DRIVER=postgres
export ROUTER_USAGE_DB_DSN="$ROUTER_USAGE_DB_DSN"
export PROJECT="harbor"
export CALLER_ENVIRONMENT="prod"
export FROM="<case-start-utc>"
export TO="<case-end-utc>"
./collect_report.sh
```

The generated `reports/$CASE_ID/case-study.md` includes:

- Harbor run status by agent and model group.
- Usage by internal router token id, user, project, and environment.
- Usage by external provider and provider model.
- Usage by router model group and client.
- Input, output, and total token counts from provider responses.
- Upstream and downstream tokens/sec per request.
- Cache hits, misses, bypasses, and occupancy snapshots.
- Latency, attempts, fallbacks, status codes, hourly usage, and caller IP usage.

## Latest Recorded Case Study

The canonical dated evidence is `docs/harbor-case-study.md` (and the aligned public page `docs-site/docs/evaluation/harbor-case-study.mdx`), captured **2026-06-29** on router build `762592b` for model group `big-coder`:

- Task: `aider/polyglot_python_two-bucket`
- Agents: Codex CLI (`0.142.0`) and Claude Code CLI (`2.1.186`); Harbor CLI `0.13.2`
- Codex: status ok, reward `1`, 0 Harbor errors
- Claude Code: status failed, reward `0`, 0 Harbor errors — starter `pass` artifact left unchanged with a repeated-read / no-edit trajectory
- Do **not** present older June 15 full-matrix (12/12) summaries as current production proof; that run is historical only and is superseded by the dated June 29 evidence above

## Notes

- The two-bucket task asks the agent to implement `/app/two_bucket.py` with a `measure` function for the bucket-measuring puzzle.
- The scripts do not print raw provider keys.
- Generated raw router tokens are ignored by git.
- Routine production Harbor runs should reuse the common Harbor caller instead of adding temporary production callers.
- Full matrix runs can be expensive. Use `MODEL_GROUPS=small` or `DRY_RUN=1` for a smoke test first.
