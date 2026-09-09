# Learned Routing Policy operator runbook

Different models complete different jobs to different degrees. The objective is
the cheapest model or mix that still completes the workload, established by
objective outcomes. LRP trains per-target quality and output-token models and
uses the router's external-policy interface to recommend a target above a chosen
quality floor. Unit tests, extraction accuracy, OCR targets, tool correctness,
browser tasks, golden datasets and product acceptance tests can establish those
outcomes; Harbor is one possible agent harness.

## Installation and protected storage

Use Python 3.12+ and uv. From the product checkout:

```bash
uv sync --project services/learned-routing-policy --locked
uv run --project services/learned-routing-policy lrp --help
make lrp-test
make lrp-synthetic-demo
```

Set `LRP_DATA_DIR` to an operator-owned directory **outside** the public product
tree, restricted to its operator. All content-bearing data, responses, judgments,
features, model artifacts and evaluation details stay there. Do not commit
captured content, caller IDs, credentials or real model outputs. Small checked-in
fixtures are synthetic. Use a unique run marker when router caching is enabled;
authenticate selection readback and match actual router request IDs. Policy
payloads themselves contain no router request ID.

Secrets come only from environment: `OPENROUTER_API_KEY`, `LRP_ROUTER_TOKEN`, and
`LRP_POLICY_AUTH_HEADER`. The last is the secret value for fixed `X-LRP-Auth`, not
a name/value pair. Never pass secrets as CLI arguments or print them. The service
discards caller IDs/token IDs and retains only project/environment for optional
session heuristics. Model files and native inference libraries are trusted code
and artifacts; validate their source and dependency/container scan results.

## Pipeline commands

These commands assume `LRP_DATA_DIR` is already set, the input files are approved,
and `lrp` is invoked with `uv run --project services/learned-routing-policy`.

```bash
lrp collect --dataset "$LRP_DATA_DIR/seed.ndjson" --approved-content --out "$LRP_DATA_DIR/requests.ndjson"
lrp collect --router-log "$LRP_DATA_DIR/requests.jsonl" --content-capture "$LRP_DATA_DIR/approved-export.ndjson" --approved-content --out "$LRP_DATA_DIR/requests.ndjson"
lrp fanout --requests "$LRP_DATA_DIR/requests.ndjson" --targets "$LRP_DATA_DIR/targets.yaml" --via router --base-url http://127.0.0.1:8080/v1 --refresh-pricing --approved-content --out "$LRP_DATA_DIR/responses.ndjson"
lrp judge --requests "$LRP_DATA_DIR/requests.ndjson" --responses "$LRP_DATA_DIR/responses.ndjson" --anchor-provider openrouter --anchor anthropic/claude-sonnet-4.6 --judge anthropic/claude-sonnet-4.6 --approved-content --out "$LRP_DATA_DIR/judgments.ndjson"
lrp featurize --requests "$LRP_DATA_DIR/requests.ndjson" --embedding-model "$LRP_DATA_DIR/embed/model.onnx" --tokenizer "$LRP_DATA_DIR/embed/tokenizer.json" --out "$LRP_DATA_DIR/features.parquet"
lrp train --features "$LRP_DATA_DIR/features.parquet" --judgments "$LRP_DATA_DIR/judgments.ndjson" --responses "$LRP_DATA_DIR/responses.ndjson" --embedding-model "$LRP_DATA_DIR/embed/model.onnx" --tokenizer "$LRP_DATA_DIR/embed/tokenizer.json" --anchor-provider openrouter --anchor anthropic/claude-sonnet-4.6 --out "$LRP_DATA_DIR/bundles"
# Set LRP_BUNDLE_DIR to $LRP_DATA_DIR/bundles/<bundle_version printed by train>.
lrp validate --bundle "$LRP_BUNDLE_DIR"
lrp eval --bundle "$LRP_BUNDLE_DIR" --features "$LRP_DATA_DIR/features.parquet" --judgments "$LRP_DATA_DIR/judgments.ndjson" --responses "$LRP_DATA_DIR/responses.ndjson" --config "$LRP_DATA_DIR/lrp.yaml" --out "$LRP_DATA_DIR/eval_report.json"
lrp serve --bundle "$LRP_BUNDLE_DIR" --config "$LRP_DATA_DIR/lrp.yaml" --port 18093 --admin-port 18094
```

