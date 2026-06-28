# GenAI Smart Router Commercialization Plan

This is an internal product and commercialization plan for GenAI Smart Router. It is not customer-facing pricing copy. Revalidate exact competitor pricing, procurement channels, marketplace availability, packaging, and commercial terms before publishing any public comparison or exact price claim.

Research snapshot: 2026-06-24.

Current status note: this document has been refreshed after signed license enforcement shipped. Treat the market research and price anchors below as a dated planning snapshot, not current public positioning. Current customer-facing docs live in `docs-site/docs/`, especially:

- `docs-site/docs/licensing/deployment-paths.md` for the current public commercial access map;
- `docs-site/docs/operations/license-protected-deployments.md` for shipped signed-license deployment behavior;
- `docs-site/docs/evaluation/commercial-evaluation.md` for the current commercial evaluation path;
- `docs-site/docs/operations/usage-reporting.md` for shipped usage reporting, rollup, and retention foundations.

Primary reference points reviewed:

- LiteLLM Enterprise: self-hosted/cloud enterprise license positioning, license-key unlock, deployment-size pricing, AWS/Azure Marketplace procurement.
  Source: https://docs.litellm.ai/docs/enterprise
- Helicone pricing: free/pro/team self-serve style, monthly tiers plus usage-based pricing.
  Source: https://www.helicone.ai/pricing
- Portkey positioning: AI gateway, observability, guardrails, governance, prompt and model-catalog platform.
  Source: https://portkey.ai/
- OpenRouter pricing: prepaid credits, manual/automatic top-up, enterprise by volume/prepayment/annual commit, and routing/fallback billing around successful model runs.
  Source: https://openrouter.ai/pricing

## Shipped Commercial Foundations

The router now includes several commercial foundations that were still future-looking when this plan was first written:

- Signed JSON license verification is shipped. Licensed deployments can enable `server.license` so the router verifies a Metrum-issued signed JSON license with embedded Ed25519 public verification keys, expiry, product checks, feature gates, deployment limits, grace behavior, clock-rollback state, safe status reporting, `/readyz` integration, and `license-*` caller errors. The original implementation issue #35 is closed.
- License-safe operations docs are shipped. Public docs cover license-protected deployments, configuration, renewal/replacement, admin status, metrics-admin visibility, troubleshooting, and secret-handling boundaries without publishing private signing keys, license payloads, private deployment paths, router tokens, or provider keys.
- Commercial evaluation docs are shipped. Public evaluation docs describe hosted, private-cloud, and enterprise/on-prem evaluation paths, deployment readiness, security assessment, workload validation, usage reporting evidence, and contact flow.
- Usage reporting and commercial retention foundations are shipped where documented. Request-time costs, upstream-reported billed costs, latency/throughput dimensions, safe license metadata, daily rollups, baseline savings fields, dry-run retention status, legal-hold rows, and guarded first-slice purge execution are available as documented. Archive/export automation, schedulers, broader data-class purge execution, and full browser/admin write workflows remain future slices.

Still-open commercial implementation issues:

- #36: Define commercial packaging, entitlements, and billing semantics. Current SKU mapping is now recorded in `docs/enterprise-license-skus.json` and summarized below.
- #37: Managed cloud self-serve checkout and provisioning.
- #38: Billing-grade usage metering, credits, and invoice ledger.
- #39: Enterprise self-hosted license operations and renewal workflow.
- #40: Private managed deployments and marketplace procurement.
- #41: Customer billing and admin portal for managed cloud.
- #42: Public pricing and package documentation.
- #159: Extend the license envelope and runtime enforcement to support capability, time, volume, and operational limits consistently.
- #164: Production-grade license generation tooling.
- #167: Deployment binding.
- #168: Revocation bundles.
- #169: Online license lease service.
- #170: Licensing portal.
- #171: Stripe billing and webhook fulfillment.

## Goals

- Let small teams start by credit card without a sales call.
- Preserve high-ACV enterprise paths for self-hosted, private-cloud, and Metrum-managed deployments.
- Monetize the router value separately from upstream provider cost.
- Keep packaging compatible with enterprise procurement: annual contracts, private offers, AWS/Azure Marketplace, and signed self-hosted licenses.
- Make commercial enforcement practical without blocking legitimate on-prem and air-gapped customers.
- Keep public docs generic and deployment-defined; avoid exposing private hostnames, provider keys, real router tokens, private pricing commitments, or production-only model group details.

