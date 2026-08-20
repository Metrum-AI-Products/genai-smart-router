# License Operations Runbook

This runbook is for Metrum operators who issue, renew, replace, top up, and support GenAI Smart Router enterprise licenses. It is intentionally internal: customer-facing Docusaurus docs explain how to install and renew a license, but they do not describe signing-key custody, signing service internals, or development bypass mechanics.

Related work:

- #36 defines commercial SKU templates and the SKU-to-entitlement map.
- #164 tracks production-grade license generation tooling.
- #167 tracks deployment binding.
- #168 tracks revocation bundles.
- #169 tracks online license leases.
- #159 defines the license envelope shape for capability, time, volume, and operational limits.
- #158 makes license enforcement mandatory for normal builds and leaves any no-license mode as an explicit internal development build.
- #921 owns the Stripe-verified purchase path and commercial entitlement/fulfillment boundary; #545 is superseded for that launch path.
- #42 owns customer-facing commercial and package copy.

The machine-readable source for launch SKU templates is `docs/enterprise-license-skus.json`. Keep that artifact, this runbook, and customer-facing Docusaurus license docs aligned when packaging changes. GitHub issue #921 is the commerce purchase/entitlement source of truth; #42 owns the corresponding customer-facing commercial copy.

## Operating Rules

- Never paste private signing keys, signing-service credentials, real customer license files, customer identifiers, router tokens, token hashes, provider API keys, or full production config into tickets, docs, logs, shell history, or chat.
- Store private signing keys only in the approved signing system or secret manager. The router runtime needs public verification keys only.
- Deliver `license.json` through an approved secure channel. Do not attach real licenses to GitHub issues, pull requests, public docs, package docs, or sample configs.
- Use safe scalar license metadata for support: `license_id`, customer alias, SKU, `key_id`, issuer, status, reason, expiry, grace flag, and request IDs.
- Treat license issuance, renewal, replacement, and top-up as commercial events. Record them in the commercial/support system, not in this repository.
- Treat license and quota state files as tamper-evident runtime state. The router signs state with deployment-local integrity metadata and fails closed when state is edited or replaced with raw JSON. Do not hand-edit counters, disabled flags, grace fields, or feature lists; use a reviewed license replacement/top-up or restore a trusted state backup. Legacy unsigned quota-state import requires an explicit one-time operator migration with `SMART_LLMROUTER_ALLOW_UNSIGNED_STATE_MIGRATION=1`, followed by restart without that flag after the router rewrites signed state.
- Treat approved commercial, procurement, and entitlement events as control-plane inputs. They may authorize or block license operations; they do not belong in router config, license payload examples, or runtime logs except as safe external record references in the commercial/support system.
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
| Commercial control-plane operator | Maintains approved commercial and entitlement records, and reconciles authorized fulfillment requests under #921. Customer-facing commercial copy is governed by #42. |

Maintain an external entitlement record with at least:

- customer legal name and internal customer ID;
- deployment alias and environment;
- commercial SKU and license template;
- approved features, limits, term, support tier, and add-ons;
- signer identity, signing key ID, license ID, issue time, expiry, and delivery channel;
- replacement reason, if replacing an earlier license;
- acceptance evidence and support ticket links.
- commercial system references such as order, quote, invoice, payment, refund, dispute, cancellation, portal account, or marketplace private-offer IDs when applicable.

## SKU Templates

These templates align with #36 and #159. They are examples for constructing the unsigned payload that is then signed by the approved license signer. Use real customer IDs, license IDs, dates, limits, and add-ons from the entitlement record.

The `product` field is required and must be `genai-smart-router`. Payload examples that include #159 fields such as `max_total_tokens`, `window_tokens`, `max_concurrent`, `max_admins`, `max_retention_days`, `max_instances`, or `allowed_instances` require a #159-capable `router-license` binary and router runtime. Do not sign those fields with older tooling: pre-#159 binaries ignore unknown JSON fields during payload decoding.

Launch SKU summary:

| SKU | Template | Commercial motion | Billing | Self-serve Stripe | Auto-provision | Default term | Primary use |
|---|---|---|---|---|---|---|---|
| `eval-72h` | `eval-72h` | Metrum-managed evaluation | payment | yes | yes | 72 hours | Free or partner proof window |
| `pilot-30d` | `pilot-30d` | Paid pilot (license template) | payment | no | no | 30 days | License template for hosted pilot |
| `hosted-pilot-30d` | `pilot-30d` | Paid pilot hosted | payment | yes | yes | 30 days | Self-serve hosted pilot Checkout |
| `credit-pack-5m` | `credit-pack-5m` | Volume top-up | top_up | yes | no | 12 months | Small prepaid or top-up envelope |
| `credit-pack-25m` | `credit-pack-25m` | Volume top-up | top_up | yes | no | 12 months | Larger prepaid or top-up envelope |
| `hosted-instance-monthly` | `hosted-instance-monthly` | Hosted subscription | subscription | yes | yes (first paid) | 1 month | Monthly hosted instance |
| `hosted-instance-annual` | `hosted-instance-annual` | Hosted subscription | subscription | yes | yes (first paid) | 12 months | Annual hosted instance |
| `hosted-instance-additional-monthly` | `hosted-instance-additional-monthly` | Hosted add-on | subscription_addon | yes | yes | 1 month | Extra hosted instance |
| `enterprise-annual` | `enterprise-annual` | Enterprise self-hosted | invoice_only | no | no | 12 months | Default BYOK annual contract |
| `marketplace-seat` | `marketplace-seat` | AWS/Azure private offer | invoice_only | no | no | Contract term | Marketplace procurement |

