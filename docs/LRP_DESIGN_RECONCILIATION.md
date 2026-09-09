# Learned Routing Policy v1 design reconciliation

Issue [#15](https://github.com/Metrum-AI-Products/genai-smart-router/issues/15)
was checked against `51450f5` on 2026-09-09 before implementation. These corrections
supersede conflicting assumptions in the issue. The standalone Python 3.12
service and training CLI require no router Go changes.

## Existing router contract

- `external_strategy.go` implements enforce, native shadow, and baseline. LRP
  returns its real recommendation in shadow. The router serves first eligible
  configured order, not weighted routing; baseline bypasses the policy. Native
  shadow is already shipped and needs no follow-up implementation issue.
- `allTargets` is declared but omitted. Validate bundle files locally and intersect
  predictions with each request's current `targets`; retain original indexes even
  across unknown or undertrained entries. An explicit empty fallback list means
  no fallback. Never reintroduce a router-filtered target.
- `model` is the actual upstream model ID; `modelRef` is the catalog alias.
  Offline joins use `(provider, model)`. Dialect overrides may duplicate that key:
  validate compatibility and preserve separate payload indexes. Bundle filenames
  use safe IDs with manifest mapping and containment checks, never raw model IDs.
- `request` is normalized IR: separate system, messages/content/parts and
  input/input_parts. `text` alone omits system content. One bounded normalization
  feeds training and serving. Router summary lengths count UTF-8 bytes, and
  estimated tokens are a heuristic, not billed tokens. Turn, language, and lexical
  features are derived separately. There is no policy-payload request ID or
  conversation ID.
- Caller metadata includes pseudonymous IDs and token IDs; LRP immediately keeps
  only project/environment. It never logs full payloads, raw IR, validation input,
  headers, or target inventory fields. Auth uses fixed `X-LRP-Auth` with nonempty
  value from `LRP_POLICY_AUTH_HEADER`; router YAML expands that environment value.
- Pinning is disabled by default. Optional bounded TTL/LRU pins hash a canonical
  tuple of group/project/environment/first user text. Shared projects and repeated
  first prompts can collide; this is a heuristic, not caller-isolated sessions.
- The exact-host allowlist also enforces nonlocal default ports and rejects
  private/link-local IP destinations. Use loopback in the same network namespace
  (same Kubernetes pod or Compose network sharing). A separate private Service
  on port 18093 does not work with the current egress contract. Admin binds
  loopback; explain/reload are disabled by default and authenticated when enabled.

## Data, pricing, and evidence

- `requests.jsonl` is metadata only. Replay requires an operator-approved content
  export or dataset; logs alone report missing content. Governed encrypted
  capture is not an automatic plaintext export. Copy no raw caller/token IDs.
  Training distribution must match PII-filtered serving content deliberately.
- Decision telemetry is disabled by default: integration tests explicitly enable
  it. Safe policy rows provide durations and class labels; JSONL calls the field
  `policy_executions`. LRP does not write the router database.
- All six catalog IDs exist, but old prices are not current evidence. Refresh
  pricing for live runs; do not infer missing Nitro prices or treat unknown as
  zero. Explicit zero-cost evidence is required. Store usage-derived and billed
  costs separately, plus latency, TTFB and actual serving provider when returned.
- MiniMax, Qwen and Grok have `honors_max_tokens: false`; Sonnet does not. Any
  positive cap excludes those targets. Record capped requests as ineligible;
  uncapped experiments need explicit operator opt-in and spend safeguards.
  Client truncation does not cap billing. Weights belong to targets, not catalogs.
  The issue's example cost is corrected to USD 0.00012792.
- Sample LRP and fanout groups remain commented, with no active-route promotion.
  Exact provider/model/dialect validation precedes activation.
- Deterministic data verifiers precede judging. Arbitrary pytest requires an
  explicitly configured isolated worker with network/host filesystem denial,
  unprivileged UID, clean environment, resource/output/PID limits and process-tree
  cleanup. Host subprocesses alone are insufficient. Missing sandbox is an
  infrastructure/unsupported result, not evidence of model failure.
- Judge cache binds content, candidate/anchor, model/settings and prompt/verifier
  versions. Parse failures are uncertain labels excluded from learning and
  tracked in coverage. Verifier success, pairwise preference and absolute rubric
  scores are reported separately.
- Single-class AUC (including an anchor fixed at quality one) is undefined:
  report N/A with reason and gate applicable learned cohorts. Report requests
  with no successful oracle candidate; do not silently discard them or choose
  unknown-price targets as free. Evaluation includes target-weighted random,
  cheapest, anchor, BT-only and oracle baselines, floor sweep and all gates.
- Synthetic provenance survives all stages and can never pass live promotion.
  Real LightGBM training/calibration and held-out evaluation are required even
  for synthetic wiring. Synthetic embeddings do not demonstrate ONNX latency.
  Deadline handling needs bounded inference admission/workers; an elapsed-time
  check after synchronous inference is insufficient. Report hardware, warmup,
  concurrency and request sizes for any real-ONNX performance claim.

The user subsequently authorized configurable inference deadlines for
latency-tolerant workloads and testing. The default remains 200 ms; YAML
`deadline_ms` and the optional `serve --deadline-ms` override accept 1–4,500 ms.
The router policy timeout must be higher, with transport headroom, within its
5,000 ms ceiling. Bounded admission and occupied-worker accounting remain in
effect. Increasing the deadline does not turn failed embedding/feature latency
budgets into passing evidence or alter embedding truncation or default threads.

## Follow-ups and release boundary

Conversation key, completion feedback and verifier-hint passthrough are
implemented as external-policy context extensions; see
[EXTERNAL_POLICY_CONTEXT.md](EXTERNAL_POLICY_CONTEXT.md). Native shadow is already
implemented. Optional enhancements remain separate from mandatory v1. Live
fanout/judging, operator-data promotion evaluation, 24-hour shadow and subsequent
request-shape promotion are manual evidence gates. The implementation PR does not
authorize merging around branch protections or releasing before required review.
No live routes or private data enter this work.
