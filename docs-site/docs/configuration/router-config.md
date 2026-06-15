---
title: Router Configuration
---

# Router Configuration

The router is configured with YAML plus environment-loaded provider keys. Customers normally keep provider credentials in the deployment environment or an `env.json` file outside source control.

<div class="contactBanner">
  <p>Metrum can help design a production routing policy. Contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Provider And Model Catalog

```yaml
providers:
  minimax:
    base_url: https://api.minimax.io/v1
    dialect: openai-chat
    api_key: ${MINIMAX_API_KEY}
    api_key_env: MINIMAX_API_KEY
    key_id: minimax-primary
    models:
      m3:
        model: MiniMax-M3
        tier: heavy

  openrouter:
    base_url: https://openrouter.ai/api/v1
    dialect: openai-chat
    api_key: ${OPENROUTER_API_KEY}
    api_key_env: OPENROUTER_API_KEY
    key_id: openrouter-primary
    models:
      deepseek-v4-flash-nitro:
        model: deepseek/deepseek-v4-flash:nitro
        tier: balanced

  kimi:
    base_url: https://api.moonshot.ai/v1
    dialect: openai-chat
    api_key: ${MOONSHOT_API_KEY}
    api_key_env: MOONSHOT_API_KEY
    key_id: kimi-primary
    models:
      kimi-k2-7-code:
        model: kimi-k2.7-code
        tier: coding
```

## Per-Group Weighted Routing

Weights are local to each model group. A target with weight `60` in `default` has no relationship to a target with weight `60` in `big-coder`.

```yaml
models:
  default:
    strategy: weighted
    targets:
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 60 }
      - { provider: minimax, model_ref: m3, weight: 30 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 10 }

  big-coder:
    strategy: weighted
    targets:
      - { provider: minimax, model_ref: m3, weight: 50 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 30 }
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 20 }
```

## Caller Tokens And Allow Lists

```yaml
callers:
  - id: example-standard-prod
    user: example-standard
    project: example-project
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_example-standard_example-project_prod_k20260614
    allow: [default, fast, small]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-coding-prod
    user: example-coding
    project: example-project
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_example-coding_example-project_prod_k20260614
    allow: [default, fast, small, medium, high, big-coder]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }
```

Disallowed model requests return `403 model-not-allowed` before any upstream provider key is used.

## Cache And Usage Store

```yaml
server:
  cache:
    enabled: true
    max_bytes: 134217728
    default_ttl: 15m
  logging:
    path: /app/logs/requests.jsonl
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}
```

The cache is intended for eligible deterministic unary responses. Tool-bearing agent requests bypass cache because tool output can depend on live shell and filesystem state.
