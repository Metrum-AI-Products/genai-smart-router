---
title: License-Protected Deployments
---

# License-Protected Deployments

Licensed deployments can enforce a Metrum-issued signed JSON license offline. The deployed router verifies the license with embedded Ed25519 public verification keys; private signing keys and signing-service credentials are not required at runtime and must never be copied into config, logs, reports, browser docs, tickets, images, or source control.

## Runtime Configuration

```yaml
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    recheck_interval: 1h
    grace_period_on_validation_error: 24h
    fail_open_for_dev: false
```

Production licensed deployments should mount the issued license file read-only, keep the license state file on durable deployment storage, and leave `fail_open_for_dev: false`. The state file preserves renewal, grace, and clock-rollback checks across restarts.

For Docker Compose packages, mount the issued file under the protected compose config directory and keep it readable by the router container user. For binary deployments, place the file in a protected config directory and keep the state file under the deployment state directory. Private signing keys are never installed on the router host.

## Verification Model

The license is a signed JSON envelope issued for GenAI Smart Router. At startup and on each configured recheck interval, the router verifies:

- payload shape and product identity;
- signature validity against an embedded public key;
- `not_before` and expiry windows;
- licensed feature gates and deployment limits;
- local clock rollback state.

When a license blocks serving, `/readyz` fails and caller APIs return documented `license-*` errors without exposing license payloads, signatures, public-key material, private keys, or signing metadata. Safe license status may appear in logs, usage rows, metrics-admin gauges, and authorized admin status APIs as scalar fields such as status, reason, license ID, customer ID, SKU, key ID, expiry, and grace-active flag.

## Commercial License Shapes

Metrum issues a signed `license.json` for the commercial agreement. A license can combine capability, time, volume, and operational limits. Common templates include:

| Template | Typical use | Customer-visible behavior |
|---|---|---|
| `eval-72h` | Short evaluation | Time-limited evaluation with a small volume ceiling. |
| `pilot-30d` | Paid pilot | Time-limited pilot, often with usage reporting and a rolling usage window. |
| `enterprise-annual` | Annual self-hosted contract | Annual license with contracted feature gates, deployment scope, support terms, and operational limits. |
| `credit-pack-5m` | Prepaid top-up | Replacement license that grants a 5M-token license-wide volume budget. |
| `credit-pack-25m` | Larger prepaid top-up | Replacement license that grants a 25M-token license-wide volume budget. |
| `marketplace-seat` | Private marketplace offer | License aligned to the marketplace/private-offer term and contracted seat or volume scope. |

Exact limits and enabled features are encoded in the signed license. The router exposes only safe status fields; it does not expose signing material or commercial back-office details.

## Expiry, Grace, Renewal, Replacement, And Top-Up

Renewal is a file replacement workflow:

1. Obtain a current Metrum-issued license through the commercial support path.
2. Back up the previous runtime license file according to the deployment's secret-handling policy.
3. Replace `server.license.path` atomically where possible.
4. Restart the router or wait for `recheck_interval`.
5. Verify `/readyz`, `/admin/license/status` for an authorized admin subject, metrics-admin license gauges, and one caller smoke for a licensed feature.

`grace_period_on_validation_error` is for transient validation problems after a previously valid license was observed. It is not a substitute for renewal before expiry, and it does not permit deployment limits or unlicensed features indefinitely.

Replacement uses the same installation flow when Metrum issues a corrected license, changes a contracted feature, updates instance scope, rotates signing keys, or recovers a lost license file. Do not edit `license.json`; any payload change invalidates the signature.

Volume top-up also uses the same file replacement flow. For `credit-pack-*` licenses, Metrum issues a replacement license with a new license ID and the new contracted token budget. The router treats the new license as a new license-wide budget while caller-token quotas remain controlled by the deployment's caller-key policy.

## Offline And Air-Gapped Operation

License validation is offline. The router does not need to call Metrum during startup or periodic license checks. Air-gapped customers can receive the signed license through their approved secure transfer process, mount it in the deployment, and verify safe status locally.

For support, share request IDs and safe status fields such as license ID, SKU, key ID, expiry, status, reason, and grace-active flag. Do not send provider keys, router tokens, raw prompts, raw images, full config files, private signing keys, or full license payloads through ordinary support channels.

## Admin Visibility

Authorized report/admin users can inspect safe license status through the admin status surface when enabled. Metrics-admin users can scrape safe license gauges. Usage rows can separate license-denied requests from caller auth, quota, routing, and upstream failures with license status fields.

Ordinary application caller tokens should not receive license payloads or operational details. They receive a structured `license-*` error plus request ID when enforcement blocks a request.

## Failure Modes

| Error | Typical operator action |
|---|---|
| `license-missing` | Mount the issued license file at `server.license.path` and check file permissions. |
| `license-invalid` | Replace the malformed or unverifiable license with a valid issued file. |
| `license-expired` | Renew or restore a current license, then restart or wait for recheck. |
| `license-not-yet-valid` | Check license dates and system clock. |
| `license-product-mismatch` | Install a license issued for GenAI Smart Router. |
| `license-feature-forbidden` | Confirm the feature is in the commercial plan or disable that feature. |
| `license-limit-exceeded` | Reduce configured usage or update the licensed limits. |
| `license-clock-rollback` | Correct system time and inspect the durable license state file. |

See [Error Reference](../reference/errors) for caller-visible details.

## Rotation And Revocation

Public verification keys are embedded in the release build. License key rotation or revocation is handled by issuing a replacement license and, when required, a release containing the updated verification-key set. Operators should keep old and new license files under the same secret-handling controls, avoid sharing full payloads in support tickets, and use safe status fields plus request IDs for support diagnostics.

## Smoke Commands

Packaged deployments ship the router runtime and supported operational CLIs. Validate the deployed license through the runtime health and admin surfaces:

```bash
curl -fsS "$ROUTER_BASE_URL/readyz"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/license/status"

curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

The source-tree `cmd/router-license` helper can inspect or verify a license file during release engineering or support validation when run from a checked-out source tree with an approved public key file. It is not part of the packaged Docker/runtime image unless a deployment explicitly adds it. Deployed routers do not need private signing keys or the license helper binary at runtime.

## Commercial Model

A signed `license.json` encodes the commercial entitlements for a deployment: SKU, capability gates, time bounds, volume / window / concurrency limits, and operational scope. Payment, invoicing, and contract terms are handled by Metrum commercial contact outside the router; the router only enforces what the license declares. License templates include time-bounded evaluation (`eval-72h`, `pilot-30d`), annual enterprise license (`enterprise-annual`), and volume prepurchase (`credit-pack-*`) for top-up.

For license issuance, renewal, volume top-up, or commercial plan changes, contact [contact@metrum.ai](mailto:contact@metrum.ai). See [Commercial Evaluation Path](../evaluation/commercial-evaluation) for evaluation access.
