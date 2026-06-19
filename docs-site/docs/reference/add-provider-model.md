---
title: Add A Provider Or Model
---

# Add A Provider Or Model

Use this process before adding a new upstream model to active routing. It applies to external providers, OpenAI-compatible aggregators, Baseten-style endpoints, and self-hosted vLLM/SGLang deployments.

## 1. Capture Required Metadata

Record:

- provider name and `base_url`;
- API dialect and authentication scheme;
- served model ID;
- model modalities;
- tool support by API shape;
- pricing or internal chargeback rates;
- pricing source and update date;
- known limitations such as `honors_max_tokens: false`.

Do not add unavailable provider models to active routing. Catalog-only is acceptable when the model exists but the deployment is not entitled or validation is incomplete.

## 2. Run Direct Provider Smokes

Run direct upstream requests before involving the router:

- text completion with realistic `max_tokens` or `max_output_tokens`;
- small cap request such as `max_tokens: 1` when cap behavior matters;
- tool request for each API shape you plan to support;
- image request when declaring `image` modality.

OpenRouter Nitro variants may not appear as separate model IDs in `/models`; validate the exact `:nitro` suffix with a real completion call.

Reasoning-heavy models can return HTTP 200 with empty final content when the output budget is too small. Test both a tiny cap and a realistic budget before activating them.

## 3. Add Catalog Metadata

Add provider catalog metadata with pricing, modality, tool, and cap fields. Keep routing weights out of provider catalogs.

## 4. Add A Smoke Group First

Create a deployment-defined smoke group with one target and no broad caller access. Run router-level smokes against the same API shapes tested directly.

## 5. Add Production Weight Conservatively

Start with a low weight in active groups. Increase only after:

- request logs show normal status and latency;
- usage/cost rows are populated;
- capped requests behave as expected;
- CLI/tool/image smokes pass where relevant;
- production logs do not show repeated fallback or provider failures.

## 6. Update Docs And Reporting

Update:

- external model metadata docs if the new capability is user-visible;
- internal rollout history and deployment notes;
- sample config if the provider/model should be part of reference config;
- usage reports if a new cost or modality field affects accounting.

## Rollback

Rollback should be a config-only weight or target change when possible:

1. Set active target weight to `0` or remove it from affected groups.
2. Keep the catalog entry with notes unless the model ID was wrong.
3. Restart the router and run `/readyz`.
4. Run a request through affected groups to confirm another eligible target is selected.

