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
- Authenticated Prometheus-compatible `/metrics` with caller/user/project labels.

## Build And Package

The deployment artifact is a binary package. Operators should not need this source tree on the deployment host.

```bash
make build        # local router and router-token-gen binaries
make package      # dist/smart-llmrouter-<version>-linux-<arch>.tar.gz
make package-all  # linux amd64 and linux arm64 tarballs
```

Each tarball contains:

```text
bin/router
bin/router-token-gen
config/config.example.yaml
config/env.example.json
config/scripts/router.ts
docs/README.md
docs/DEPLOYMENT.md
caddy/Caddyfile
```

The packaged config expects the routing script at `config/scripts/router.ts`, so the standard packaged run command is:

```bash
bin/router --config config/config.yaml
```

See `docs/DEPLOYMENT.md` for the `llm-api-engg.metrum.ai` deployment plan with Caddy TLS termination.

## Run From Source

Create a config from the example:

```bash
cp config.example.yaml config.yaml
go run ./cmd/router-token-gen generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow default,fast,big-coder
```

Save the printed `token` value as the caller's bearer token, and copy the generated `callers:` entry into `config.yaml`. Tokens use the traceable prefix `rtr_metrum_<user>_<project>_<env>_<key>_<secret>`, while the router stores only `token_sha256` and logs/exports only `token_id`.

Provider keys are read from `env.json` in this project before `${VAR}` references in `config.yaml` are expanded. Real `env.json` is gitignored; use `env.example.json` as the template.

```bash
go run ./cmd/router --config config.yaml
```

If a variable is already set in the shell, the shell value wins over `env.json`. This lets CI or one-off live tests override local secrets without editing files.

In a packaged deployment, put provider keys in `config/env.json` beside `config/config.yaml`. The same loading rule applies: shell environment values win over `env.json`.

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
- `openai-chat` and `openai-responses` targets cover OpenAI-compatible providers such as OpenAI, Moonshot/Kimi, Qwen, MiniMax, OpenRouter, and xAI. The example catalog includes newer Kimi `kimi-k2.7-code`, MiniMax `MiniMax-M3`, and OpenRouter coding `:nitro` entries alongside lower-cost fallback models.
- OpenRouter can also be configured through its Anthropic-compatible skin with `base_url: https://openrouter.ai/api`, `dialect: anthropic`, and `auth_scheme: bearer`.
- `replicate` targets call Replicate Predictions. Use `target.model` as `owner/model-name`, for example `meta/meta-llama-3-70b-instruct`.

## API Key Flow

The router uses two different classes of keys:

- Caller tokens authenticate clients that call this router. A caller sends `Authorization: Bearer <router-token>` or `X-API-Key: <router-token>`. The router hashes the presented token with SHA-256, compares it to configured `callers[].token_sha256`, checks `allow`, rate limits, quotas, and lifetime token budget, then logs/exports only caller metadata and `token_id`.
- Provider API keys authenticate the router to upstream LLM providers. They come from `providers.<name>.api_key`, usually via `${OPENAI_API_KEY}`, `${OPENROUTER_API_KEY}`, `${MOONSHOT_API_KEY}`, and similar values loaded from `env.json` or the shell. The router injects the selected provider key only when calling the selected upstream target.

Raw caller tokens, caller token hashes, and raw provider API keys are not exposed to TypeScript routing scripts, logs, metrics, or responses. Scripts get safe identifiers only: caller `id`, `user`, `project`, `environment`, `tokenId`, and target `keyId`, `apiKeyEnv`, and `keyConfigured`. This is enough to route by caller key prefix or by the configured provider key name without making secrets available to script code.

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

For providers that use Anthropic Messages shape but bearer-token authentication, set `auth_scheme: bearer`:

```yaml
providers:
  openrouter_anthropic:
    base_url: https://openrouter.ai/api
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${OPENROUTER_API_KEY}
    models:
      claude-sonnet-46-nitro: { model: anthropic/claude-sonnet-4.6:nitro, tier: heavy, weight: 2 }
```

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