Placeholder USD amounts live only in `docs/enterprise-license-skus.json` (`pricing_status: placeholder_assumption`). Do not publish dollar amounts in `docs-site/`. Operator Stripe sync notes: `docs/COMMERCE_STRIPE.md` (#921).

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
2. Create an entitlement file from `docs/enterprise-license-skus.json` plus the approved customer, deployment, date, add-on, limit, and signing-key fields.
3. Render and validate the unsigned payload with `router-license template render`.
4. Issue with `router-license issue`; it renders, validates, signs, verifies, writes `license.json`, and emits optional safe handoff artifacts in one auditable flow.
5. Store the safe issuance record in the commercial/support system.
6. Deliver only the signed `license.json` over the approved secure channel.
7. Ask the customer to install it and return only safe status/acceptance evidence.

Example entitlement file, using placeholder values only:

```yaml
customer_id: cust_example_enterprise
customer_name: Example Enterprise
sku: enterprise-annual
deployment:
  mode: self_hosted
  allowed_environments: ["production", "staging"]
  instance_fingerprint_required: true
  allowed_instances: ["fp:example-instance-fingerprint"]
term:
  issued_at: 2026-06-28T00:00:00Z
  not_before: 2026-06-28T00:00:00Z
  expires_at: 2027-06-28T00:00:00Z
features:
  add:
    - admin_security_reports
limits:
  max_callers: 150
  max_model_groups: 60
  max_admins: 12
  allowed_skins:
    - openai-chat
    - openai-responses
    - anthropic-messages
signing:
  key_id: metrum-license-ed25519-2026-06-prod
```

Example issuance commands, using placeholder paths only:

```bash
rtk go run ./cmd/router-license template list \
  --catalog docs/enterprise-license-skus.json

rtk go run ./cmd/router-license template render \
  --catalog docs/enterprise-license-skus.json \
  --entitlement tmp/entitlement.example.yaml \
  --out tmp/license-payload.example.json

rtk go run ./cmd/router-license issue \
  --catalog docs/enterprise-license-skus.json \
  --entitlement tmp/entitlement.example.yaml \
  --key "$LICENSE_SIGNING_KEY_FILE" \
  --out tmp/license.example.json \
  --payload-out tmp/license-payload.example.json \
  --summary-out tmp/license-summary.example.json \
  --checklist-out tmp/license-checklist.example.md

rtk go run ./cmd/router-license verify \
  --license tmp/license.example.json \
  --public-key tmp/license-public-key.example.pem

rtk go run ./cmd/router-license safe-summary \
  --license tmp/license.example.json
```

The command rejects unsupported feature names, unsupported API skins, invalid date ordering, invalid window cap/duration combinations, missing product/issuer/key ID fields, and key IDs absent from the runtime public verification set unless an explicit future-runtime/test override is used. It writes generated artifacts with `0600` permissions by default.

Do not run signing commands with real private-key paths in shared terminals, shell transcripts, or CI logs. Prefer the approved signing service when available. Private signing keys must stay outside this repository, release packages, production router hosts, and customer artifacts.

## Revocation Bundle Workflow

Use a signed revocation bundle when an already issued license must be blocked before its natural expiry, temporarily suspended, or marked superseded by a replacement license. Revocation bundles are offline artifacts verified with the same embedded public verification keys as license files. They are checked during normal license revalidation and are not bypassed by `grace_period_on_validation_error`.

Operator rules:

- each bundle must have a unique `revocation_set_id` and monotonically increasing `revocation_epoch`;
- keep bundle `not_before` and `expires_at` current for the deployment support window;
- use `revoked` for permanent cancellation, `suspended` for temporary commercial/support holds, and `superseded` when the customer must install a replacement license;
- set `effective_at` in the future only for planned enforcement; until then, status is safe-reported as `pending`;
- never send signing keys or private key paths to customer hosts.

Example:

```bash
rtk go run ./cmd/router-license revocation create \
  --set-id revset_customer_example_20260628 \
  --epoch 42 \
  --license-id lic_customer_example_2026 \
  --status superseded \
  --superseded-by lic_customer_example_replacement_2026 \
  --reason replacement-issued \
  --effective-at 2026-06-28T20:00:00Z \
  --expires-at 2026-07-28T20:00:00Z \
  --key "$LICENSE_SIGNING_KEY_FILE" \
  --key-id metrum-license-ed25519-2026-06-prod \
  --out tmp/revocations.example.json \
  --public-key tmp/license-public-key.example.pem

rtk go run ./cmd/router-license revocation validate \
  --bundle tmp/revocations.example.json \
  --public-key tmp/license-public-key.example.pem

rtk go run ./cmd/router-license revocation safe-summary \
  --bundle tmp/revocations.example.json
```

Customer-facing config shape when revocation enforcement is part of the support plan:

```yaml
server:
  license:
    revocation:
      mode: file
      path: /app/config/revocations.json
      require_current_bundle: true
      fail_closed_on_bundle_error: true
```

`require_current_bundle: true` fails closed when the bundle is absent. `fail_closed_on_bundle_error: true` fails closed when a configured bundle is malformed, expired, signed by an unknown key, has an invalid signature, or rolls back to an older observed epoch. Effective `revoked`, `suspended`, and `superseded` entries return `403` and must not enter license grace.

## Commercial And Customer Control-plane Boundary

GitHub issue #921 is the source of truth for Stripe-verified purchase, entitlement recording, and Fleet bootstrap fulfillment. GitHub issue #42 owns the customer-facing commercial and package copy. Public self-serve checkout is not claimed as shipped; see docs/COMMERCE_STRIPE.md for operator sandbox notes.

- An approved external entitlement record may authorize a license issuance, renewal, replacement, revocation, or top-up. The control-plane service must make that decision idempotently and retain only the safe references needed for support and audit.
- Keep payment data, payment-provider credentials, webhook secrets, signing-service credentials, and full customer records outside router configuration, deployment packages, and runtime logs. The router runtime never processes card data or holds private license-signing keys.
- Bind fulfillment to an approved SKU/template and deployment record. Verify the signed license before delivery; use the existing secure delivery and support workflow in this runbook.
- If a commercial decision changes an issued entitlement, choose a reviewed technical action such as a replacement license, revocation bundle, or lease-state change. Do not hand-edit customer license or quota state.
- Customer-facing materials must not describe a purchase, portal, download, or renewal mechanism as available until #921 sandbox acceptance is complete and #42 has approved the shipped wording.

## Deployment Binding, Revocation, And Online Leases

Deployment binding (#167), revocation bundles (#168), and online leases (#169) are separate enforcement mechanisms:

- Deployment binding constrains a license to approved instance fingerprints or deployment scopes. It is useful for enterprise production, DR, and private managed plans, but air-gapped customers need a preapproved replacement or migration process.
- Revocation bundles let a deployment learn that specific licenses or key IDs should no longer be honored. They are appropriate for compromised, refunded, disputed, or mistakenly issued licenses when the plan and contract allow revocation.
- Online leases let the router enforce short-lived signed authorization for monthly, card-paid, trial, usage-sensitive, or managed plans where payment state and concurrent-use controls matter.

Do not require online checks for every enterprise self-hosted deployment. The public product story must preserve offline signed-license operation for contracted and air-gapped customers.

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

Normal shipped builds must not accept `enabled: false` as an unlicensed production mode. Customer deployments must set `enabled: true` and keep `fail_open_for_dev: false`.

## Renewal Workflow

Use renewal when the customer keeps the same commercial shape and needs a new term.

1. Confirm renewal order, support tier, and any changed add-ons.
2. Issue a new license with a new `license_id`, current `issued_at`, current `not_before`, updated `expires_at`, and preserved or updated limits.
3. Ask the customer to stage the file beside the current runtime license using deployment secret controls.
4. Replace the file atomically when possible.
5. Restart the router or wait for `recheck_interval`.
6. Verify `/readyz`, authorized `/admin/license/status`, metrics-admin license gauges, and one caller smoke.
7. Record acceptance evidence and close the renewal task.

Example:

```bash
rtk go run ./cmd/router-license renew \
  --license tmp/current-license.example.json \
  --key "$LICENSE_SIGNING_KEY_FILE" \
  --expires-at 2028-06-28T00:00:00Z \
  --out tmp/renewed-license.example.json \
  --summary-out tmp/renewed-license-summary.example.json
```

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

Example:

```bash
rtk go run ./cmd/router-license top-up \
  --catalog docs/enterprise-license-skus.json \
  --license tmp/current-license.example.json \
  --sku credit-pack-25m \
  --key "$LICENSE_SIGNING_KEY_FILE" \
  --out tmp/top-up-license.example.json \
  --summary-out tmp/top-up-license-summary.example.json
```

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

Keep public docs customer-focused, and keep internal signing details out of Docusaurus.
