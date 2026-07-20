# Prosumer Launch Decisions Register

Status labels below are intentional gates, not implementation assumptions.

**Commercial product revision (2026-07-19):** The July hosted launch sells a
base monthly Stripe subscription with a model-mix-specific included allowance
and a hard usage cap by default. This is the universally purchasable, bounded
product path. Usage above the included allowance is denied before upstream; the
customer changes plan through its authorized commercial channel. Stripe never
runs on the router inference hot path.

## D1 — Tenancy model — NEEDS-HUMAN

Question: shared multi-tenant router versus dedicated tenant instance/namespace.

* **Shared logical tenancy:** one hosted router fleet; tenant-scoped callers,
  model-group access, budgets, subscription entitlement, included-allowance
  grants, and hard-cap enforcement. Fastest and least expensive; directly reuses
  existing caller isolation. Requires rigorous authorization, noisy-neighbor
  controls, and scoped telemetry.
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

The ledger must track at least: subscription recognition, included-allowance
grant/consume/expire/reset, adjustments, reversals, and reconciliation labels.
It is **not** a prepaid-credit wallet as the
primary commercial model.

## D3 — Billing model — DECIDED (revised product shape 2026-07-19)

**Decision:** For Metrum-hosted / managed model-mix product:

1. **Base monthly subscription** via Stripe Checkout + Customer Portal
   (recurring Price; SAQ-A hosted card capture).
2. **Included usage allowance** per billing period, sized by the plan’s
   **model mix** (at minimum input tokens + output tokens; additional billable
   units only when the plan explicitly defines them).
3. **Usage beyond the included allowance** is denied before upstream by default
   with an actionable allowance/plan-limit error. The customer can change plan
   through the authorized commercial channel. This makes the launch spend cap
   legible to buyers and bounded for Metrum.
4. Stripe Billing Meters, card auto-recharge, prepaid credit packs, and
   per-request payment rails are not launch defaults. A later product decision
   may add a standard invoice/card overage rail; it must retain the same local
   admission boundary.
5. Enterprise self-hosted remains signed `license.json` (+ optional online
   lease) and is out of this hosted billing model.

Implications:

* Subscription acquisition, verified webhooks, and Customer Portal remain
  separate from router inference.
* Admission uses the **included-allowance grant** first. When allowance cannot
  cover the quote it fails closed with a documented allowance-limit error.
* Any future hosted launch plan must validate the complete subscription and
  included-allowance hard-cap journey.

## D4 — Pricing and plan catalog — NEEDS-HUMAN (numbers) / DECIDED (shape)

**Shape (decided):**

* Versioned **plan SKUs** map to:
  * base monthly Stripe Price ID;
  * allowed deployment-defined model groups (the “model mix”);
  * included billable units per period (input tokens, output tokens, and any
    plan-defined extras such as image units);
  * hard RPM/TPM/concurrency and traffic-shape defaults;
  * feature flags (admin reports, external policy, staging groups, etc.).
* Customer-facing price book stores **quoted customer charge** separately from
  **upstream provider cost**. Deterministic integer minor-unit rounding.
* Included allowance is granted at subscription period start (and on plan
  change per finance rules) and does not roll over unless Finance approves
  rollover.

**Numbers (needs Finance):** base prices, included token amounts per SKU,
minimum charge, tax, trial policy, refunds, and margin
targets. Blocks production price activation.

## D5 — AWS Marketplace motion — NEEDS-HUMAN

Options: SaaS listing with Marketplace Metering; AMI; container/EKS product;
parallel BYOC. Recommendation: do not block hosted launch on Marketplace; pursue
SaaS/private offers after the Stripe subscription path works, and retain
AMI or container packaging for enterprise BYOC. Stripe-hosted card capture keeps
PCI scope at SAQ-A. Blocks listing implementation and commercial launch claims.

Marketplace metering, if used later, is a control-plane integration and never an
inference hot-path dependency. It must not create a new unbounded hosted
overage path without an explicit product decision.

## D6 — Hosted data residency — NEEDS-HUMAN

Options: one launch region, region selectable at signup, or single-region shared
with future regional cells. Recommendation: one documented launch region and no
cross-region replication of telemetry; make customer region a control-plane field
and refuse unsupported-region signup. Blocks production topology and legal copy.

## D7 — Provider-cost exposure — NEEDS-HUMAN

Options: Metrum-pooled keys, BYOK, or hybrid. Recommendation: pooled keys only
with active subscription entitlement, included-allowance grants, and hard caps;
BYOK can be a fast-follow because it changes
credential custody and pricing. Blocks initial plan catalog and admission
policy.

Pooled traffic must never run unbounded: no active subscription / no remaining
included allowance ⇒ deny before upstream.

## D8 — Identity and transactional email — NEEDS-HUMAN

Question: which managed OIDC/email services own signup, verified email, session
claims, MFA readiness, and customer communications.

Options: Amazon Cognito plus SES; a dedicated SaaS identity provider plus its
email/integration; or a self-hosted identity stack. Recommendation: use a managed
OIDC provider with authorization-code/PKCE and verified-email claims; on the AWS
launch stack, Cognito plus SES has the smallest new vendor surface. Self-hosting
identity is not recommended for this product. Blocks future identity and console
implementation.

## D9 — Hosted DNS model — NEEDS-HUMAN

Options: one regional shared API hostname, one hostname per logical tenant, or a
dedicated hostname only for dedicated deployments. Recommendation: one regional
hostname for the shared Prosumer fleet; tenant identity comes from the caller
credential, not the hostname. Use per-tenant Route 53/TLS only for a paid dedicated
tier. Blocks future deployment topology and public onboarding copy.

## D10 — Trial and free-credit policy — NEEDS-HUMAN

Options: no free trial; time-boxed trial subscription with reduced included
allowance after verified identity/risk; or card-verified trial that converts to
paid. Recommendation: small time-boxed trial **subscription entitlement** (not
withdrawable prepaid cash) after verified identity and abuse checks, with hard
per-tenant and provider-account caps. Finance/Legal/Security must approve term,
included allowance, eligibility, region, and appeal. Blocks future abuse controls
and public pricing/trial copy.

Do **not** describe trial as “free prepaid credits” under the current vision.

## D11 — Customer-facing commercial error contract — NEEDS-HUMAN (HTTP status)

Required distinct, documented error types (names stable; HTTP status needs API
owner approval):

| Situation | Error type (stable) | Behavior |
| --- | --- | --- |
| No active subscription / entitlement | `entitlement-inactive` | Deny before upstream; link to subscribe/reactivate |
| Plan does not include model group | existing model-access denial | Unchanged |
| Included allowance insufficient | `allowance-exhausted` | Deny before upstream; link to authorized plan change |
| Hard quota/RPM/TPM/abuse cap | existing quota/traffic errors | Distinct from payment/entitlement |
| Subscription past-due after grace | `entitlement-past-due` | Deny or restricted mode per finance policy |

Recommendation: never overload provider quota/billing errors for commercial
subscription failures. Include safe request ID and console remediation URL/header.
It is an input to any future customer-facing commercial workflow.

## Historical note (superseded)

Earlier drafts recommended prepaid credits with opt-in auto-top-up as the primary
hosted billing model. That is superseded. Issue text, sprint plans, and agent
prompts must follow subscription + included allowance + hard cap for the general
launch path. Do not reintroduce prepaid packs, Stripe Billing Meters, card
auto-recharge, or per-request payment rails without an explicit new decision.
