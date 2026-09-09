---
title: Learned routing policy
doc_type: explanation
---

Learned Routing Policy (LRP) recommends the lowest-cost eligible target predicted
to meet an operator-defined quality floor. Different models are good at different
jobs and to different degrees. Teams establish the cheapest sufficient model mix
using objective outcomes, such as unit tests, extraction accuracy, tool-call
correctness, browser tasks or product acceptance tests.

## What callers request

Use an allowed deployment-defined model group from
[Available Models and Access](../getting-started/available-models.mdx). The same
Chat, Responses or Anthropic client request reaches the router. Operators decide
which groups use a learned policy and validate each provider, dialect, tool mode
and request shape independently. Choosing a group does not bypass access,
capability, output-cap, budget or rate-limit checks.

The router filters targets before asking LRP for a recommendation. LRP predicts
quality and output-token count for eligible targets and compares their expected
costs. If no predicted quality meets the floor, it chooses the highest predicted
quality. Unknown pricing cannot make a target appear free. Fallbacks remain
within eligible targets. Predictions are estimates; a quality floor is an
operator's selection criterion, not a guarantee that every answer passes.

## Training and evaluation

The standalone `lrp` training CLI collects approved datasets, fans requests out to
candidate targets, verifies or judges responses, builds shared features, trains
per-target LightGBM models, calibrates predictions, and evaluates a held-out split.
Session-based splitting prevents a conversation appearing in training and test.
Small or undertrained models are excluded. Model bundles bind their embedding
artifacts and feature definitions for consistent training and serving.

Evaluation compares learned decisions with cheapest, anchor, weighted-random,
strength-only and oracle baselines. Operators review quality, stored cost,
coverage, floor violations, calibration and performance, including response
duration and time to first byte. A cost saving is useful when workload outcomes
remain acceptable. Synthetic demonstrations prove wiring and evaluate gates;
provider-backed outcomes and actual embedding latency establish promotion evidence.
The shipped sample configuration is the source of truth for catalog pricing
evidence; live experiments refresh provider metadata before recording costs.

## Privacy and operational behavior

LRP is trusted deployment infrastructure. Request-aware routing sends normalized
content to the service only when the operator enables `include_request`.
Configured group PII filtering runs first. LRP retains no request content in
service logs; training datasets and judgments live in protected operator storage.
Third-party judging requires operator approval for the content transfer. Ordinary
router request logs contain metadata and cannot reconstruct prompts.

Optional exploration is restricted to explicitly configured calibration projects.
Optional session pins are in-memory heuristics based on group, project,
environment and first user text. Identical first prompts in a shared project can
collide; pins are disabled initially. Image-bearing requests use first eligible
order in v1; learned image-quality selection requires separate future validation.

Missing request content uses first eligible order with a degraded diagnostic.
Deadline or embedding failure uses strength-based fallback. If no bundle target
is known, the service uses first eligible order and records that condition.
Policy failures produce `502 routing-policy-error` when the operator selected
fail-closed behavior. See [Error Responses](../reference/errors.md).

## Staging and rollback

Native external-policy shadow mode records the learned recommendation while the
router serves first eligible configured order. This baseline differs from weighted
routing. Operators validate a restricted staging group before enforcement, then
require workload, security, client and request-shape acceptance before broad
promotion. Baseline mode bypasses LRP and preserves configured eligible order;
restoring a prior weighted strategy is a separate configuration rollback.

Deploy the service over loopback in the router's network namespace, such as a
sidecar in the same pod. A separate private service on a custom port does not
satisfy the router's current egress rules. Policy requests are authenticated;
diagnostics use a separate loopback listener, with authenticated explain/reload
disabled initially. Router global metrics continue to require metrics-admin
access. See [Customer-Controlled Routing](customer-controlled-routing.md) for
the external-policy contract and contact [Metrum](mailto:contact@metrum.ai) for
deployment guidance.
