---
title: License-Protected Deployments
doc_type: explanation
---

# License-Protected Deployments

All GenAI Smart Router first-party content is licensed under Apache-2.0. A
signed `license.json` is operator runtime policy only: it is not a copyright
license or commercial-use condition and does not narrow Apache-2.0 rights.

Licensed deployments can enforce a Metrum-issued signed JSON license offline. The deployed router verifies the license with embedded Ed25519 public verification keys; private signing keys and signing-service credentials are not required at runtime and must never be copied into config, logs, reports, browser docs, tickets, images, packages, versioned deployment files, or shared support bundles.

For the customer installation and renewal path, see [Choose a Deployment Path](../licensing/deployment-paths), [How Licensing Works](../licensing/), and [Renewal And Top-Up](../licensing/renewal).

## Runtime Configuration

Normal release builds require license enforcement. Runtime YAML cannot disable licensing in packaged deployments; operators provide the issued `license.json` and durable state path.

```yaml
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    instance_fingerprint: "issued-instance-fingerprint"
    recheck_interval: 1h
    grace_period_on_validation_error: 24h
```

Production licensed deployments should mount the issued license file read-only, keep the license state file on durable private deployment storage, and leave `fail_open_for_dev: false`. Set `instance_fingerprint` only when Metrum issues an instance-bound license for the deployment; it must match the licensed instance scope. The router writes license and quota state with integrity metadata and private file modes; restore trusted backups or use an approved renewal/top-up workflow instead of editing state JSON. Legacy unsigned quota-state import is a one-time migration with `SMART_LLMROUTER_ALLOW_UNSIGNED_STATE_MIGRATION=1`, followed by restart without that flag after signed state is written. The state file preserves renewal, grace, and clock-rollback checks across restarts.

For Docker Compose packages, mount the issued file under the protected compose config directory and keep it readable by the router container user. For binary deployments, place the file in a protected config directory and keep the state file under the deployment state directory. Private signing keys are never installed on the router host.

## Verification Model

The license is a signed JSON envelope issued for GenAI Smart Router. At startup and on each configured recheck interval, the router verifies:

- payload shape and product identity;
- signature validity against an embedded public key;
- `not_before` and expiry windows;
- licensed feature gates and deployment limits;
- local clock rollback state.

When a license blocks serving, `/readyz` fails and caller APIs return documented `license-*` errors without exposing license payloads, signatures, public-key material, private keys, or signing metadata. Safe license status may appear in logs, usage rows, metrics-admin gauges, and authorized admin status APIs as scalar fields such as status, reason, license ID, customer ID, SKU, key ID, expiry, and grace-active flag.

## Runtime Policy Shapes

An operator can issue a signed `license.json` that combines capability, time,
volume, and operational limits. These are deployment policy templates, not
software-license grants or restrictions. Common templates include:

| Template | Typical use | Customer-visible behavior |
|---|---|---|
| `eval-72h` | Short evaluation | Time-limited evaluation with a small volume ceiling. |
| `pilot-30d` | Paid pilot | Time-limited pilot, often with usage reporting and a rolling usage window. |
| `enterprise-annual` | Annual self-hosted contract | Annual license with contracted feature gates, deployment scope, support terms, and operational limits. |
| `credit-pack-5m` | Prepaid top-up | Replacement license that grants a 5M-token license-wide volume budget. |
| `credit-pack-25m` | Larger prepaid top-up | Replacement license that grants a 25M-token license-wide volume budget. |
| `marketplace-seat` | Private marketplace offer | License aligned to the marketplace/private-offer term and contracted seat or volume scope. |

Features are licensed by product capability, such as routing, usage reporting, admin reports, dynamic routing, TypeScript or external policy routing, model-group contracts, retention rollups, private upstreams, PII filtering, audit export, and usage export. Licenses do not require callers to know any fixed product-default model group names; deployed model groups remain operator-defined and discoverable through `/v1/models` for each caller token.

Exact limits and enabled features are encoded in the signed license. The router exposes only safe status fields; it does not expose signing material or commercial back-office details. If a deployment needs a capability that is not enabled by the current license, contact Metrum to update the license rather than editing runtime configuration around the gate.

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

