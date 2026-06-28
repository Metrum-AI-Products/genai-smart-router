---
title: Buying an Evaluation or Pilot License
---

# Buying an Evaluation or Pilot License

Evaluation and pilot access is contact-led today. Contact [contact@metrum.ai](mailto:contact@metrum.ai) with the deployment model, client APIs, provider preferences, security requirements, and workload evidence you want to validate.

## Current Path

1. Metrum and the customer agree on evaluation scope, deployment path, provider access, reporting needs, and acceptance criteria.
2. Metrum provides either a managed endpoint and caller token or a deployment package with a signed `license.json`.
3. The customer validates `/v1/models`, one or more client workflows, license status, usage reporting, and the agreed quality criteria.
4. Production conversion uses an enterprise self-hosted, private managed, renewal, top-up, or marketplace/private-offer path.

## Planned Portal Path

A self-service licensing portal is planned but not part of the shipped router behavior until the portal and Stripe fulfillment work is implemented and enabled. When available, it is expected to support only approved constrained packages, not arbitrary feature or entitlement customization.

The planned flow is:

1. Choose an approved evaluation, pilot, or top-up package.
2. Complete Stripe-hosted checkout, quote approval, or invoice payment.
3. Wait for webhook-confirmed fulfillment.
4. Download or re-download the signed license from the portal.
5. Install `license.json`, restart or wait for license recheck, and run the smoke tests in [Licensing](./).

The router does not process card details and does not expose a public API-credit wallet. Stripe-hosted pages handle payment collection where the portal path is used, and the router enforces only the signed license or online lease associated with the commercial plan.

## Package Boundaries

Evaluation and pilot licenses can include time, feature, volume, and operational limits. The exact model groups and upstream providers are deployment-defined, so callers should use [`/v1/models`](../getting-started/available-models) with their own token to discover allowed model groups.

If a pilot requires custom private upstreams, air-gapped deployment, legal review, marketplace procurement, dedicated infrastructure, or non-standard entitlements, use the enterprise or private managed path instead of the planned self-service checkout path.
