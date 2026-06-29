---
title: Cost Governance
doc_type: explanation
---

# Cost Governance

GenAI Smart Router treats cost control as a routing and accounting problem. Provider selection, caller authorization, quotas, caching, and usage reporting are handled centrally instead of being left to each application.

## Request-Time Cost Accounting

Provider catalog metadata can include:

- input-token price;
- output-token price;
- image-token price;
- per-image chargeback price;
- pricing source;
- pricing update date;
- pricing notes.

When a request finishes, the router stores the price values and calculated costs used for that request. This keeps historical reports stable even if provider prices or internal chargeback rates change later.

If an upstream returns billed-cost metadata, the router can store that separately from router-calculated cost.

## Chargeback Dimensions

Usage reports can group traffic by:

- public token ID;
- caller user, project, and environment;
- client type;
- router model group;
- selected provider and model;
- hour or day;
- caller IP;
- request status;
- cache hit, miss, or bypass;
- input, output, and image tokens;
- calculated and upstream-reported costs.

These dimensions support internal chargeback, quota review, provider evaluation, and savings analysis.

## Cost Controls

GenAI Smart Router can reduce uncontrolled spend through:

- per-key allow lists;
- RPM and TPM limits;
- concurrency limits;
- daily, monthly, and lifetime token/request budgets;
- lower-cost default model groups;
- low-weight fallback targets;
- caching for eligible non-tool requests;
- target filtering for tools, images, and cap behavior.

Token-budget admission uses a reservation estimate before upstream calls: estimated input tokens plus the caller's requested output cap. This makes `max_tokens`, `max_completion_tokens`, and `max_output_tokens` part of cost governance, not just provider generation controls. In-flight reservations count against TPM, daily token, monthly token, and lifetime key budgets until the request completes, fails, or is canceled; cache hits avoid token reservation and persisted token quota.

Platform teams keep the cost policy explicit: choose which routes are available, set budgets and weights, decide when caching is appropriate, and measure the result with durable usage and cost reports.

## Savings Analysis

For savings reports:

- use stored request-time costs for actual router traffic;
- document the baseline model and price separately;
- include both input and output tokens;
- keep image costs separate when applicable;
- state the price source and date for any comparison baseline.

Use stored request-time values for actual historical router cost. They are the durable financial record even when current provider metadata changes.

For anonymized graphical examples that show daily savings, caller-cohort savings, and upstream performance triage, see [Report Examples](/docs/operations/report-examples).

Deployments can also enable [Admin Browser Reports](/docs/operations/admin-browser-reports) for operator-only inspection of cost, token, cache, latency, fallback, provider/model, and request-drilldown data. That surface is disabled by default and requires browser-admin identity plus Casbin report authorization.

## Image And VLM Costs

For VLM requests:

- standard input-token pricing is the fallback;
- `image_input_price_per_million_tokens_usd` applies when the upstream reports image tokens;
- `image_input_price_per_image_usd` applies when the provider or enterprise chargeback model bills per image;
- upstream-reported billed cost is stored separately when available.

This makes image analysis, OCR, and browser-control workloads visible in the same reporting model as text traffic.
