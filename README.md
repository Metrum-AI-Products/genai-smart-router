# Smart LLM Router

Go implementation of the Smart LLM Router described in `LLM_Router_SRS_1.docx`.

Current MVP capabilities:
- Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses ingress.
- Anthropic token-count estimate endpoint for Claude Code startup.
- Bearer-token auth using configured SHA-256 token hashes.
- Config-driven model groups and static, weighted, failover, latency, cost, and stub semantic routing.
- TypeScript routing scripts for custom model-selection logic inside the Go router.
- Separate caller dialects from upstream provider adapters: callers can use Anthropic/OpenAI wire formats while targets route to Anthropic, OpenAI-compatible providers, or Replicate.
- Server-side provider key injection.
- Unary upstream proxying with caller-dialect response encoding.
- Caller-facing SSE framing for streaming requests.
- In-process LRU+TTL cache for eligible unary responses.
- Per-caller RPM, TPM, concurrency, rolling quota, and lifetime key budget enforcement.
- Disk-persisted quota/key state.
- JSONL request logs using the SRS schema.

## Run

Create a config from the example:

```bash
cp config.example.yaml config.yaml
export ROUTER_TOKEN="rtr_alice_local_replace"
python3 - <<'PY'
import hashlib, os
print(hashlib.sha256(os.environ["ROUTER_TOKEN"].encode()).hexdigest())
PY
```

Put the printed hash in `callers[0].token_sha256`. Provider keys are read from `env.json` in this project before `${VAR}` references in `config.yaml` are expanded. Real `env.json` is gitignored; use `env.example.json` as the template.

```bash
go run ./cmd/router --config config.yaml
```

If a variable is already set in the shell, the shell value wins over `env.json`. This lets CI or one-off live tests override local secrets without editing files.

Expected provider env vars in `config.example.yaml`:

```bash
ANTHROPIC_API_KEY
OPENAI_API_KEY
MOONSHOT_API_KEY
KIMI_API_KEY
QWEN_API_KEY
MINIMAX_API_KEY
OPENROUTER_API_KEY
REPLICATE_API_KEY
XAI_API_KEY
```

Provider adapter notes:
- `anthropic` targets call Anthropic Messages.
- `openai-chat` and `openai-responses` targets cover OpenAI-compatible providers such as OpenAI, Moonshot/Kimi, Qwen, MiniMax, OpenRouter, and xAI.
- `replicate` targets call Replicate Predictions. Use `target.model` as `owner/model-name`, for example `meta/meta-llama-3-70b-instruct`.

## Provider Model Catalogs

A provider can serve many locally configured models without repeating its base URL or credentials:

```yaml
providers:
  openai:
    base_url: https://api.openai.com/v1
    dialect: openai-responses
    api_key: ${OPENAI_API_KEY}
    api_key_env: OPENAI_API_KEY
    key_id: openai-default
    models:
      gpt55: { model: gpt-5.5, tier: heavy, weight: 3 }
      gpt54mini: { model: gpt-5.4-mini, tier: cheap, weight: 2 }

models:
  default:
    strategy: script
    script: scripts/router.ts
    targets:
      - { provider: openai, model_ref: gpt55 }
      - { provider: openai, model_ref: gpt54mini, weight: 5 }
```

`model_ref` is local to its provider. Target-local fields override catalog defaults, so the second target above uses the same external model as `gpt54mini` but overrides its weight to `5`. Direct `{ provider, model }` targets are still supported.

## TypeScript Routing

Use `strategy: script` on a model group and point `script` at a TypeScript file:

```yaml
models:
  default:
    strategy: script
    script: scripts/router.ts
    targets:
      - { provider: openai, model_ref: gpt55 }
      - { provider: anthropic, model_ref: opus }
```

The script must export `route(ctx)` and return one configured target by index or by `{ provider, model }`. The default script uses target weights as relative probabilities:

```ts
export function route(ctx) {
  const total = ctx.targets.reduce((sum, target) => sum + target.weight, 0);
  const heavy = ctx.text.toLowerCase().includes("architecture") && total > 0;
  return {
    targetIndex: heavy ? 1 : 0,
    classLabel: heavy ? "script:heavy" : "script:cheap",
  };
}
```

The router transpiles TypeScript with embedded esbuild and evaluates it with an embedded JS runtime. Scripts receive request metadata/text and configured target metadata including provider, model, modelRef, baseUrl, dialect, weight, keyId, apiKeyEnv, and keyConfigured. Raw provider API keys are never passed to scripts; returned targets are validated against the configured list.

Check which names are present without printing secret values:

```bash
env | grep -E '^(ANTHROPIC|OPENAI|MOONSHOT|KIMI|QWEN|MINIMAX|OPENROUTER|REPLICATE|XAI)_.*=' | sed 's/=.*/=***REDACTED***/'
```

Health checks:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

## Live Claude Code Gate

Installed version observed locally: `Claude Code 2.1.165`.

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"
export ANTHROPIC_MODEL="default"

claude --bare --print --model default \
  "Reply with exactly: router claude ok"
```

Expected log fields: `client=claude-code`, `inbound_dialect=anthropic`, `requested_model=default`, and a concrete target provider/model. Provider keys must not appear in output or logs.

## Live Codex Gates

Installed version observed locally: `codex-cli 0.139.0`.

Responses API:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"

codex exec --ignore-user-config --ephemeral \
  -c 'model="default"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:8080/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex responses ok"
```

Chat Completions API:

```bash
codex exec --ignore-user-config --ephemeral \
  -c 'model="default"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:8080/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="chat"' \
  "Reply with exactly: router codex chat ok"
```

Expected log fields: `client=codex`, `inbound_dialect=openai-responses` or `openai-chat`, `requested_model=default`, and no leaked credentials.

## Test

```bash
go test ./...
go build ./cmd/router
```

The automated suite uses deterministic mock upstreams. The Claude Code and Codex commands above are the live provider acceptance gates.
