---
title: Usage, Cost, And Reports
doc_type: explanation
---

# Usage, Cost, And Reports

GenAI Smart Router records usage as operational evidence, not just a billing summary. Reports should explain who used the gateway, which model groups and upstream/provider models served requests, how much traffic cost at request time, where savings came from, and which provider/model routes were slow or unreliable.

## What The Router Records

Each completed request stores safe scalar fields such as caller identity, public token ID, requested model group, selected upstream/provider model, token counts, cache status, latency, throughput, fallback behavior, terminal status, and request-time cost values. Historical cost reports use stored request-time values instead of recalculating old usage from current provider pricing.

Diagnostics use relational child rows for attempts, trace events, errors, and decision telemetry. They do not store raw prompts, images, tool outputs, provider keys, raw router tokens, token hashes, or full runtime configuration.

## Report Questions

| Question | Start with |
|---|---|
| Who used the router and which public token ID was responsible? | Usage by user, caller, project, public token ID, and model group. |
| Which upstreams are most used? | Provider/model mix and model-group usage reports. |
| Which upstreams are slow? | Latency, TTFB, upstream throughput, downstream throughput, and attempt/fallback reports. |
| Where did savings come from? | Savings reports using stored router actual cost and source-dated baseline assumptions. |
| What failed? | Error, fallback, troubleshooting bucket, request evidence/drilldown, and terminal error reports. |
| Are limits sized correctly? | Quota, TPM/RPM, concurrency, traffic-shaping, max-token, and input-token reports. |

## Related Pages

- [Usage Reporting](../operations/usage-reporting)
- [Admin Browser Reports](../operations/admin-browser-reports)
- [Report Examples](../operations/report-examples)
- [Observability](../operations/observability)
- [Cost Governance](../evaluation/cost-governance)
- [Error Reference](../reference/errors)
