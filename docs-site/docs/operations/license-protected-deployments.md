---
title: Runtime Policy Licenses
doc_type: howto
---

# Runtime Policy Licenses

The repository is licensed under Apache-2.0. A deployment's `license.json` is a
separate operator-controlled runtime-policy input; it is not a copyright
license, purchase requirement, or restriction on Apache-2.0 rights.

## Create And Install

Generate an Ed25519 keypair on a protected operator host. Keep the private key
outside source control, runtime containers, Kubernetes, logs, tickets, and
backups that lack signing-key controls. Issue `license.json` locally and mount
only the license plus verification public key into the router.

The Kubernetes wrapper `scripts/helm_install_with_license.sh` automates local
key generation, issuance, Secret refresh, and Helm install/upgrade. For other
deployment shapes, use the packaged license CLI and configure
`server.license.path`, `state_path`, and `public_keys` according to the shipped
sample config.

## Renew, Rotate, And Recover

- Renewal: issue a replacement with the same protected keypair and atomically
  replace `license.json`.
- Trust rotation: generate a new keypair and deploy the new license and public
  key together.
- Lost private key: treat it as a trust rotation; it cannot be recovered from
  the router.
- Invalid update: restore the prior valid license and public key as a pair.

Never edit a signed payload. Verify `/readyz`, safe license status, and one
authenticated caller request after every change. Keep the prior valid pair for
rollback and protect the durable license-state file from concurrent writers.

## Failure Boundaries

Invalid, expired, untrusted, or policy-blocked licenses fail readiness and
return documented `license-*` errors. Diagnostics may expose safe scalar status
but must not expose signatures, private keys, provider keys, caller secrets, or
the full runtime configuration. See [License Configuration](../configuration/license)
and [Licensing](../licensing/).