## Commercial Access Map

Public docs should present one coherent access map:

1. **Enterprise self-hosted license.** Shipped signed `license.json` enforcement is the core path. It supports customer-operated deployments, BYOK provider keys, private upstreams, annual or contracted procurement, and offline or air-gapped operation when allowed by the contract. Future deployment binding (#167), revocation bundles (#168), and online leases (#169) are optional enforcement layers, not replacements for the offline enterprise story.
2. **Private managed deployment.** Metrum operates a dedicated deployment for one customer or a customer-specific HA environment. Do not describe this as a shared public multitenant promise. Procurement can be direct order, Stripe Quote/Invoicing, or marketplace private offer when available. Keep operational hostnames, SSH paths, token files, provider keys, and production procedures out of public docs.
3. **Self-service evaluation, pilot, renewal, or top-up.** This is planned only if #170 and #171 ship. The portal may expose approved constrained SKU/templates, collect payment through Stripe-hosted checkout, quote, or invoice flows, then generate a signed license after webhook-confirmed fulfillment. It must not become an arbitrary entitlement configurator or a public API-credit wallet inside the router.
4. **Online lease / payment-state enforcement.** Planned for monthly, card-paid, trial, usage-sensitive, or managed commercial plans where payment state and concurrent-use enforcement matter. The runtime may require periodic signed lease renewal. Contracted offline enterprise and air-gapped buyers must remain supportable with signed license files.
5. **Renewal, replacement, revocation, and top-up.** Offline licenses use file replacement. Portal re-download/top-up is planned where available. Revocation bundles and online leases should be explained publicly only at a customer-safe behavior level.

Do not publish exact prices until product, legal, and commercial owners approve them for source-controlled docs. Public copy should route buyers to `mailto:contact@metrum.ai` for enterprise, private managed, custom pilots, marketplace/private-offer, and non-standard entitlements.

## Recommended Commercial Motions

### 1. Metrum Managed Cloud

Future hosted or private managed motion. Do not publish this as an available public self-serve product until the commercial portal, billing, operations, and support workflows are implemented and approved.

Customer buys access to a hosted Metrum router endpoint. Metrum operates the router, usage DB, admin reports, billing integration, and provider connectivity.

Variants:

- Metrum-billed provider usage through credits.
- Customer BYOK provider credentials, with Metrum billing only for platform usage.
- Hybrid: Metrum-billed default providers plus customer private upstreams.

Best for:

- credit-card onboarding;
- developers and teams that want no infrastructure work;
- demos, trials, and expansion into enterprise accounts;
- usage-based packaging.

Commercial shape:

- monthly platform fee;
- included usage allowance;
- usage overage by request, token/cost unit, or credit drawdown;
- optional markup on Metrum-billed provider usage;
- paid add-ons for admin reports, dynamic routing, longer retention, external policy, private upstreams, SSO, and support.

### 2. Enterprise Self-Hosted License

Customer runs the Docker package or binary in their infrastructure with a signed license file.

Best for:

- regulated environments;
- VPC/on-prem buyers;
- customers with strict data-residency requirements;
- customers with existing provider contracts and BYOK;
- annual contracts and procurement.

Commercial shape:

- annual prepaid license;
- signed JSON license with expiry and feature entitlements;
- support/SLA tier;
- limits by production instances, annual request/token volume, admin seats, model groups, or enabled features;
- procurement through direct order form and later AWS/Azure Marketplace private offers.

Implementation status: signed JSON license enforcement has shipped. The remaining commercial work is operational packaging around license issuance, renewal, support workflows, and entitlement policy, tracked in issue #39.

### 3. Private Managed Deployment

Metrum operates a dedicated router deployment for one customer in Metrum cloud, customer cloud, or a managed VPC pattern.

Best for:

- customers who need isolation but do not want to operate the router;
- pilots that may later become self-hosted;
- enterprise buyers who need custom upstream/network configuration.

Commercial shape:

- setup fee;
- monthly managed service fee;
- usage charge or provider pass-through;
- higher support/SLA tier;
- optional private networking, SSO, custom policy, and acceptance validation package.

### 4. Marketplace Procurement

Package Enterprise Self-Hosted and Private Managed Deployment for cloud marketplace procurement after the first direct enterprise sales.

Best for:

- enterprise procurement simplification;
- private offers;
- annual committed spend;
- reducing contract friction.

Do not make marketplace the first dependency for the planned portal path. A constrained portal for evaluation, pilot, renewal, and top-up packages can ship before broader hosted managed-cloud packaging, but only after billing, fulfillment, support, and security review are complete.

### 5. Planned Licensing Portal And Stripe Fulfillment

The portal should be an optional packaging layer over approved license templates, not a new runtime billing system.

Allowed portal motions after #170/#171 ship:

- evaluation or pilot package purchase;
- renewal payment and license re-download;
- volume top-up purchase for approved credit-pack templates;
- Stripe Customer Portal access for invoices and payment method updates where applicable.

Quote-only or account-owner-approved motions:

- enterprise annual licenses;
- private managed deployments;
- marketplace/private-offer equivalents;
- customer-specific add-ons, deployment binding exceptions, revocation accommodations, online lease exceptions, or air-gapped terms.

Internal operating requirements:

- Stripe Products and Prices must map to approved `docs/enterprise-license-skus.json` templates or quote-only SKUs.
- Commercial ops must approve which SKUs are self-service, quote-only, renewal-only, or top-up-only.
- Stripe Checkout, Quotes, Invoices, and Customer Portal should be configured so payment collection happens on Stripe-hosted surfaces; the router and portal must not store card details.
- Stripe Tax or other tax/compliance handling requires finance/legal review before public launch.
- Fulfillment must wait for payment-confirmed webhook events and must be idempotent by Stripe event ID, customer ID, SKU, and target license record.
- License signer custody stays separated from Stripe admin access. Stripe events authorize fulfillment; they do not grant raw signing-key access.
- Webhook replay, failed fulfillment, refund/dispute/cancellation, and accidental bad license issuance need runbooks before launch.

## Suggested Product Tiers

Exact public prices must be revalidated before publishing. The ranges below are internal planning anchors.

### Developer

Target: individual developer or small evaluation.

- Planned portal checkout for approved evaluation or pilot package only.
- One organization.
- One or two projects.
- Limited monthly included usage or credit allowance.
- Short retention.
- Basic usage view.
- Standard shared model groups.
- No custom private upstream.

Possible price anchor: low double-digit to low triple-digit monthly platform fee plus usage.

### Team

Target: growing team using the router for multiple apps.

- Multiple projects.
- Budgets and rate limits.
- Admin reports.
- Longer retention.
- More model groups.
- Alerts.
- Shared team tokens.
- Optional BYOK.

Possible price anchor: several hundred to low four figures monthly plus usage.

### Business

Target: production usage across a department.

- SSO/OIDC/SAML when implemented.
- Audit logs.
- Advanced admin reports.
- Dynamic score routing.
- External policy integration.
- Private upstream support.
- Higher limits and retention.
- Priority support.

Possible price anchor: low thousands monthly plus usage or annual commit.

### Enterprise

Target: company-wide or regulated deployment.

- Self-hosted or private managed.
- Signed license file.
- SSO/RBAC/audit.
- Dedicated support/SLA.
- Marketplace/private offer option.
- Custom model-group validation.
- Private upstream and network controls.
- Contracted retention/security terms.

Price by annual contract, deployment scope, support tier, and usage commitment.

## Billing Models

### Platform Subscription Plus Usage

Recommended default for a future managed service or portal-backed commercial plan.

- Monthly platform fee buys product capability and support.
- Included usage allowance makes starting simple.
- Overage based on provider cost, token/request volume, or credit consumption.

Pros:

- predictable business model;
- easy to map to tiers;
- supports approved checkout, quote, or invoice flows outside the router runtime;
- separates product value from raw model cost.

Cons:

- requires metering and invoice clarity;
- provider-cost pass-through needs careful accounting.

### Credit Wallet

Customer prepays credits, optionally auto-top-up. This is a future managed-service billing concept, not an in-router public API-credit wallet.

Pros:

- familiar for API users;
- reduces payment failure risk;
- easy to cap spend;
- maps well to constrained portal top-up flows when approved.

Cons:

- requires wallet ledger, refunds/adjustments, and tax/accounting review;
- customers may ask how credits map to provider cost.

### Provider Pass-Through Plus Markup

Customer pays provider cost plus a Metrum routing/observability fee.

Pros:

- aligned to usage;
- simple for Metrum-billed provider usage;
- can charge only successful model runs while still recording fallback attempts.

Cons:

- provider pricing changes can confuse customers;
- requires accurate request-time cost storage and billing reconciliation.

### BYOK Platform Fee

Customer supplies provider keys; Metrum charges only platform subscription/usage.

Pros:

- enterprise friendly;
- lowers Metrum provider credit exposure;
- easier for private upstreams.

Cons:

- lower gross margin;
- billing value must be clearly tied to routing, telemetry, governance, support, and validation.

### Annual Enterprise License

Prepaid license for self-hosted/private deployments.

Pros:

- procurement friendly;
- predictable revenue;
- works for on-prem and air-gapped environments;
- can be enforced by signed license file.

Cons:

- slower sales cycle;
- needs renewal process and license operations.

## Entitlements And Feature Gates

Commercial entitlements should map to product capabilities rather than hardcoded model group names.

Entitlement dimensions:

- deployment type: portal-issued evaluation/pilot, private managed, self-hosted;
- enabled features: dynamic_score, external_policy, admin_reports, usage_reporting, audit_log_export, pii_filtering, private_upstreams;
- usage limits: requests, tokens, provider spend, storage/retention;
- operational limits: projects, callers, model groups, admins, environments;
- support tier and SLA.

For self-hosted:

- entitlements live in signed JSON license payload;
- router validates expiry, product, issuer, features, and limits locally;
- license can be replaced without binary rebuild.

For future managed service or portal-backed plans:

- entitlements live in the approved commercial/control-plane system;
- router receives effective limits from config or admin/control-plane sync;
- commercial systems are the source of truth for credits, invoices, and usage charges; the router stores operational usage evidence and enforces the signed license or online lease it receives.

## Launch Enterprise SKUs

The launch SKU mapping is intentionally named by commercial motion and license envelope shape, not by hosted model group names. The machine-readable operator source is `docs/enterprise-license-skus.json`; this section is the human-readable product plan.

| Commercial SKU | License template | Motion | Default term | Volume / window shape | Default capability posture |
|---|---|---|---|---|---|
| `eval-72h` | `eval-72h` | Metrum-managed evaluation | 72 hours | 5M total tokens and 5k total requests | Routing plus usage reporting for a small evaluation footprint |
| `pilot-30d` | `pilot-30d` | Paid pilot | 30 days | 1M tokens / 1 hour and 1k requests / 1 hour | Routing, usage reporting, and dynamic score; add-ons for reports, policy, private upstreams, and PII filtering |
| `enterprise-annual` | `enterprise-annual` | Enterprise self-hosted | 12 months | Unlimited unless the contract adds a ceiling | Default annual BYOK package with reports, routing controls, contracts, rollups, private upstreams, PII filtering, and export capabilities |
| `credit-pack-5m` | `credit-pack-5m` | Volume top-up | 12 months | 5M total tokens and 100k total requests | Routing plus usage reporting; narrow top-up envelope |
| `credit-pack-25m` | `credit-pack-25m` | Volume top-up | 12 months | 25M total tokens and 500k total requests | Top-up envelope with admin reports and usage/baseline exports |
| `marketplace-seat` | `marketplace-seat` | AWS/Azure private offer | Contract term | Contract-defined per-seat or pooled volume | Marketplace equivalent of enterprise/private-managed terms with add-ons encoded in the private offer |

Time-only, volume-only, and time-plus-volume licenses are valid. When a license includes both time and volume limits, whichever limit is reached first blocks further traffic. License replacement with a new `license_id` resets license-wide volume counters; per-caller counters remain separate.

Default launch feature names:

- `routing`
- `usage_reporting`
- `admin_reports`
- `admin_security_reports`
- `dynamic_score`
- `typescript_routing`
- `external_policy`
- `external_policy_http`
- `model_group_contracts`
- `retention_rollups`
- `content_capture`
- `private_upstreams`
- `audit_log_export`
- `pii_filtering`
- `usage_csv_export`
- `usage_baseline_export`

Default launch limit names aligned to issue #159:

- `max_model_groups`
- `max_callers`
- `max_admins`
- `max_monthly_requests`
- `max_total_requests`
- `max_total_tokens`
- `window_requests`
- `window_tokens`
- `window_duration_seconds`
- `max_concurrent`
- `max_retention_days`
- `max_instances`
- `allowed_skins`

The mapping is a commercial and operations contract. Runtime enforcement for fields not currently present in `internal/router/license.go` is owned by issue #159. Public documentation should describe how customers request, mount, renew, and troubleshoot issued licenses; internal docs may describe template generation and support workflow.

## Metering Requirements

Commercial billing should use durable request-time facts.

Meter at minimum:

- request count;
- successful request count;
- failed/fallback attempt count;
- input tokens;
- output tokens;
- total tokens;
- image count and image tokens where applicable;
- calculated input/image/output/total cost in USD;
- upstream-reported billed cost where available;
- selected provider/model/model group;
- caller org/project/environment/token ID;
- cache hit/miss/bypass;
- latency and throughput for service-tier reporting.

Rules:

- Use stored request-time cost values, not current provider pricing, for historical invoices.
- Bill only according to documented commercial policy.
- If charging only successful model runs, still store fallback attempts for operations but exclude failed attempts from billable provider-pass-through unless policy says otherwise.
- Keep billing data relational and queryable; no JSONB/array/serialized structured columns.

## Initial Launch Recommendation

Updated order after shipped license enforcement and issue #172 packaging review:

1. Define tiers, entitlements, and metering contract.
2. Use shipped signed license enforcement for enterprise/on-prem evaluations while license operations mature.
3. Package enterprise self-hosted and private managed deployment operations.
4. Implement production-grade license generation, deployment binding, revocation bundles, and online lease foundations where approved.
5. Build the licensing portal and Stripe webhook fulfillment for approved evaluation, pilot, renewal, and top-up templates.
6. Add public pricing/package docs and marketplace procurement after commercial, legal, tax, support, and security review.

Rationale:

- A constrained portal creates faster evaluation and top-up operations without weakening enterprise procurement or offline license support.
- The same metering foundation supports enterprise invoices and commercial true-up evidence.
- Signed licensing already protects self-hosted deployments; issue #39 should make issuance, renewal, support, and entitlement operations repeatable.
- Marketplace should follow proof of enterprise pull.

## Prioritized GitHub Workstreams

### P0 - Commercial packaging, entitlements, and pricing contract (#36)

Define SKUs, feature gates, usage dimensions, billing semantics, and documentation language. The launch SKU mapping is captured in `docs/enterprise-license-skus.json`; future changes should update that catalog, `docs/LICENSE_OPERATIONS.md`, and public license docs together.

### P0 - Licensing portal and Stripe fulfillment (#170/#171)

Start with approved evaluation, pilot, renewal, and top-up templates. Provision a signed license only after payment-confirmed webhook fulfillment. Do not expose arbitrary entitlement configuration or a public API-credit wallet.

### P0 - Usage metering, billing ledger, credits, and invoice reconciliation (#38)

Convert usage DB events into billable line items, credits, overages, and invoices without losing request-time cost reproducibility.

### P1 - Enterprise self-hosted license operations (#39)

Build on shipped signed license enforcement to support repeatable license issuance, renewal, support workflows, and entitlement policy.

### P1 - Private managed deployments and marketplace procurement (#40)

Define repeatable packaging, runbooks, support boundaries, private offers, and deployment acceptance.

### P1 - Customer billing/admin portal (#41/#170/#171)

Expose invoices, payment method management, license download/re-download, renewal/top-up status, and safe account metadata once portal fulfillment is stable. Router usage reports remain the source for operational usage evidence; product billing remains outside the router runtime.

### P2 - Public pricing and packaging docs (#42)

Publish customer-facing pricing/package pages once commercial policy is stable and legally reviewed.

## Open Decisions

- Whether future managed-service default should be Metrum-billed provider credits, BYOK, or both from day one.
- Whether credits map one-to-one to USD or abstract units.
- Whether successful fallback routing bills only final successful upstream call or includes router attempt overhead.
- Which features are gated by plan at launch versus included in all paid plans.
- Which payment provider to use first.
- Whether self-hosted license enforcement is mandatory in all production packages or only enterprise packages.
- How support/SLA terms map to product tiers.

## Risks

- Billing disputes if provider pass-through, fallback attempts, cache hits, and credits are not clearly defined.
- Margin risk if Metrum-billed provider usage is underpriced or abused.
- Enterprise friction if self-hosted licensing is too restrictive for air-gapped customers.
- Product confusion if hosted deployment group names are treated as product constants.
- Operational risk if portal onboarding creates router/provider resources without quotas and abuse controls.

## Success Metrics

- Time from landing page to first successful API call.
- Trial-to-paid conversion.
- Gross margin by provider/model group.
- Monthly recurring revenue by tier.
- Credit top-up frequency and failed-payment rate.
- Enterprise pilot-to-contract conversion.
- Support tickets per active customer.
- Router usage growth by project and model group.
