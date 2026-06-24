# GenAI Smart Router Commercialization Plan

This is an internal product and commercialization plan for GenAI Smart Router. It is not customer-facing pricing copy. Revalidate competitor pricing and procurement details before publishing any public comparison or exact price claim.

Research snapshot: 2026-06-24.

Primary reference points reviewed:

- LiteLLM Enterprise: self-hosted/cloud enterprise license positioning, license-key unlock, deployment-size pricing, AWS/Azure Marketplace procurement.
  Source: https://docs.litellm.ai/docs/enterprise
- Helicone pricing: free/pro/team self-serve style, monthly tiers plus usage-based pricing.
  Source: https://www.helicone.ai/pricing
- Portkey positioning: AI gateway, observability, guardrails, governance, prompt and model-catalog platform.
  Source: https://portkey.ai/
- OpenRouter pricing: prepaid credits, manual/automatic top-up, enterprise by volume/prepayment/annual commit, and routing/fallback billing around successful model runs.
  Source: https://openrouter.ai/pricing

## Goals

- Let small teams start by credit card without a sales call.
- Preserve high-ACV enterprise paths for self-hosted, private-cloud, and Metrum-managed deployments.
- Monetize the router value separately from upstream provider cost.
- Keep packaging compatible with enterprise procurement: annual contracts, private offers, AWS/Azure Marketplace, and signed self-hosted licenses.
- Make commercial enforcement practical without blocking legitimate on-prem and air-gapped customers.
- Keep public docs generic and deployment-defined; avoid exposing private hostnames, provider keys, real router tokens, private pricing commitments, or production-only model group details.

## Recommended Commercial Motions

### 1. Metrum Managed Cloud

Primary self-serve motion.

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

Related implementation issue: signed license enforcement is tracked separately in GitHub issue #35.

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

Do not make marketplace the first dependency for self-serve. Credit-card managed cloud should ship faster.

## Suggested Product Tiers

Exact public prices must be revalidated before publishing. The ranges below are internal planning anchors.

### Developer

Target: individual developer or small evaluation.

- Credit card checkout.
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

Recommended default for managed cloud.

- Monthly platform fee buys product capability and support.
- Included usage allowance makes starting simple.
- Overage based on provider cost, token/request volume, or credit consumption.

Pros:

- predictable business model;
- easy to map to tiers;
- supports credit-card checkout;
- separates product value from raw model cost.

Cons:

- requires metering and invoice clarity;
- provider-cost pass-through needs careful accounting.

### Credit Wallet

Customer prepays credits, optionally auto-top-up.

Pros:

- familiar for API users;
- reduces payment failure risk;
- easy to cap spend;
- maps well to self-serve.

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

- deployment type: managed cloud, private managed, self-hosted;
- enabled features: dynamic_score, external_policy, admin_reports, advanced_telemetry, SSO, audit, private_upstreams;
- usage limits: requests, tokens, provider spend, storage/retention;
- operational limits: projects, callers, model groups, admins, environments;
- support tier and SLA.

For self-hosted:

- entitlements live in signed JSON license payload;
- router validates expiry, product, issuer, features, and limits locally;
- license can be replaced without binary rebuild.

For managed cloud:

- entitlements live in billing/control-plane DB;
- router receives effective limits from config or admin/control-plane sync;
- billing ledger is source of truth for credits, invoices, and usage charges.

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

Order:

1. Define tiers, entitlements, and metering contract.
2. Build managed-cloud self-serve checkout with one simple paid tier and credits.
3. Implement billing ledger and invoice reconciliation from usage DB.
4. Ship signed self-hosted license enforcement.
5. Package enterprise/private managed deployment operations.
6. Add public pricing/docs and marketplace procurement.

Rationale:

- Self-serve managed cloud creates fast adoption and pricing feedback.
- The same metering foundation supports enterprise invoices.
- Signed licensing protects self-hosted later without delaying cloud launch.
- Marketplace should follow proof of enterprise pull.

## Prioritized GitHub Workstreams

### P0 - Commercial packaging, entitlements, and pricing contract

Define SKUs, feature gates, usage dimensions, billing semantics, and documentation language. This unblocks all other commercial work.

### P0 - Managed cloud self-serve checkout and account provisioning

Let a user create an organization, pay by credit card, receive a router endpoint/token, and start calling the API.

### P0 - Usage metering, billing ledger, credits, and invoice reconciliation

Convert usage DB events into billable line items, credits, overages, and invoices without losing request-time cost reproducibility.

### P1 - Enterprise self-hosted license operations

Build on signed license enforcement to support license issuance, renewal, install docs, support workflows, and entitlement checks.

### P1 - Private managed deployments and marketplace procurement

Define repeatable packaging, runbooks, support boundaries, private offers, and deployment acceptance.

### P1 - Customer billing/admin portal

Expose invoices, credits, usage, limits, token/project management, and plan controls to customers.

### P2 - Public pricing and packaging docs

Publish customer-facing pricing/package pages once commercial policy is stable and legally reviewed.

## Open Decisions

- Whether managed cloud default should be Metrum-billed provider credits, BYOK, or both from day one.
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
- Operational risk if self-serve onboarding creates router/provider resources without quotas and abuse controls.

## Success Metrics

- Time from landing page to first successful API call.
- Trial-to-paid conversion.
- Gross margin by provider/model group.
- Monthly recurring revenue by tier.
- Credit top-up frequency and failed-payment rate.
- Enterprise pilot-to-contract conversion.
- Support tickets per active customer.
- Router usage growth by project and model group.
