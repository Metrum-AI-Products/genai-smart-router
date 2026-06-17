---
title: Image Analysis And VLM Routing
---

# Image Analysis And VLM Routing

Smart LLM Router accepts image inputs through the OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages API shapes. Image-bearing requests use the same router model-group names as text requests, but the router only selects upstream targets that advertise `image` in `input_modalities`.

## Configure A Vision-Capable Target

Add modality and pricing metadata to each upstream model after a direct provider smoke and a router-level image smoke pass.

```yaml
providers:
  xai:
    base_url: https://api.x.ai/v1
    dialect: openai-chat
    api_key: ${XAI_API_KEY}
    api_key_env: XAI_API_KEY
    key_id: xai-primary
    models:
      grok-4-3:
        model: grok-4.3
        tier: vision
        input_price_per_million_usd: 1.25
        output_price_per_million_usd: 2.50
        image_input_price_per_million_tokens_usd: 1.25
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://docs.x.ai/developers/models/grok-4.3
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_chat: [tools, structured_outputs]

models:
  vision:
    strategy: static
    targets:
      - provider: xai
        model_ref: grok-4-3
```

Use `image_input_price_per_million_tokens_usd` when the provider reports image tokens. Use `image_input_price_per_image_usd` for internal chargeback or providers that bill per image. If neither image-specific field is set, image tokens use the normal input-token price.

## OpenAI Chat Example

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "vision",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Read the receipt. Reply with only the merchant name."},
        {"type": "image_url", "image_url": {"url": "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}
      ]
    }],
    "max_tokens": 128,
    "stream": false
  }'
```

## OpenAI Responses Example

```bash
curl "$ROUTER_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "vision",
    "input": [{
      "role": "user",
      "content": [
        {"type": "input_text", "text": "Read the receipt. Reply with only the merchant name."},
        {"type": "input_image", "image_url": "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}
      ]
    }],
    "max_output_tokens": 128,
    "stream": false
  }'
```

## Anthropic Messages Example

```bash
curl "$ROUTER_BASE_URL/v1/messages" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "vision",
    "max_tokens": 128,
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Read the receipt. Reply with only the merchant name."},
        {"type": "image", "source": {"type": "url", "url": "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}
      ]
    }]
  }'
```

## Codex CLI Image Smoke

Codex CLI sends image attachments through the OpenAI Responses API shape. Configure an OpenAI-compatible provider that points at the router, then attach an image with `--image`.

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"

mkdir -p tmp/router-vision-smoke
curl -fsSL "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png" \
  -o tmp/router-vision-smoke/receipt.png

codex exec --ignore-user-config --ephemeral --skip-git-repo-check \
  --image tmp/router-vision-smoke/receipt.png \
  -c 'model="vision"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="'"$ROUTER_BASE_URL"'/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Read the attached receipt image. Reply with only the merchant name." </dev/null
```

## Claude Code Image Validation

Claude Code uses the Anthropic Messages API shape. For non-interactive validation, send the same image content blocks to `/v1/messages` with `ANTHROPIC_AUTH_TOKEN` set to the router token.

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="$ROUTER_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"

curl "$ANTHROPIC_BASE_URL/v1/messages" \
  -H "Authorization: Bearer $ANTHROPIC_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "vision",
    "max_tokens": 128,
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Read the receipt. Reply with only the merchant name."},
        {"type": "image", "source": {"type": "url", "url": "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}
      ]
    }]
  }'
```

## Routing And Logging Behavior

Image-bearing requests bypass response caching. The router logs `input_has_image`, `input_image_count`, `input_image_tokens` when the upstream reports them, calculated `image_cost_usd`, and upstream-reported billed cost when the provider includes it.

Keep catalog-only VLM candidates out of active traffic until the exact API shapes you plan to support pass. Some providers advertise image support in a model catalog before the current account, region, or endpoint can actually serve image requests.
