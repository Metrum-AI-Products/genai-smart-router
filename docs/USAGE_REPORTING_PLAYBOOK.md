# Usage Reporting Playbook

Usage reports support cost governance, quota reviews, incident analysis, and savings analysis.

## Standard Dimensions

Report by token ID, caller user/project/environment, model group, provider/model/dialect, client type, caller IP, hour/day, cache status, request status, input/output/image tokens, request-time USD cost, and performance fields.

Performance sections are included for latency triage:

- Downstream user performance groups by user, project, environment, and client with average/max latency, TTFB, downstream duration, downstream token throughput, errors, streams, and fallbacks.
- Upstream endpoint performance groups by provider, model, and API dialect with average/max upstream duration, latency, TTFB, upstream token throughput, attempts, fallbacks, errors, and cost.

## Common Reports

Daily usage:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

Filtered benchmark or project report:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-project <project> \
  --caller-environment <environment> \
  --resolved-group <model-group> \
  --client <client> \
  --out /app/logs/usage-filtered.md
```

## Savings Reports

Use stored request-time calculated costs for actual route cost. Compare to a documented baseline model and price, state the baseline price source and date, and include both input and output tokens. Do not recalculate historical actual costs using current provider prices.

## Image Cost Reporting

For VLM requests, normal input-token pricing is the fallback. `image_input_price_per_million_tokens_usd` applies when upstream reports image tokens. `image_input_price_per_image_usd` applies for per-image billing or internal chargeback. Upstream-reported billed cost should be stored separately when the provider returns it.

## Data Handling

Reports may include public token IDs, caller metadata, providers, models, counts, costs, latency, and statuses. Reports must not include raw tokens, provider keys, token hashes, raw prompts, raw image payloads, or unsanitized provider responses.
