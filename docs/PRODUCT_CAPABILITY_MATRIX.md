# Product Capability Matrix

This matrix records what is visible in this repository and current docs. Keep it current when adding runtime behavior, config, or product claims.

| Area | Status | Notes |
|---|---|---|
| OpenAI Chat Completions | Implemented | `/v1/chat/completions` request shape supported. |
| OpenAI Responses | Implemented | Used by Codex CLI and Responses-compatible tool flows. |
| Anthropic Messages | Implemented | Used by Claude Code CLI and Anthropic-compatible clients. |
| Filtered model discovery | Implemented | `/v1/models` returns groups allowed for the caller token. |
| Caller tokens | Implemented | Raw tokens are generated once; config stores hashes. |
| Per-key allow lists | Implemented | Disallowed model requests return before provider routing. |
| Rate/usage limits | Implemented | RPM, TPM, concurrency, caller/server traffic shaping, quota, and lifetime budget fields exist in config/state behavior. |
| Weighted/failover/static routing | Implemented | Configured under deployment-defined model groups. |
| Dynamic-score routing | Implemented | In-process rolling observations for latency, throughput, reliability, plus catalog cost and evaluation metadata. Response-cache hits are excluded from adaptive observations. |
| Model-group routing contracts | Implemented | Optional `models.<group>.contract` filters group-local targets by declared API surfaces, capability requirements, validation metadata, quality floors, and operational thresholds before strategy selection. |
| TypeScript routing | Implemented | Scripts run inside router with safe context and optional allowlisted HTTP helper. |
| External routing policy | Implemented | Trusted HTTP policy service receives safe context (optional include_request) and returns validated group-local targets. |
| External provider keys | Implemented | Provider credentials are server-side env/config values. |
| Tool-aware routing | Implemented | Tool support metadata is dialect-specific. |
| Image/VLM-aware routing | Implemented | Image content is detected and target eligibility uses `input_modalities`. |
| Request-time cost accounting | Implemented | Usage rows/logs include configured price fields and calculated costs. |
| Upstream-reported billed cost | Implemented where provider returns it | Stored separately from calculated router cost when available. |
| Usage DB/reporting | Implemented | GORM-backed relational usage storage and markdown report tool. |
| Browser admin reports | Implemented first slice | Disabled by default; browser-admin identity plus Casbin policy gates embedded report UI, domain-scoped JSON APIs, request drilldown/evidence bundles, spreadsheet-safe CSV, escaped Markdown export, and shared short-label chart legends for long category buckets under `/admin/reports/`. |
| Diagnostics tables | Implemented | Attempts, trace events, and sanitized request errors. |
| Governed content capture foundation | Implemented first slice | Disabled by default; enabled capture requires AES-256-GCM application encryption, a configured KMS key ID, out-of-band KMS-backed key material, content-admin delete/purge, retention timestamps, and audit events. Export/read APIs remain follow-ups. |
| Commercial retention foundation | Implemented first slice | Disabled by default; config-derived policy/rule rows, legal holds and audits, dry-run status jobs, and batch purge for usage diagnostics, governed content capture, and rollup-gated usage detail. Archive/export, scheduler, remaining purge classes, and full admin UI/API workflows remain follow-ups. |
| Response cache | Implemented | In-process LRU/TTL for eligible non-tool requests. |
| Prometheus metrics | Implemented | Restricted to caller subjects authorized for `metrics` `read`; existing `metrics_admin: true` callers remain compatible. |
| Embedded hosted docs | Implemented | Docusaurus output embedded in release binaries. |
| Version metadata | Implemented | `/version`, health responses, docs badge, and headers. |
| Codex CLI smoke | Implemented operationally | Production smokes validate tool file creation. |
| Claude Code CLI smoke | Implemented operationally | Production smokes validate tool file creation. |
| Warp/OpenAI Chat tools | Supported by API shape | Validate with OpenAI Chat tool smoke when changing tool routes. |
| Signed license enforcement | Implemented | Signed Ed25519 envelope; capability, time, and volume entitlements enforced in-router. |
| Public model marketplace | Not a product goal | Can use marketplace providers as upstreams. |
| Enterprise web dashboard | Partial | Admin browser reporting is present; broader SSO/session administration and compliance workflow automation remain follow-ups. |
| Automated model quality oracle | Not implemented as a generic feature | Use explicit evals and smokes for model activation. |
| Metrum customer control plane / commercial fulfillment | Tracked | #921 owns Stripe-verified purchase, entitlement, and commerce fulfillment enqueue; #42 owns customer-facing commercial and package copy. Enterprise keeps signed licenses and external finance; the router must not become a billing ledger, card-processing surface, or holder of private signing keys. |