Router JSONL contains metadata only. It cannot supply prompts or automatically
replay explore-tagged requests. Content capture is governed encrypted storage;
an operator must supply an approved export. Non-synthetic collection, fanout and
judging require `--approved-content` after the operator approves that transfer.
Judging sends request and candidate/anchor content to a third-party model.
Use an appropriately authorized judge and data-retention policy. Prefer a judge
outside the candidate set; self-judging is explicitly reported even with swapped
positions. Verifier outcomes, pairwise preference and absolute rubric scores
have different semantics and must be assessed separately. Parse failures and
unavailable verifier infrastructure are missing evidence, not successful outcomes.

Arbitrary pytest execution is disabled unless an explicitly configured isolated
worker enforces no network, no host files/secrets, an unprivileged UID, bounded
scratch/CPU/memory/PIDs/output, and whole-process-tree timeout cleanup. A host
subprocess and temporary directory alone do not provide isolation. Never install
requirements or execute shell commands supplied by a dataset.

Use a local trusted int8 ONNX export of
[BAAI bge-small-en-v1.5](https://huggingface.co/BAAI/bge-small-en-v1.5) with its matching
tokenizer. The manifest binds artifact hashes, feature order, embedding settings
and target model identities. No runtime download or silent synthetic fallback is
allowed. Train and serve share normalization of system plus recent turns and
truncate from the end to 512 tokens. Session-hash splitting keeps sessions out
of multiple partitions. Predictions below 200 training rows are excluded.

## Candidate targets, pricing and caps

The six historical OpenRouter candidates from issue #15 remain a **catalog-only
experiment** until exact account/model/dialect smokes pass. Actual upstream model
IDs are `model`; catalog aliases are `model_ref`. Always refresh the
[OpenRouter model metadata](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties)
for live runs and store the fetched prices with each response. A missing exact
Nitro ID or price requires operator resolution; do not infer zero or silently use
another model. The shipped sample config is historical catalog evidence, not
current cost authority. Routing weights belong to group targets.

Router fanout uses one restricted, single-target group per candidate and one
deployment-owned caller with access to those groups. Record router request IDs
and selected targets. Positive caps filter targets marked `honors_max_tokens:
false`, irrespective of cap size. For the historical set this affects MiniMax,
Qwen and Grok, not Sonnet. Capped rows are ineligible. An uncapped experiment
requires explicit `--allow-uncapped`, separate spend safeguards and an operator
budget; client truncation cannot cap billed generation. Retry costs and actual
reported billing remain distinct from terminal usage-times-price estimates.
Reports use stored prices/costs and include TTFB, duration, throughput and errors
so both cost and slow user experience can be investigated.

## Routing and deployment

Start from `services/learned-routing-policy/lrp.example.yaml`. Group names are
deployment-defined. Keep exploration and pinning disabled initially. Opt-in
exploration requires both a nonzero rate and an allowed calibration project.
Optional TTL/LRU pins hash group/project/environment/first user text in memory;
identical first prompts in a shared project can collide. This is a heuristic,
not an authenticated conversation identity. Training on PII-redacted data must
match the group's serving redaction distribution deliberately.

Configure the router group with `strategy: external`, `include_request: true`,
`url: http://127.0.0.1:18093/route`, `allow_hosts: [127.0.0.1]`,
`headers: {X-LRP-Auth: '${LRP_POLICY_AUTH_HEADER}'}`, and `mode: shadow`.
The commented sample shows the full configuration. LRP only selects from current
eligible targets, preserving indexes after router filtering. Unknown/undertrained
targets are excluded from learned predictions; with none known it returns first
eligible order with `lrp:no-known-target`. Unknown prices cannot win as free.
Image requests pass through to first eligible order; no learned image-quality
claim is made. Missing request content degrades with `lrp:no-request-content`.

The current router egress rules require same-network-namespace loopback for a
private LRP sidecar: use one Kubernetes pod or explicit Compose network sharing.
A separate private Docker/Kubernetes Service on port 18093 is rejected, even if
allowlisted. Nonlocal endpoints must meet the router's public-address/default-port
rules. Use HTTPS for nonlocal trusted infrastructure.

`/healthz`, `/readyz` and `/metrics` bind a separate **loopback** admin port.
Readiness requires a valid bundle and warmup. `--enable-admin` enables authed
`/explain`, `/admin/reload` and SIGHUP reload; candidate bundles validate/warm
before atomic replacement. In-flight requests retain their old bundle. Keep
router `/metrics` separately restricted to `metrics_admin` callers.

## Validation, promotion and rollback

Run `make lrp-test`, `make lrp-synthetic-demo`, and `make lrp-e2e`. Synthetic CI
demonstrates wiring and evaluates gates; it never demonstrates provider quality,
real bge latency or live readiness. The JSON/Markdown evaluation compares LRP
with cheapest, anchor, target-weighted random, BT-only and oracle, including a
quality-floor sweep. Promotion requires quality at least 97% of anchor, cost at
most 60%, floor violation at most 10%, applicable AUC at least .70 with 100 test
rows per active target, and quality/cost dominance over BT-only. Single-class
AUC is N/A with reason. Unknown costs, uncertain labels and no-oracle-success
rows remain visible and cannot silently improve the gates.

Manual evidence steps:

1. Run direct fanout on 50 committed synthetic prompts and six exactly validated
   candidates (`--via openrouter --refresh-pricing`); require 300 rows and at least
   95% successful responses. These provider-backed rows remain protected.
2. Judge those rows with approved settings; record coverage and judge spend.
3. Train/evaluate at least 2,000 operator-owned requests with session-disjoint
   holdout. Review all gates, label semantics, floor sweep, language distribution
   and workload acceptance. Provider-backed evidence includes actual embeddings,
   upstream responses, objective/reviewer outcomes and selection readback.
4. Measure real pinned ONNX inference and router-observed p99 policy duration on
   a 4-vCPU runner: document warmup, 500 requests, sizes, concurrency and hardware.
   The targets are embedding p99 <=15ms and total policy p99 <40ms. Synthetic
   measurements cannot satisfy either claim.
5. Run 24 hours of native router shadow on a restricted staging group. Assert
   configured-order serving baseline and `shadow_recommended` telemetry. Native
   shadow does **not** preserve a former weighted distribution.
6. After review, enforce only on staging; compare stored costs, upstream and
   downstream performance, errors and fallbacks for seven days. Broad promotion
   requires independent Chat, Responses, Anthropic, tool/large-payload and any
   bridge/modality validation. Include security scan, redaction, metrics isolation,
   network controls, client acceptance, readiness, rollback and cleanup evidence.

Enable decision telemetry explicitly for validation. Inspect safe scalar
`request_policy_executions` duration/class label/outcome plus actual target and
attempt rows; persist no policy payload JSON. Stop promotion on failed gates or
compatibility evidence. Roll back policy influence with native `mode: baseline`
(configured eligible order), or deliberately restore the prior strategy/config.
Stopping LRP with `on_error: fallback` also serves configured order; it is not an
equivalent weighted rollback. `fail_closed` produces `502 routing-policy-error`
on policy failure. Keep previous validated bundles/config for atomic rollback,
then remove temporary calibration groups/callers through normal operator control.
