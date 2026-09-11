---
title: Deprecated Selectors
doc_type: reference
---

# Deprecated Selectors

Legacy selectors named `latency`, `cost`, and `semantic` remain for
compatibility only (configured RPM/cost ranks and stub keyword
classification); they are not observed-signal routers. `strategy: intelligent`
is a licensed baseline-only config contract, not an active decision-model
picker.

Prefer `dynamic_score` for config-only observed-signal scoring, or
`script`/`external` (including Learned Routing Policy) for programmable
policy. See [Routing](../routing/overview) and
[Routing Strategy Decision Tree](../routing/strategy-decision-tree).
