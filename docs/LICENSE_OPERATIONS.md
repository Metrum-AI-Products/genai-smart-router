# License Operations Runbook

This runbook is for Metrum operators who issue, renew, replace, top up, and support GenAI Smart Router enterprise licenses. It is intentionally internal: customer-facing Docusaurus docs explain how to install and renew a license, but they do not describe signing-key custody, signing service internals, or development bypass mechanics.

Related work:

- #36 defines commercial SKU templates and the SKU-to-entitlement map.
- #159 defines the license envelope shape for capability, time, volume, and operational limits.
- #158 makes license enforcement mandatory for normal builds and leaves any no-license mode as an explicit internal development build.

The machine-readable source for launch SKU templates is `docs/enterprise-license-skus.json`. Keep that artifact, this runbook, `COMMERCIALIZATION.md`, and customer-facing Docusaurus license docs aligned when packaging changes.

## Operating Rules

- Never paste private signing keys, signing-service credentials, real customer license files, customer identifiers, router tokens, token hashes, provider API keys, or full production config into tickets, docs, logs, shell history, or chat.
- Store private signing keys only in the approved signing system or secret manager. The router runtime needs public verification keys only.
- Deliver `license.json` through an approved secure channel. Do not attach real licenses to GitHub issues, pull requests, public docs, package docs, or sample configs.
- Use safe scalar license metadata for support: `license_id`, customer alias, SKU, `key_id`, issuer, status, reason, expiry, grace flag, and request IDs.
- Treat license issuance, renewal, replacement, and top-up as commercial events. Record them in the commercial/support system, not in this repository.
- When investigating an installed customer deployment, ask for safe status output and request IDs. Do not ask for full payloads unless legal/support policy explicitly authorizes a secure file transfer.

## Roles And Records

| Role | Responsibility |
|---|---|
| Sales or account owner | Confirms customer, SKU, term, volume, add-ons, support tier, and allowed deployment mode. |
| Finance or commercial ops | Confirms invoice, purchase order, private offer, top-up order, or evaluation approval. |
| License issuer | Generates the signed license from the approved entitlement record. |
| Support engineer | Guides customer installation, renewal, replacement, and diagnostics using safe status fields. |
| Release engineer | Confirms shipped binaries contain public verification keys and normal builds enforce licensing. |
| Security reviewer | Reviews signing-key custody, secure delivery, and incident handling. |

Maintain an external entitlement record with at least:

- customer legal name and internal customer ID;
- deployment alias and environment;
- commercial SKU and license template;
- approved features, limits, term, support tier, and add-ons;
- signer identity, signing key ID, license ID, issue time, expiry, and delivery channel;
- replacement reason, if replacing an earlier license;
- acceptance evidence and support ticket links.

## SKU Templates

These templates align with #36 and #159. They are examples for constructing the unsigned payload that is then signed by the approved license signer. Use real customer IDs, license IDs, dates, limits, and add-ons from the entitlement record.

The `product` field is required and must be `genai-smart-router`. Payload examples that include #159 fields such as `max_total_tokens`, `window_tokens`, `max_concurrent`, `max_admins`, `max_retention_days`, `max_instances`, or `allowed_instances` require a #159-capable `router-license` binary and router runtime. Do not sign those fields with older tooling: pre-#159 binaries ignore unknown JSON fields during payload decoding.

Launch SKU summary:

| SKU | Template | Commercial motion | Default term | Default volume / window | Primary use |
|---|---|---|---|---|---|
| `eval-72h` | `eval-72h` | Metrum-managed evaluation | 72 hours | 5M total tokens, 5k total requests | Free or partner proof window |
| `pilot-30d` | `pilot-30d` | Paid pilot | 30 days | 1M tokens / 1 hour, 1k requests / 1 hour | Time-boxed customer validation |
| `enterprise-annual` | `enterprise-annual` | Enterprise self-hosted | 12 months | Unlimited unless the contract adds a ceiling | Default BYOK annual contract |
| `credit-pack-5m` | `credit-pack-5m` | Volume top-up | 12 months | 5M total tokens, 100k total requests | Small prepaid or top-up envelope |
| `credit-pack-25m` | `credit-pack-25m` | Volume top-up | 12 months | 25M total tokens, 500k total requests | Larger prepaid or top-up envelope |
| `marketplace-seat` | `marketplace-seat` | AWS/Azure private offer | Contract term | Contract-defined per-seat or pooled volume | Marketplace procurement |

