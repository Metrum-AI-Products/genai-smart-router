# Harbor Agentic Coding Case Study

This example compares Smart LLM Router model groups on Harbor's `aider/polyglot_python_two-bucket` agentic coding task. The canonical task command is:

```bash
harbor run -t aider/polyglot_python_two-bucket
```

The workflow runs both Codex CLI and Claude Code through the router across the model groups `default`, `fast`, `small`, `medium`, `high`, and `big-coder`. Each `{agent, model_group}` run uses a fresh router caller token so the usage DB can attribute provider calls, input tokens, output tokens, latency, throughput, cache behavior, and fallback behavior to a single test cell.

## Prerequisites

Install Harbor with `uv`:

```bash
uv tool install harbor
```

Install and verify the local agent CLIs:

```bash
codex --version
claude --version
```

Build the local router tools if they are not already present:

```bash
make build
```

## Generate Test Tokens

Generate one token per `{agent, model_group}`:

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

Register the caller entries from `callers.yaml` in the router config before running the case study. Each token allows exactly one model group. The router stores only token hashes and logs only the public `token_id`.

For production, follow the normal production config process:

1. Back up `/opt/smart-llmrouter/compose/config/config.yaml`.
2. Append the generated caller blocks with structured YAML tooling.
3. Run `sudo docker compose config >/dev/null`.
4. Restart the router container.
5. Verify `https://llm-api-engg.metrum.ai/readyz`.

After the report is collected, remove the temporary caller blocks from production config and restart the router again. Historical usage remains queryable by `token_id`.

## Run The Matrix

Hosted production example:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<generated-case-id>"
export ROUTER_BASE_URL="https://llm-api-engg.metrum.ai"
./run_case_study.sh
```

Local router example:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<generated-case-id>"
export ROUTER_BASE_URL="http://127.0.0.1:18080"
./run_case_study.sh
```

Useful overrides:

```bash
export AGENTS="codex,claude-code"
export MODEL_GROUPS="small,big-coder"
export DRY_RUN=1
```

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
model_provider = "metrum-router"

[model_providers."metrum-router"]
name = "Metrum Router"
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

docker compose exec -T router /app/bin/router-usage-report \
  --driver postgres \
  --dsn "$dsn" \
  --caller-project harbor-algotune-pca \
  --caller-environment "<generated-case-id-lowercase>" \
  --out /app/logs/harbor-agentic-usage.md
```

From this repository, the helper has the same filters:

```bash
cd examples/harbor-algotune-pca
export CASE_ID="<generated-case-id>"
export ROUTER_USAGE_DB_DRIVER=postgres
export ROUTER_USAGE_DB_DSN="$ROUTER_USAGE_DB_DSN"
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

The checked-in report at `docs/harbor-case-study.md` records a full production run on June 14, 2026:

- Task: `aider/polyglot_python_two-bucket`
- Agents: Codex CLI and Claude Code CLI
- Model groups: `default`, `fast`, `small`, `medium`, `high`, `big-coder`
- Outcome: 12/12 Harbor trials passed with reward `1.0`
- Production usage window: 54 router requests, 314,599 total tokens, 0 router errors

## Notes

- The two-bucket task asks the agent to implement `/app/two_bucket.py` with a `measure` function for the bucket-measuring puzzle.
- The scripts do not print raw provider keys.
- Generated raw router tokens are ignored by git.
- Full matrix runs can be expensive. Use `MODEL_GROUPS=small` or `DRY_RUN=1` for a smoke test first.
