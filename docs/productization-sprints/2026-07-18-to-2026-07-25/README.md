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

```text
customer signup + verified email
  -> organization/tenant created in commercial control plane
  -> trial risk check OR Stripe-hosted prepaid Checkout
  -> signed webhook -> immutable ledger credit
  -> provisioning saga
       shared launch path: logical tenant + caller + lease/grant on regional EKS fleet
       dedicated path: namespace/Helm + secrets + license + DNS/TLS + smokes
  -> API token shown once + /v1/models test
  -> inference admission reserves from local signed balance grant
  -> upstream call -> usage persisted -> async ledger settlement
  -> console balance/usage + alerts + top-up
  -> suspend/cancel: revoke future grants first, then governed teardown
```

Stripe, card state, disputes, invoices, and the commercial ledger remain outside
the inference request path. A router request validates a locally available signed
grant and uses its existing admission/reservation controls; it never calls Stripe
or waits on the commercial control plane.

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
| 05 | #523 | [Double-entry prepaid ledger](05-prepaid-ledger-523.md) | launch blocking |
| 06 | #522 | [Balance grants and router admission](06-balance-grants-522.md) | launch blocking |
| 07 | #526 | [EKS tenant provisioning and teardown](07-eks-provisioning-526.md) | launch blocking |
| 08 | #529 | [Abuse, fraud, and provider-cost protection](08-abuse-fraud-protection-529.md) | launch blocking |
| 09 | #524 | [Stripe payment and reconciliation flow](09-stripe-payments-524.md) | launch blocking |
| 10 | #525 | [Prosumer onboarding and console](10-prosumer-console-525.md) | launch blocking |
| 11 | #531 | [SLOs, monitoring, and on-call](11-observability-oncall-531.md) | launch blocking |
| 12 | #533 | [Docs, legal, pricing, and support](12-docs-legal-support-533.md) | launch blocking |
| 13 | #528 | [Experimental-model canaries](13-routing-canaries-528.md) | fast follow |
| 14 | #532 | [Marketplace and hardened artifacts](14-marketplace-artifacts-532.md) | fast follow |

## Viewable UI prototypes

These are static, customer-safe design previews. Open them directly in a browser;
they contain fictional values and no production configuration or credentials.

* [Checkout and provisioning journey](previews/checkout-provisioning.html)
* [Prosumer console](previews/prosumer-console.html)
* [Canary operations console](previews/canary-operations.html)
* [Operations and reconciliation dashboard](previews/operations-dashboard.html)

## Decisions required from people

The launch defaults below are recommendations only until the named owner records
approval. See the full tradeoffs in the root [`DECISIONS.md`](../../../DECISIONS.md).

| Gate | Recommended launch default | Owner | Blocks |
| --- | --- | --- | --- |
| D1 tenancy | Shared regional EKS fleet with logical tenant isolation | Product + Security + SRE | #521, #526 |
| D2 ledger | PostgreSQL double-entry behind a `Ledger` interface | Finance + Architecture | #522, #523 |
| D3 billing | Prepaid credits with opt-in auto-top-up | Product + Finance | #524, #525 |
| D4 pricing | Versioned per-model-group price book and deterministic microcredit rounding | Finance + Product | #522-#525 |
| D5 Marketplace | SaaS/private offer after direct Stripe launch; AMI/container for BYOC | Commercial | #532 |
| D6 region | One disclosed launch region; region field is immutable after provisioning | Legal + SRE | #521, #526 |
| D7 providers | Pooled keys only behind grants/hard caps; BYOK fast-follow | Finance + Security | #522, #526 |
| D8 identity/email | Managed OIDC with verified-email and MFA-ready claims; recommend Cognito on AWS | Security + Product | #521, #525 |
| D9 domain | One regional API hostname for shared tenancy; per-tenant DNS only for dedicated tier | Product + SRE | #526 |
| D10 trial policy | Small one-time credit only after verified identity and risk checks | Finance + Legal + Security | #529 |
| D11 customer error | Use a new documented `balance-exhausted` error with HTTP status approved by API owner | API + Product | #522, #525, #533 |

## Definition of sprint-plan completeness

* Every issue has one ordered versioned plan and a GitHub backlink.
* Every user/admin UI surface has a committed HTML preview.
* Every plan describes unit, integration, E2E, load/fault, security, monitoring,
  rollback, and evidence expectations where applicable.
* Evidence is redacted: no prompts, raw tool/image content, provider keys, router
  tokens/hashes, card data, signing keys, or complete production configuration.
* Issue implementation is not considered done because a plan or mockup exists;
  the issue closes only after its documented tests and proof pass.