The script must export `route(ctx)` and return one configured target by index or by `{ provider, model }`. The default `scripts/router.ts` does three things:

- Removes targets whose provider key is not configured or whose target weight is zero.
- Applies named regex rules against safe caller-key metadata and safe target-key metadata.
- Falls back to weighted random routing across eligible targets, using configured target weights as relative probabilities.

Minimal weighted example:

```ts
export function route(ctx) {
  const eligible = ctx.targets
    .map((target, index) => ({ target, index, weight: Math.max(0, target.weight || 1) }))
    .filter((entry) => entry.weight > 0 && entry.target.keyConfigured);
  return { targetIndex: eligible[0]?.index || 0, classLabel: "script:minimal" };
}
```

The router transpiles TypeScript with embedded esbuild and evaluates it with an embedded JS runtime. Scripts receive request metadata/text, safe caller metadata, and configured target metadata including provider, model, modelRef, baseUrl, dialect, weight, keyId, apiKeyEnv, and keyConfigured. Raw provider API keys, raw caller tokens, and caller token hashes are never passed to scripts; returned targets are validated against the configured list.

Caller metadata enables key-specific routing with regular expressions over the generated router-token prefix. Target metadata also lets the script route to targets backed by a specific configured provider key identifier or environment variable name:

```ts
export function route(ctx) {
  if (
    /^rtr_metrum_chetan_metrum-insights_prod_/.test(ctx.caller?.tokenId || "") &&
    /^metrum-insights$/.test(ctx.caller?.project || "")
  ) {
    const heavyIndex = ctx.targets.findIndex((target) =>
      target.tier === "heavy" &&
      (
        (/^openrouter-default$/.test(target.keyId || "") &&
          /^OPENROUTER_API_KEY$/.test(target.apiKeyEnv || "")) ||
        (/^openai-default$/.test(target.keyId || "") &&
          /^OPENAI_API_KEY$/.test(target.apiKeyEnv || ""))
      ) &&
      target.keyConfigured
    );
    if (heavyIndex >= 0) {
      return { targetIndex: heavyIndex, classLabel: "key-regex:prod-heavy" };
    }
  }
  return { targetIndex: 0, classLabel: "default" };
}
```

`ctx.caller.tokenId` is the generated token prefix without the secret suffix, for example `rtr_metrum_chetan_metrum-insights_prod_key1`. Use it for traceable key classes. Do not route on raw token secrets; the router never passes them to scripts.

Check which names are present without printing secret values:

```bash
env | grep -E '^(ANTHROPIC|OPENAI|MOONSHOT|KIMI|QWEN|MINIMAX|OPENROUTER|REPLICATE|XAI)_.*=' | sed 's/=.*/=***REDACTED***/'
```

Health checks:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl -H "Authorization: Bearer $ROUTER_TOKEN" http://127.0.0.1:8080/metrics
```

## Make Targets

```bash
make test       # Go unit tests
make build      # build ./router and ./router-token-gen
make build-all  # build linux amd64 and linux arm64 binaries under dist/build
make package    # build one tarball with binaries, config, docs, tools, and Caddyfile
make package-all # build linux amd64 and linux arm64 tarballs
make e2e-mock   # local mock Claude/Codex C harness
make e2e-live-c # live OpenRouter :nitro C-generation e2e through Claude Code and Codex
```

`make e2e-live-c` starts the router once per OpenRouter sample target, runs both local CLIs, extracts the generated C source, compiles it with `cc -std=c11 -Wall -Wextra -Werror`, and runs the binary. It reads the project `env.json` before invoking the router. To keep logs and generated C files:

```bash
KEEP_LIVE_E2E_WORKDIR=1 make e2e-live-c
```

To run one live case:

```bash
LIVE_E2E_CASE_REGEX=or-kimi-k27 make e2e-live-c
```

## CLI Smoke Tests

The following commands were tested locally with `Claude Code 2.1.165`, `codex-cli 0.139.0`, router port `18081`, and OpenRouter model `qwen/qwen3.7-max:nitro`. They require `OPENROUTER_API_KEY` in the project `env.json`.

Create a temporary router config and caller token:

```bash
make build

