---
title: Hosted Quickstart
---

# Hosted Quickstart

Use the hosted router endpoint with a router-issued caller token. The token authenticates the caller to Metrum Smart LLM Router; provider keys stay server-side.

<div class="contactBanner">
  <p>Need a hosted evaluation or deployment? Email <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Environment

```bash
export ROUTER_BASE_URL="https://llm-api-engg.metrum.ai"
export ROUTER_TOKEN="rtr_metrum_<user>_<project>_<env>_<key>_<secret>"
export ROUTER_MODEL="default"
```

## OpenAI-Compatible Chat

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"$ROUTER_MODEL"'",
    "messages": [
      {"role": "user", "content": "Reply with exactly: router ok"}
    ]
  }'
```

## Anthropic-Compatible Messages

```bash
curl "$ROUTER_BASE_URL/v1/messages" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"$ROUTER_MODEL"'",
    "max_tokens": 32,
    "messages": [
      {"role": "user", "content": "Reply with exactly: router ok"}
    ]
  }'
```

## Model Discovery

`/v1/models` is filtered by the caller token allow list.

```bash
curl "$ROUTER_BASE_URL/v1/models" \
  -H "Authorization: Bearer $ROUTER_TOKEN"
```

If a caller token is limited to `default`, `fast`, and `small`, premium groups such as `high` or `big-coder` will not be listed and cannot be requested.
