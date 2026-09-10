---
title: Deployment Paths
doc_type: howto
---

# Deployment Paths

Metrum Router is self-managed software. Operators run it in their own cloud,
on-premises, or air-gapped infrastructure. A signed runtime license is an
optional operator policy, not a required key for OSS use.

## Self-Hosted Path (no license)

1. Render a blueprint for the selected topology and upstreams.
2. Keep provider credentials in protected local files.
3. Deploy with `server.license.enabled: false` (the default).
4. Validate `/readyz`, allowed models, an authorized chat request, streaming, and rejected unauthorized/disallowed requests.

## Self-Hosted Path (optional signed license)

1. Render a blueprint for the selected topology and upstreams.
2. Keep provider credentials in protected local files.
3. Run `scripts/helm_install_with_license.sh`; it creates a local signing keypair on first use, issues `license.json`, refreshes the runtime Secret, and installs Helm with `server.license.enabled: true`.
4. Validate `/readyz`, allowed models, an authorized chat request, streaming, and rejected unauthorized/disallowed requests.

When enforcement is enabled, the router verifies the operator-generated license with the paired public key mounted at `/app/config/license.pub`. No commercial service, hosted issuer, marketplace, or Metrum contact is required.

## Renewal and Key Rotation

Reissue a license from the protected local signing key for normal renewal. To rotate trust, generate a new keypair and deploy the new `license.json` and `license.pub` together. See [Self-Managed Licensing](./).
