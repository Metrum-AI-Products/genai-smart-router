---
title: LRP uncertainty and exploration
doc_type: explanation
---

Learned Routing Policy can optionally abstain under calibrated uncertainty,
compare Thompson sampling with seeded epsilon-greedy exploration, monitor
feature drift, and cold-start new eligible targets. Operators configure these
on the LRP service. Defaults keep Wave-1 epsilon-greedy behavior. No live
routing activation is authorized by enabling these knobs alone.

## What callers observe

- The router still filters eligible targets before asking LRP.
- When abstention is enabled and calibrated uncertainty for the would-be
  primary exceeds the reviewed threshold, LRP may select the configured anchor
  or first fallback and emit a short `classLabel` such as `lrp:uncertain`.
- Exploration remains restricted to operator-approved calibration projects.
  Thompson draws use ensemble uncertainty; epsilon-greedy stays available for
  comparison. Explore labels stay coarse (`lrp:explore-thompson`,
  `lrp:explore-cold-start`).
- Drift monitoring produces operator-side safe scalar reports. Automatic shadow
  is only a recommendation; the router's `external_policy.mode` remains the
  activation authority.
- Cold-start exploration never adds targets the router did not already mark
  eligible.

## Privacy

Labels and drift reports stay bounded and content-free. Rich explanations still
require authenticated operator `/explain` access. See the operator document
[LRP uncertainty](https://github.com/metrum-ai/router/blob/main/docs/LRP_UNCERTAINTY.md)
and the [learned routing policy](learned-routing-policy.md) overview. Contact
[contact@metrum.ai](mailto:contact@metrum.ai) for deployment guidance.
