---
title: LRP selection constraints
doc_type: explanation
---

Learned Routing Policy can apply optional selection constraints on top of
cheapest-above-floor recommendations. Operators configure these on the LRP
service; callers continue to send ordinary Chat, Responses, or Anthropic
requests to an allowed model group.

## What callers observe

- The router still filters eligible targets before asking LRP.
- Decisions return `targetIndex`, `fallbackIndexes`, and a short `classLabel`
  (at most 64 characters, router-safe charset). Labels may include coarse
  quality/cost buckets such as `lrp:caf:q0.90:c2` without model names or prompt
  content.
- When operators configure per-project quality floors, the effective floor can
  differ by the caller project metadata the router already sends. Unknown
  projects use the group default.
- Optional upstream latency gates may exclude slow targets before cost ranking.
  Cold-start targets without latency evidence stay eligible unless the operator
  chooses exclusion.
- Prompt-cache savings appear in selection estimates only when the deployment
  exposes trustworthy cache metadata and catalog cached-input prices. Session
  continuity alone does not imply a cache hit.

Predictions and floors remain operator selection criteria, not guarantees that
every answer will pass a workload verifier.

## Privacy

Ordinary labels and metrics stay coarse. Rich per-target quality, cost, latency,
and feature explanations require authenticated operator `/explain` access on the
LRP admin surface. See the operator document
[LRP selection constraints](https://github.com/metrum-ai/router/blob/main/docs/LRP_SELECTION_CONSTRAINTS.md)
and the [learned routing policy](learned-routing-policy.md) overview. Contact
[contact@metrum.ai](mailto:contact@metrum.ai) for deployment guidance.
