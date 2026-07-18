# Prosumer Launch Decisions Register

Status labels below are intentional gates, not implementation assumptions.

## D1 — Tenancy model — NEEDS-HUMAN

Question: shared multi-tenant router versus dedicated tenant instance/namespace.

* **Shared logical tenancy:** one hosted router fleet; tenant-scoped callers,
  model-group access, budgets, and balance grants. Fastest and least expensive;
  directly reuses existing caller isolation. Requires rigorous authorization,
  noisy-neighbor controls, and scoped telemetry.
* **Dedicated tenant instance/namespace:** one namespace/pod/config for each
  customer. Stronger blast-radius and network isolation, but makes provisioning,
  upgrades, cost, observability, and teardown materially more complex.
* **Hybrid:** shared Prosumer tier and a later dedicated tier. Adds product and
  operational complexity now.

Recommendation: launch a shared logical-tenancy Prosumer tier with hard per-tenant
limits and reserve dedicated deployment for a paid managed tier. Blocks tenant
provisioning topology and the production launch configuration.

## D2 — Ledger technology — NEEDS-HUMAN

Question: TigerBeetle or Postgres double-entry ledger.

* **Postgres:** relational, transactional, familiar alongside the control plane;
  lowest launch operations cost. Require append-only journal/posting tables,
  idempotency keys, serialization tests, and a clean `Ledger` interface.
* **TigerBeetle:** purpose-built high-throughput double-entry correctness and
  auditability; appropriate if event rate/financial-control requirements justify
  a second stateful cluster. Adds provisioning, backup, monitoring, and developer
  expertise.
* **Unstructured wallet counters:** rejected; cannot prove debit/credit integrity.

Recommendation: Postgres double-entry at launch behind a narrow interface, with
TigerBeetle adapter acceptance tests specified but not deployed. Revisit before
launch if forecast exceeds 100 ledger postings/sec sustained or finance requires
TigerBeetle-grade controls. Blocks final ledger schema and infrastructure scope.

## D3 — Billing model — NEEDS-HUMAN

Options: prepaid credits with optional auto-top-up; Stripe metered invoices;
hybrid. Recommendation: prepaid credits plus opt-in auto-top-up and a small,
abuse-gated trial credit. It bounds provider-cost exposure and reduces bill shock.
Metered invoicing is fast-follow only. Blocks public checkout copy and dunning.

## D4 — Pricing and margin — NEEDS-HUMAN

Options: uniform markup, per-model-group price book, or pass-through plus
platform fee. Recommendation: versioned per-model-group price book with an
explicit markup, minimum charge, and deterministic microcredit rounding; store
quoted price and upstream cost separately. Finance must approve numbers, taxes,
trial credit, hold buffer, and refund policy. Blocks production price activation.

## D5 — AWS Marketplace motion — NEEDS-HUMAN

Options: SaaS listing with Marketplace Metering; AMI; container/EKS product;
parallel BYOC. Recommendation: do not block hosted launch on Marketplace; pursue
SaaS/private offers after the Stripe/control-plane path works, and retain AMI or
container packaging for enterprise BYOC. Stripe-hosted card capture keeps PCI
scope at SAQ-A. Blocks listing implementation and commercial launch claims.

## D6 — Hosted data residency — NEEDS-HUMAN

Options: one launch region, region selectable at signup, or single-region shared
with future regional cells. Recommendation: one documented launch region and no
cross-region replication of telemetry; make customer region a control-plane field
and refuse unsupported-region signup. Blocks production topology and legal copy.

## D7 — Provider-cost exposure — NEEDS-HUMAN

Options: Metrum-pooled keys, BYOK, or hybrid. Recommendation: pooled keys only
with prepaid grants/holds and hard caps; BYOK can be a fast-follow because it
changes credential custody and pricing. Blocks initial price book and balance
admission policy.