When Metrum provides a signed revocation bundle, mount it beside the license file and set `server.license.revocation.mode: file`. Effective `revoked`, `suspended`, and `superseded` entries block serving immediately and are not bypassed by license grace. Deployments that require a current bundle should set `require_current_bundle: true`; deployments that must fail closed on malformed, expired, untrusted, or rolled-back bundles should keep `fail_closed_on_bundle_error: true`.

Metrum may include a safe summary or operator checklist with a delivered license. Those artifacts are informational and should contain only scalar metadata such as license ID, customer ID, SKU, key ID, issuer, issue time, not-before time, expiry, features, and limits. Install the signed `license.json`; do not install or edit the summary in place of the license.

## Offline And Air-Gapped Operation

License validation is offline. The router does not need to call Metrum during startup or periodic license checks. Air-gapped customers can receive the signed license through their approved secure transfer process, mount it in the deployment, and verify safe status locally.

For support, share request IDs and safe status fields such as license ID, SKU, key ID, expiry, status, reason, and grace-active flag. Do not send provider keys, router tokens, raw prompts, raw images, full config files, private signing keys, or full license payloads through ordinary support channels.

## Online Lease Status

Offline signed-license validation is the default fit for enterprise self-hosted, private-cloud, on-prem, and air-gapped deployments. Evaluation, pilot, renewal, and top-up access use the signed license or managed-service handoff described in [Licensing](/docs/licensing/).

Some customer plans may require periodic signed online lease renewal. That mode is separate from offline enterprise licenses and should be documented in the customer plan when used. Do not assume that every self-hosted or air-gapped deployment requires online checks.

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
| `license-revoked` | Install a replacement license or contact Metrum support if revocation is unexpected. |
| `license-suspended` | Resolve the commercial/support hold or install an updated license and revocation bundle. |
| `license-superseded` | Install the replacement license identified through the approved support channel. |
| `license-revocation-required` | Mount the required signed revocation bundle at `server.license.revocation.path`. |
| `license-revocation-check-failed` | Replace the malformed, expired, untrusted, or rolled-back revocation bundle. |
| `license-clock-rollback` | Correct system time and inspect the durable license state file. |
| config rejects `enabled: false` | Normal release builds cannot be configured to run unlicensed; install a valid license file. |

See [Error Reference](../reference/errors) for caller-visible details.

## Rotation And Revocation

Public verification keys are embedded in the release build. License key rotation is handled by issuing a replacement license and, when required, a release containing the updated verification-key set. License revocation can be enforced offline through a Metrum-issued signed revocation bundle. Operators should keep old and new license files plus revocation bundles under the same secret-handling controls, avoid sharing full payloads in support tickets, and use safe status fields plus request IDs for support diagnostics.

## Smoke Commands

Packaged deployments ship the router runtime and supported operational CLIs. Validate the deployed license through the runtime health and admin surfaces:

```bash
curl -fsS "$ROUTER_BASE_URL/readyz"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/license/status"

curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

License inspection and verification are support operations handled with approved Metrum tooling and public verification keys. Metrum operators use separate internal issuance systems to generate, renew, top up, or revoke licenses from approved entitlement records. Those issuance systems and signing keys are not part of the packaged Docker/runtime image. Deployed routers do not need private signing keys or a license-generation tool at runtime.

## Operator Policy And Commercial Services

A signed `license.json` can encode operator-selected deployment policy: SKU
labels, capability gates, time bounds, volume / window / concurrency limits,
and operational scope. It does not grant or restrict copyright or commercial
use. Payment, invoicing, quotes, refunds, support, and other service terms are
handled outside the router; the router enforces only the configured runtime
policy. Templates include time-bounded evaluation (`eval-72h`, `pilot-30d`),
annual deployment policy (`enterprise-annual`), marketplace/private-offer
policy, and volume presets (`credit-pack-*`).

For license issuance, renewal, volume top-up, or commercial plan changes, contact [contact@metrum.ai](mailto:contact@metrum.ai). See [Commercial Evaluation Path](../evaluation/commercial-evaluation) for evaluation access.
