# Prosumer Launch Sprint: 2026-07-18 to 2026-07-25

Plan version: **v1.0.0**  
Last updated: **2026-07-18**  
Milestone: [`productization-sprint-1`](https://github.com/sysadmin-metrum-ai/genai-smart-router/milestone/1)  
Epic: [#520](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/520)

This folder is the implementation-design source of truth for the first Prosumer
Launch sprint. The dates are a planning window, not a promise that the entire
launch backlog can be implemented in seven days. Each numbered document is an
agent-ready work contract: it identifies the design, human gates, configuration,
end-to-end flow, tests, evidence, observability, rollback, and completion rule.

## Product flow this sprint is designing

Commercial model (see `DECISIONS.md` D3): **base monthly subscription** (Stripe)
+ **included usage allowance** sized by the plan's model mix (input + output
tokens and any plan-defined billable units) + **x402 per-request payment** for
usage beyond the included allowance.

```text
customer signup + verified email
  -> organization/tenant created in commercial control plane
  -> trial risk check OR Stripe-hosted subscription Checkout
  -> signed webhook -> active subscription entitlement
  -> ledger: recognize subscription + grant included-allowance for the period
  -> provisioning saga
       shared launch path: logical tenant + caller + lease/grant on regional EKS fleet
       dedicated path: namespace/Helm + secrets + license + DNS/TLS + smokes
  -> API token shown once + /v1/models test
  -> inference admission:
       1. validate subscription entitlement (local, cached)
       2. reserve from included-allowance grant (worst-case quote)
       3. if allowance cannot cover the quote -> x402 challenge (HTTP 402)
       4. on valid x402 payment -> verify + settle locally, then proceed
  -> upstream call -> usage persisted
  -> async ledger settlement (allowance consume / x402 settlement record)
  -> console: subscription state, included-allowance usage, x402 spend, alerts
  -> suspend/cancel: revoke future grants/leases first, then governed teardown
```

Stripe, card state, disputes, invoices, and the commercial ledger remain outside
the inference request path. A router request validates a locally available
subscription entitlement and included-allowance grant, or a valid x402
settlement; it never calls Stripe or waits on the commercial control plane.

## Ordered implementation documents

The filename order is the recommended start/closure order. Security and
observability begin early and close after the surfaces they assess exist.

| Order | Issue | Plan | Close class |
| ---: | ---: | --- | --- |
| 00 | all | [Shared account, key, secret, and configuration registry](00-shared-account-config-inventory.md) | prerequisite |
| 01 | #520 | [Epic delivery contract](01-epic-prosumer-launch-520.md) | launch gate |
| 02 | #527 | [Migration-safe EKS delivery foundation](02-release-engineering-527.md) | launch blocking |
| 03 | #530 | [Threat model and compliance baseline](03-security-compliance-530.md) | launch blocking |
| 04 | #521 | [Commercial control plane and tenant lifecycle](04-control-plane-lifecycle-521.md) | launch blocking |
| 05 | #540 | [Plan catalog: SKUs, allowance, x402 bands](15-plan-catalog-540.md) | launch blocking |
| 06 | #523 | [Double-entry prepaid ledger](05-prepaid-ledger-523.md) | launch blocking |
| 07 | #522 | [Balance grants and router admission](06-balance-grants-522.md) | launch blocking |
| 08 | #506 | [x402 overage payment and settlement](16-x402-overage-506.md) | launch blocking |
| 09 | #526 | [EKS tenant provisioning and teardown](07-eks-provisioning-526.md) | launch blocking |
| 10 | #529 | [Abuse, fraud, and provider-cost protection](08-abuse-fraud-protection-529.md) | launch blocking |
| 11 | #524 | [Stripe subscription and reconciliation](09-stripe-payments-524.md) | launch blocking |
| 12 | #525 | [Prosumer onboarding and console](10-prosumer-console-525.md) | launch blocking |
| 13 | #531 | [SLOs, monitoring, and on-call](11-observability-oncall-531.md) | launch blocking |
| 14 | #533 | [Docs, legal, pricing, and support](12-docs-legal-support-533.md) | launch blocking |
| 15 | #534 | [Integrated beta: sub + allowance + x402](17-integrated-beta-534.md) | launch gate |
| 16 | #528 | [Experimental-model canaries](13-routing-canaries-528.md) | fast follow |
| 17 | #532 | [Marketplace and hardened artifacts](14-marketplace-artifacts-532.md) | fast follow |
| 18 | #541 | [Outcome-based auto-tune](18-auto-tune-541.md) | fast follow |

## Viewable UI prototypes

**Canonical UX catalog (narratives + Metrum bento mocks):** [`ux/INDEX.md`](ux/INDEX.md)

Master E2E subscription journey: [`ux/e2e-subscription.md`](ux/e2e-subscription.md) · personas: [`ux/personas.md`](ux/personas.md)

Rewritten previews (subscription + included allowance + x402; bento/Metrum):

* Checkout and provisioning: [HTML](ux/previews/checkout-provisioning.html) (also mirrored under [previews/](previews/checkout-provisioning.html))
* Prosumer console: [HTML](ux/previews/prosumer-console.html)
* Canary operations: [HTML](ux/previews/canary-operations.html)
* Operations dashboard: [HTML](ux/previews/operations-dashboard.html)

Per-issue narrative + HTML: [`ux/issues/`](ux/issues/) · Shipped surfaces: [`ux/shipped/`](ux/shipped/)

Legacy PNG thumbnails under `previews/*.png` may still show prepaid-era art; prefer the HTML mocks above.

## Narrated-video approval packages

* Sprint dubday package: [`video/dubday-approval-package.md`](video/dubday-approval-package.md)
* **Mock-interaction clips (V1–V7, ~1 min each):** [`ux/video/README.md`](ux/video/README.md) — awaiting explicit narration approval before TTS/render.

No audio or video render may begin until the relevant narration is explicitly approved.

## Decisions required from people

The launch defaults below are recommendations only until the named owner records
approval. See the full tradeoffs in the root [`DECISIONS.md`](../../../DECISIONS.md).

| Gate | Launch default | Owner | Blocks |
| --- | --- | --- | --- |
| D1 tenancy | Shared regional EKS fleet with logical tenant isolation | Product + Security + SRE | #521, #526 |
| D2 ledger | PostgreSQL double-entry behind a `Ledger` interface | Finance + Architecture | #522, #523 |
| D3 billing | **DECIDED:** base monthly Stripe subscription + model-mix included allowance + x402 overage | Product + Finance | #506, #522–#525, #534 |
| D4 pricing | Versioned plan SKUs (Stripe Price + included tokens by mix + x402 bands) | Finance + Product | #506, #522–#525, #534 |
| D5 Marketplace | SaaS/private offer after Stripe+x402 launch; AMI/container for BYOC | Commercial | #532 |
| D6 region | One disclosed launch region; region field is immutable after provisioning | Legal + SRE | #521, #526 |
| D7 providers | Pooled keys only behind entitlement + allowance grants + x402; BYOK fast-follow | Finance + Security | #522, #526 |
| D8 identity/email | Managed OIDC with verified-email and MFA-ready claims; recommend Cognito on AWS | Security + Product | #521, #525 |
| D9 domain | One regional API hostname for shared tenancy; per-tenant DNS only for dedicated tier | Product + SRE | #526 |
| D10 trial policy | Time-boxed trial **subscription entitlement** (not prepaid cash) after verified identity/risk | Finance + Legal + Security | #529 |
| D11 customer error | Distinct `entitlement-*` / `payment-required` / `payment-invalid` / `payment-unavailable` errors | API + Product | #506, #522, #525, #533, #534 |
| D12 x402 prod | Protocol+staging launch-blocking; live settlement gated on treasury/compliance | Security + Finance + Legal | #506, #534 |

## Definition of sprint-plan completeness

* Every issue has one ordered versioned plan and a GitHub backlink.
* Every user/admin UI surface has a committed HTML preview.
* Every plan describes unit, integration, E2E, load/fault, security, monitoring,
  rollback, and evidence expectations where applicable.
* Evidence is redacted: no prompts, raw tool/image content, provider keys, router
  tokens/hashes, card data, signing keys, or complete production configuration.
* Issue implementation is not considered done because a plan or mockup exists;
  the issue closes only after its documented tests and proof pass.
