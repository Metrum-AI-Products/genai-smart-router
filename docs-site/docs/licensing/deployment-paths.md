---
title: Choose a Deployment Path
doc_type: explanation
---

# Choose a Deployment Path

GenAI Smart Router supports enterprise-first commercial access with signed licenses and deployment-specific routing policy. Evaluation and production access start with the deployment path, because the right license, provider-key model, reporting package, and support plan depend on where the router runs. For topology choices such as central, per-environment, per-team, hierarchical, federated, or private managed routers, see [Enterprise Deployment Patterns](../operations/deployment-patterns).

For how customers own model-group membership, routing strategy, provider keys, validation gates, and rollback evidence inside a deployment, see [Customer-Controlled Routing](../routing/customer-controlled-routing).

<div class="contactBanner">
  <p>To discuss access, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Commercial Paths

| Path | Best fit | Licensing and procurement |
|---|---|---|
| Enterprise self-hosted | Customers operating the router in their own infrastructure, including regulated, private-cloud, on-prem, or air-gapped environments. | Metrum-issued signed `license.json`, customer-controlled provider keys or private upstreams, and annual or contracted procurement. |
| Private managed deployment | Customers who want a dedicated router instance operated for one customer without running the service themselves. | Dedicated customer deployment with direct order, quote/invoice, or private marketplace offer when available. Operational details are handled through the managed-service plan. |
| Evaluation or pilot | Teams proving workload quality, provider mix, cost controls, security posture, and reporting before production. | Contact-led evaluation with either a managed endpoint or a signed evaluation/pilot license. |
| Renewal or top-up | Existing customers extending term, replacing an issued license, or adding a prepaid volume envelope. | Replacement `license.json` delivered through the approved commercial/support path. |
| Marketplace or private offer | Enterprise procurement teams that prefer cloud marketplace contracting. | Private offer or marketplace-aligned signed license when commercially available; not a public shared multitenant API-credit product. |

Enterprise self-hosted and private managed deployments use direct commercial engagement, quote/invoice, or private offer. Evaluation, renewal, and top-up packages are issued from approved commercial terms rather than ad hoc runtime entitlement changes.

## Enterprise Self-Hosted

In self-hosted deployments, the customer runs the router package and installs a signed `license.json`. The router verifies the license locally with embedded public verification keys. This path supports BYOK provider keys, private upstreams, customer network controls, and offline or air-gapped operation when the commercial plan allows it.

Runtime enforcement can include feature gates, time bounds, volume limits, deployment scope, and operational limits. Optional future enforcement modes can add deployment binding, revocation bundles, or online leases for plans that require them, while preserving offline signed-license operation for contracted air-gapped buyers.

## Private Managed Deployment

In a private managed deployment, Metrum operates a dedicated router deployment for one customer or one customer-specific high-availability environment. It is not described as a shared public multitenant API service. The customer receives deployment-specific endpoint and access details through the managed-service handoff, while public docs use generic hostnames and token placeholders.

Procurement can be direct order, quote or invoice, or marketplace private offer when available. Provider-cost handling, BYOK scope, reporting, network isolation, and acceptance tests are defined in the customer plan.

## Evaluation, Pilot, Renewal, And Top-Up

Current evaluation access is contact-led so the customer and Metrum can agree on deployment shape, provider access, security constraints, model-group quality criteria, and reporting evidence. See [Commercial Evaluation Path](../evaluation/commercial-evaluation).

Evaluation, pilot, renewal, and top-up packages follow the same runtime model: Metrum issues a signed license or managed-service entitlement, the router enforces it, and commercial payment or procurement records remain outside the router request path.

For renewal and top-up installation, see [Renewal And Top-Up](./renewal).