Default features should be encoded exactly as stable license feature names. Do not encode deployment-defined model group names in license features, SKU names, or public docs.

### `eval-72h`

Purpose: short free or partner evaluation.

Default entitlement shape:

- term: 72 hours from approved start;
- volume: `max_total_tokens: 5000000`;
- suggested request cap: `max_total_requests: 5000`;
- features: `routing`, optionally `usage_reporting`;
- operational limits: small caller and model-group count.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_eval_example_001",
  "customer_id": "cust_example_eval",
  "product": "genai-smart-router",
  "sku": "eval-72h",
  "features": ["routing", "usage_reporting"],
  "limits": {
    "max_total_tokens": 5000000,
    "max_total_requests": 5000,
    "max_callers": 3,
    "max_model_groups": 5
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["eval"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2026-06-30T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- `/readyz` passes with the license installed;
- `/v1/models` works for an evaluation caller token;
- one licensed chat request succeeds;
- safe status shows `sku=eval-72h` and the expected expiry.

### `pilot-30d`

Purpose: paid time-boxed pilot with richer reporting and routing validation.

Default entitlement shape:

- term: 30 days;
- volume: rolling window such as `window_tokens: 1000000` with `window_duration_seconds: 3600`;
- features: `routing`, `usage_reporting`, `dynamic_score`; add `private_upstreams` if the pilot validates customer-hosted models;
- operational limits: moderate caller and model-group count, retention appropriate for pilot evidence.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_pilot_example_001",
  "customer_id": "cust_example_pilot",
  "product": "genai-smart-router",
  "sku": "pilot-30d",
  "features": ["routing", "usage_reporting", "dynamic_score", "private_upstreams"],
  "limits": {
    "window_tokens": 1000000,
    "window_requests": 10000,
    "window_duration_seconds": 3600,
    "max_callers": 10,
    "max_model_groups": 10,
    "max_retention_days": 30
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["pilot"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2026-07-27T00:00:00Z",
  "grace_until": "2026-07-30T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- readiness and a caller smoke pass;
- usage report access works only if `usage_reporting` is present;
- dynamic-score routing config starts only if `dynamic_score` is present;
- window limits are documented for the customer success plan.

### `enterprise-annual`

Purpose: default self-hosted annual enterprise contract.

Default entitlement shape:

- term: 12 months;
- volume: unlimited unless the contract includes a ceiling;
- features: `routing`, `usage_reporting`, `admin_reports`, `dynamic_score`, `typescript_routing`, `external_policy`, `model_group_contracts`, `retention_rollups`, `private_upstreams`, `pii_filtering`, `usage_csv_export`, and `usage_baseline_export`; add optional security reports, content capture, audit export, or external HTTP policy when contracted;
- operational limits: caller, admin, model-group, retention, instance, and concurrency limits sized by contract.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_annual_example_001",
  "customer_id": "cust_example_enterprise",
  "product": "genai-smart-router",
  "sku": "enterprise-annual",
  "features": [
    "routing",
    "usage_reporting",
    "admin_reports",
    "admin_security_reports",
    "dynamic_score",
    "typescript_routing",
    "external_policy",
    "external_policy_http",
    "model_group_contracts",
    "retention_rollups",
    "private_upstreams",
    "pii_filtering",
    "usage_csv_export",
    "usage_baseline_export"
  ],
  "limits": {
    "max_callers": 100,
    "max_model_groups": 50,
    "max_admins": 10,
    "max_retention_days": 90,
    "max_instances": 1,
    "max_concurrent": 200
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["prod"],
    "instance_fingerprint_required": true,
    "allowed_instances": ["fp:example-instance-fingerprint"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "grace_until": "2027-07-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- readiness, model discovery, and one caller smoke pass;
- enabled admin/reporting features pass with authorized subjects and fail for unauthorized subjects;
- private upstream routing works only when the feature is present;
- safe status matches customer, SKU, key ID, expiry, and instance scope.

### `credit-pack-5m`

Purpose: volume top-up or prepaid volume license.

Default entitlement shape:

- term: 12 months unless contract says otherwise;
- volume: `max_total_tokens: 5000000`;
- features: normally `routing` and `usage_reporting`, with add-ons copied from the base contract when applicable;
- replacement behavior: new `license_id` resets license-wide volume counters per #159.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_credit_5m_example_001",
  "customer_id": "cust_example_enterprise",
  "product": "genai-smart-router",
  "sku": "credit-pack-5m",
  "features": ["routing", "usage_reporting"],
  "limits": {
    "max_total_tokens": 5000000,
    "max_total_requests": 100000,
    "max_callers": 50,
    "max_model_groups": 20
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["prod"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- old license is near or at `license-volume-exceeded`;
- replacing with the new file restores traffic;
- safe status shows the new `license_id`;
- license-wide counters reset while per-key caller quotas remain unchanged.

### `credit-pack-25m`

Purpose: larger prepaid volume license or top-up.

Default entitlement shape:

- term: 12 months unless contract says otherwise;
- volume: `max_total_tokens: 25000000`;
- features and add-ons match the order form;
- replacement behavior is identical to `credit-pack-5m`.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_credit_25m_example_001",
  "customer_id": "cust_example_enterprise",
  "product": "genai-smart-router",
  "sku": "credit-pack-25m",
  "features": ["routing", "usage_reporting", "usage_csv_export"],
  "limits": {
    "max_total_tokens": 25000000,
    "max_total_requests": 500000,
    "max_callers": 50,
    "max_model_groups": 20
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["prod"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- same as `credit-pack-5m`;
- usage reporting/export access matches the features encoded in the license.

### `marketplace-seat`

Purpose: AWS/Azure private offer or similar marketplace-driven contract.

Default entitlement shape:

- term: marketplace contract term;
- volume: per-seat or contract-specific total/window volume;
- features: at least `routing` and `usage_reporting`; add enterprise features according to private offer;
- support tier and renewal date must match the marketplace entitlement record.

Example unsigned payload:

```json
{
  "schema_version": 1,
  "license_id": "lic_marketplace_example_001",
  "customer_id": "cust_example_marketplace",
  "product": "genai-smart-router",
  "sku": "marketplace-seat",
  "features": [
    "routing",
    "usage_reporting",
    "admin_reports",
    "dynamic_score",
    "model_group_contracts"
  ],
  "limits": {
    "max_total_tokens": 10000000,
    "max_callers": 25,
    "max_model_groups": 15,
    "max_admins": 5,
    "max_retention_days": 60
  },
  "deployment": {
    "mode": "self_hosted",
    "allowed_environments": ["prod"]
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

Acceptance evidence:

- marketplace order and license expiry agree;
- safe status shows `sku=marketplace-seat`;
- feature access matches the private offer.

## Issuance Workflow

1. Confirm the entitlement record is approved by sales/commercial ops.
2. Select the license template from #36 and fill the #159 envelope fields.
3. Review payload for product identity, customer ID, SKU, features, limits, deployment scope, dates, key ID, issuer, and add-ons.
4. Generate a unique `license_id`. For replacements and top-ups, use a new `license_id`.
5. Sign the payload with the approved signing workflow.
6. Verify the signed envelope before delivery.
7. Store the safe issuance record in the commercial/support system.
8. Deliver `license.json` over the approved secure channel.
9. Ask the customer to install it and return only safe status/acceptance evidence.

Example local validation commands, using placeholder paths only:

```bash
rtk go run ./cmd/router-license sign \
  --payload tmp/license-payload.example.json \
  --key "$LICENSE_SIGNING_KEY_FILE" \
  --key-id metrum-license-ed25519-2026-01 \
  --out tmp/license.example.json

rtk go run ./cmd/router-license verify \
  --license tmp/license.example.json \
  --public-key tmp/license-public-key.example.pem

rtk go run ./cmd/router-license inspect \
  --license tmp/license.example.json
```

Do not run signing commands with real private-key paths in shared terminals, shell transcripts, or CI logs. Prefer the approved signing service when available.

## Customer Installation Handoff

Send the customer only:

- the signed `license.json`;
- the expected SKU, expiry date, and safe license ID;
- mount path and config snippet appropriate for their deployment;
- smoke-test commands from the public docs;
- support escalation instructions with request IDs and safe status fields.

Customer-facing config shape:

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

For normal shipped builds after #158, `enabled: false` must not be accepted as an unlicensed production mode. Until that lands, customer deployments must set `enabled: true` and keep `fail_open_for_dev: false`.

## Renewal Workflow

Use renewal when the customer keeps the same commercial shape and needs a new term.

1. Confirm renewal order, support tier, and any changed add-ons.
2. Issue a new license with a new `license_id`, current `issued_at`, current `not_before`, updated `expires_at`, and preserved or updated limits.
3. Ask the customer to stage the file beside the current runtime license using deployment secret controls.
4. Replace the file atomically when possible.
5. Restart the router or wait for `recheck_interval`.
6. Verify `/readyz`, authorized `/admin/license/status`, metrics-admin license gauges, and one caller smoke.
7. Record acceptance evidence and close the renewal task.

Rollback: restore the previous valid license if it is still within term/grace and still matches the deployment. If the old license is expired, rollback requires issuing a corrected replacement license.

## Replacement Workflow

Use replacement when a customer needs corrected fields, changed features, changed instance binding, a recovered license file, or a signing-key rotation.

1. Confirm why replacement is needed.
2. Confirm whether the old license should remain valid until replacement is installed.
3. Issue a new license with a new `license_id`.
4. Preserve commercial limits unless the approved order changed them.
5. Ask the customer to replace the file and restart or wait for recheck.
6. Verify safe status shows the new license ID and expected SKU.
7. Record old and new license IDs in the commercial/support system.

Do not ask customers to manually edit a signed license. Any payload change invalidates the signature.

## Volume Top-Up Workflow

Use top-up for `credit-pack-*` licenses or prepaid volume extensions.

1. Confirm the top-up order and target customer/deployment.
2. Choose the template (`credit-pack-5m`, `credit-pack-25m`, or contract-specific volume).
3. Issue a replacement license with a new `license_id` and the new volume limit.
4. Tell the customer that license-wide counters reset on replacement per #159, while caller-key quotas remain separate.
5. Replace the file and restart or wait for recheck.
6. Verify safe status shows the new license ID, SKU, limit, and healthy readiness.
7. Run or request a small caller smoke.

Top-up is not a manual database reset. Do not edit customer license state files except under an approved incident procedure.

## Offline And Air-Gapped Customers

The router validates licenses offline. No runtime network call to Metrum is required for license checks.

Offline issuance and renewal:

1. Customer sends commercial order details and, if required, a safe instance fingerprint through the approved support path.
2. Metrum issues a signed `license.json`.
3. Customer transfers the file into the air-gapped environment using their approved media process.
4. Customer mounts the file read-only and persists the license state file on durable storage.
5. Customer returns safe status output, expiry, SKU, license ID, and request IDs if anything fails.

For air-gapped support, use redacted screenshots or copied safe status JSON only. Do not request provider keys, router tokens, full config, raw prompts, raw images, or full license payloads.

## Support Playbook

Start every case by collecting:

- UTC timestamp;
- endpoint and request ID;
- safe license status and reason;
- router version;
- SKU, license ID, key ID, expiry, and grace-active flag from safe status;
- whether the issue started after renewal, replacement, clock changes, host migration, or config changes.

| Symptom | Likely cause | Support action |
|---|---|---|
| `license-missing` | File absent, wrong path, unreadable permissions, or mount failure. | Verify configured path, file owner/mode, container UID, and mount. Deliver replacement file if lost. |
| `license-invalid` | Malformed file, wrong signature, unknown key ID, modified payload, truncated transfer. | Re-deliver the signed file through secure channel. Do not debug by pasting payloads into tickets. |
| `license-expired` | Term ended. | Renew or issue temporary evaluation extension if commercially approved. |
| `license-not-yet-valid` | Future `not_before` or wrong system clock. | Check UTC clock and issuance dates. Issue corrected replacement if dates are wrong. |
| `license-product-mismatch` | File is for another product. | Issue or deliver the GenAI Smart Router license. |
| `license-clock-rollback` | Local clock moved backward beyond tolerance. | Correct system time, inspect durable state policy, and escalate before touching state files. |
| `license-feature-forbidden` | Feature not present in SKU/add-on. | Confirm entitlement. Issue upgraded replacement or disable the unlicensed feature. |
| `license-limit-exceeded` | Config exceeds model group, caller, admin, retention, or other limit. | Reduce config or issue a license with approved larger limits. |
| `license-volume-exceeded` | License-wide request or token budget exhausted. | Process a credit-pack top-up or replacement license. |
| `license-window-exceeded` | Rolling request/token window exceeded. | Wait for window reset, reduce load, or issue approved larger window. |
| `license-concurrency-exceeded` | Router-wide in-flight limit reached. | Reduce concurrency, inspect clients, or issue approved higher limit. |
| `license-instance-limit-exceeded` | Instance fingerprint mismatch or too many instances. | Confirm migration/DR event and issue replacement with approved instance scope. |
| `license-skin-forbidden` | Request dialect not allowed by contract. | Confirm allowed skins or issue replacement. |
| `license-admin-limit-exceeded` | Admin subject count exceeds entitlement. | Reduce admin config or issue replacement. |
| `license-retention-limit-exceeded` | Configured retention exceeds entitlement. | Lower retention or issue replacement. |

Escalate to engineering when:

- a valid replacement does not become active after restart/recheck;
- safe status and caller-visible errors disagree;
- license state appears corrupted;
- clock rollback status persists after system time is corrected;
- counters do not reset after a new `license_id` top-up;
- a normal release build appears to run with licensing disabled.

## Acceptance Checklist

For every issued or replaced license, record:

- entitlement approval and issue timestamp;
- signed license verified before delivery;
- secure delivery completed;
- customer installed at expected path;
- `/readyz` result;
- authorized safe status result;
- one caller smoke result;
- feature-specific smoke for any add-on such as admin reports, security reports, private upstreams, TypeScript routing, external policy, PII filtering, or exports;
- renewal/top-up/replacement notes and old/new license IDs.

## Test Checklist

Run these checks when changing the license operations docs, examples, or tooling:

```bash
rtk python3 scripts/check_license_skus.py
rtk make docs-build
rtk make docs-qa
rtk make secret-check
```

When runtime license shape or signing tooling changes in #159, also add fixture signing/verification tests for each SKU template and run:

```bash
rtk go test ./cmd/... ./internal/...
```

## Stale-Doc Search

Before closing license operations work, search for stale placeholders and old guidance:

```bash
rtk rg -n "license.enabled: false|fail_open_for_dev|license-disabled|credit-pack|enterprise-annual|eval-72h|pilot-30d|marketplace-seat" README.md docs docs-site config.example.yaml scripts
```

Replace placeholder issue references with #159, keep public docs customer-focused, and keep internal signing details out of Docusaurus.
