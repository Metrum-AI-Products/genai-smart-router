---
title: Buying an Evaluation or Pilot License
doc_type: explanation
---

# Buying an Evaluation or Pilot License

Evaluation and pilot access is contact-led today. Contact [contact@metrum.ai](mailto:contact@metrum.ai) with the deployment model, client APIs, provider preferences, security requirements, and workload evidence you want to validate.

## Current Path

1. Metrum and the customer agree on evaluation scope, deployment path, provider access, reporting needs, and acceptance criteria.
2. Metrum provides either a managed endpoint and caller token or a deployment package with a signed `license.json`.
3. The customer validates `/v1/models`, one or more client workflows, license status, usage reporting, and the agreed quality criteria.
4. Production conversion uses an enterprise self-hosted, private managed, renewal, top-up, or marketplace/private-offer path.

## License Delivery

Evaluation and pilot packages use the same runtime license model as production deployments. Metrum issues either a managed endpoint/caller token or a signed `license.json`; the customer installs the license, restarts or waits for license recheck, and runs the smoke tests in [Licensing](./).

The router enforces the issued license or managed-service entitlement. Payment collection, procurement records, and commercial approvals remain outside the router request path.

## Package Boundaries

Evaluation and pilot licenses can include time, feature, volume, and operational limits. The exact model groups and upstream providers are deployment-defined, so callers should use [`/v1/models`](../getting-started/available-models) with their own token to discover allowed model groups.

If a pilot requires custom private upstreams, air-gapped deployment, legal review, marketplace procurement, dedicated infrastructure, or non-standard entitlements, use the enterprise or private managed path.
