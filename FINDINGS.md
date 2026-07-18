# Prosumer Launch Findings

Investigation date: 2026-07-18. This is planning evidence, not a statement that a
hosted self-service product has shipped.

## Current architecture

The checked-in product is a Go data plane (`cmd/router`, `internal/router`) driven
by typed YAML plus environment-backed secrets. The router has a GORM-backed usage
store (SQLite or PostgreSQL), normalized relational diagnostic children, a
Prometheus endpoint restricted to `metrics_admin` callers, and an embedded
Docusaurus product site. The browser reports UI is an operator/admin surface, not
a customer console.

There is no deployed commercial control-plane service, account system, payment
integration, customer console, tenant provisioner, or billing ledger. Existing
licensing is deliberately separate: release builds verify a signed `license.json`
locally; the router can expose safe license status and return documented
`license-*` errors. The roadmap describes online leases, but they are not a
shipped self-service fulfillment path.

The live documentation was read at `https://llm-api.metrum.ai/docs` on this date:
Overview; Licensing/Deployment Paths and Billing Enforcement; Configuration/Caller
Tokens, Caller Traffic Shaping, and Cache And Usage Store; Operations/Usage
Reporting and Observability; and Reference/Diagnostics Schema and Errors. It
confirms the intended boundary: payment, refunds, disputes, invoices, and card
data stay outside the inference path; ordinary telemetry never stores prompts,
images, bearer tokens, provider keys, token hashes, or raw tool payloads.

## Reusable foundations

* Caller authentication already hashes router tokens, enforces per-token model
  access, and makes `/v1/models` caller-filtered.
* Admission already reserves estimated input plus requested output/schema capacity
  before upstream work, reconciles actual usage, releases failed/cancelled work,
  and prevents concurrent token-budget overshoot. RPM, TPM, concurrency, rolling
  quotas, lifetime budgets, and traffic shaping are available.
* `request_usage` and normalized children persist request-time pricing, calculated
  and upstream-reported cost, provider/model/dialect, tokens, image fields,
  latency, attempts, fallbacks, and safe routing evidence. Reports sum stored
  costs rather than recomputing history from current catalog prices.
* A relational-only schema discipline, retention/legal-hold primitives, rollups,
  safe diagnostics, and Casbin-backed admin authorization are already established.
* Signed offline licenses, revocation support, safe status/readiness integration,
  and a documented future online-lease direction provide the router-side
  entitlement boundary. The commercial system must issue/renew entitlements, not
  put Stripe or card state in the hot path.
* EKS delivery work is already active: #516--#519 cover foundation, immutable
  staging delivery, promotion/cutover/rollback, and reproducible delivery.

## Productization gaps

1. A separate control plane must own identities, organizations, membership,
   tenant lifecycle, fulfillment, license/lease issuance, audit events, and
   config-as-data activation.
2. A prepaid commercial ledger and balance admission contract are needed so
   Metrum cannot incur unbounded pooled-provider spend. Router admission must use
   a locally enforceable, bounded balance/hold grant; it must never synchronously
   call Stripe or the control plane per request.
3. Stripe-hosted payment surfaces, signed idempotent webhook handling, top-ups,
   refunds/disputes, dunning, reconciliation, and support workflows do not exist.
4. Self-service signup, verification, customer API-key lifecycle, a usage/balance
   console, and first-request guidance do not exist.
5. Tenant topology, isolated secrets/config rollout, region/residency, abuse
   protection, and cancellation teardown are unresolved.
6. Launch operationalization is incomplete: control-plane SLOs, reconciliation
   dashboards, on-call/runbooks, legal terms, pricing policy, and Marketplace
   packaging remain required.

## Existing-roadmap reconciliation and deduplication

| Proposed cluster | Relationship to existing work |
| --- | --- |
| Control plane and tenant lifecycle | Extends #7; replaces the previously deferred hosted scope of closed #37 without editing it; consumes #169/#170 and #159. |
| Commercial ledger / balance grants | Reopens the product need that closed #38 explicitly deferred; extends #508 and must not duplicate #511's enterprise spend-management reporting. |
| Stripe payments and fulfillment | Replaces the self-service scope deferred in closed #37/#38; extends #171 and #170; separate from #506 x402. |
| Migration and rollout | Depends on #507 and extends #516--#519; does not duplicate their EKS delivery foundation. |
| Canaries | Extends #505 (not duplicate #504) and must use the model-contract/diagnostic evidence already in the router. |
| Console and customer reporting | Reuses router reports and extends closed #41's deferred portal scope; does not expose metrics-admin telemetry to ordinary callers. |
| Marketplace | Extends closed #40's deferred procurement scope. |
| Abuse/security/operations/docs | New launch slices that consume existing router limits, #508--#511 reporting, and #516--#519 delivery evidence. |

## Closed work and lessons

* #36 was completed: enterprise SKU/entitlement mapping is available and should
  be reused rather than introducing product-group constants.
* #37 and #38 were closed `not_planned` on 2026-06-27 under the enterprise-first
  strategy (#159), not because their implementation was completed. They are the
  closest prior design attempts; this sprint intentionally creates new scoped
  issues rather than reopening or editing them.
* #7 was deferred as post-MVP in a comment: YAML remains bootstrap/fallback and
  Go should own typed runtime reads while PostgREST is only an admin access path.
  The new work retains that boundary.
* Recent closed EKS and diagnostics issues demonstrate that production-proof,
  sanitized telemetry, and rollback artifacts are expected. Every new issue
  therefore requires captured terminal replay or screenshot evidence.