export WORK=/tmp/smart-llmrouter-readme-smoke
rm -rf "$WORK"
mkdir -p "$WORK"

./router-token-gen generate \
  --user readme \
  --project metrum-insights \
  --env dev \
  --allow cli-smoke \
  --format json > "$WORK/token.json"

python3 - <<'PY'
import json
import os
import shlex
from pathlib import Path

work = Path(os.environ["WORK"])
generated = json.loads((work / "token.json").read_text())
(work / "token.env").write_text(
    f"ROUTER_TOKEN={shlex.quote(generated['token'])}\n"
    f"ROUTER_MODEL=cli-smoke\n"
)
(work / "config.yaml").write_text(f"""server:
  listen: ":18081"
  cache: {{ enabled: false }}
  logging:
    path: {work}/requests.jsonl
state_path: {work}/state.json
providers:
  openrouter:
    base_url: https://openrouter.ai/api/v1
    dialect: openai-chat
    api_key: ${{OPENROUTER_API_KEY}}
    api_key_env: OPENROUTER_API_KEY
    key_id: openrouter-readme-smoke
models:
  cli-smoke:
    strategy: static
    targets:
      - {{ provider: openrouter, model: "qwen/qwen3.7-max:nitro" }}
callers:
  - id: readme-metrum-insights-dev
    user: readme
    project: metrum-insights
    environment: dev
    token_sha256: "{generated['token_sha256']}"
    token_id: "{generated['token_id']}"
    allow: ["cli-smoke"]
    rate: {{ rpm: 120, tpm: 200000, concurrent: 4 }}
    quota:
      day: {{ requests: 1000, tokens: 2000000 }}
      month: {{ tokens: 10000000 }}
      soft_pct: 80
    key: {{ lifetime_tokens: 10000000, soft_pct: 90, on_exhaust: disable }}
""")
PY
```

Start the router in one terminal. This form intentionally reads the project `env.json` for the smoke test, so a stale shell variable does not override the tested provider key:

```bash
export WORK=/tmp/smart-llmrouter-readme-smoke

OPENROUTER_API_KEY=$(python3 - <<'PY'
import json
from pathlib import Path
print(json.loads(Path("env.json").read_text())["OPENROUTER_API_KEY"])
PY
) ./router --config "$WORK/config.yaml"
```

Then run the CLI checks in another terminal:

```bash
export WORK=/tmp/smart-llmrouter-readme-smoke

set -a
. "$WORK/token.env"
set +a
```

### Claude Code

Claude Code uses Anthropic-style requests. Set both token variables so installations that prefer either `ANTHROPIC_AUTH_TOKEN` or `ANTHROPIC_API_KEY` send the router caller token.

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:18081"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"
export ANTHROPIC_API_KEY="$ROUTER_TOKEN"
export ANTHROPIC_MODEL="$ROUTER_MODEL"

claude --bare --print --model "$ROUTER_MODEL" \
  "Reply with exactly: router claude ok"
```

Expected output:

```text
router claude ok
```

Expected log fields include `client=claude-code`, `inbound_dialect=anthropic`, `requested_model=cli-smoke`, and a concrete target provider/model. Provider keys must not appear in output or logs.

### Codex CLI

Codex is configured with ephemeral provider settings and the OpenAI Responses wire API:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
mkdir -p "$WORK/codex-work"

codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  -C "$WORK/codex-work" \
  -c "model=\"$ROUTER_MODEL\"" \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:18081/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex ok" </dev/null
```

Expected final assistant output:

```text
router codex ok
```

Expected log fields include `client=codex`, `inbound_dialect=openai-responses`, `requested_model=cli-smoke`, and no leaked credentials. A local Codex installation may print a bubblewrap/user-namespace warning; that is separate from the router request and does not indicate provider failure.

## Test

```bash
go test ./...
go build ./cmd/router
```

The automated suite uses deterministic mock upstreams. The Claude Code and Codex commands above are the live provider acceptance gates.
