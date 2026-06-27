# License Operations Runbook

This document is internal. It describes the operational flow around issued licenses for enterprise self-hosted and private-managed deployments. It is paired with `docs/DEPLOYMENT.md` and `docs-site/docs/operations/license-protected-deployments.md`.

## Scope

- License issuance, renewal, and volume top-up for all commercial license templates.
- Support playbook for license errors and edge cases.
- Secret-handling boundaries.
- SKU-to-template mapping (placeholder until #36 lands).

This runbook is **not** a billing system. Payment, invoicing, and contract management are handled by Metrum commercial contact outside the product.

## License Templates

Each commercial SKU maps to a license template. Templates are defined in `COMMERCIALIZATION.md`; runtime fields and enforcement are tracked in #159.

| Template | Time | Volume | Typical use |
|---|---|---|---|
| `eval-72h` | +72 hours | 5,000,000 tokens total | Partner / prospective customer evals |
| `pilot-30d` | +30 days | 1M tokens / 1h window | Paid time-boxed pilot |
| `enterprise-annual` | +12 months | unlimited or contract ceiling | Default annual contract |
| `credit-pack-5m` | +12 months | 5,000,000 tokens total | Volume top-up |
| `credit-pack-25m` | +12 months | 25,000,000 tokens total | Larger volume prepay |
| `marketplace-seat` | term of contract | per-seat volume | AWS/Azure private offer |

## SKU to Template Mapping (placeholder)

The commercial mapping table is owned by #36. Until that lands, this runbook uses the following placeholder mapping. **Replace with the final mapping when #36 is closed.**

| SKU (placeholder) | Template | Default features |
|---|---|---|
| `eval-72h` | `eval-72h` | `routing`, `usage_reporting` |
| `pilot-30d` | `pilot-30d` | `routing`, `usage_reporting`, `dynamic_score` |
| `enterprise-annual` | `enterprise-annual` | full feature set |
| `credit-pack-5m` | `credit-pack-5m` | `routing`, `usage_reporting` |
| `credit-pack-25m` | `credit-pack-25m` | `routing`, `usage_reporting` |
| `marketplace-seat` | `marketplace-seat` | per marketplace listing |

## Operations Flow

### Issuance

1. Sales / order form produces an entitlement record: customer ID, SKU, volume, time window, enabled features, operational limits, optional instance fingerprint.
2. Metrum license signer produces the signed envelope using `cmd/router-license sign`:
   ```bash
   rtk go run ./cmd/router-license sign \
     --payload payload.json \
     --key ./metrum-license-ed25519-2026-01.priv \
     --key-id metrum-license-ed25519-2026-01 \
     --out license.json
   ```
3. Support delivers the signed `license.json` to the customer through a secure channel (out-of-band). **Never** commit to repo, paste in tickets, or attach to public docs.
4. Customer mounts the license at `server.license.path` (e.g. `/app/config/license.json`) and restarts the router.

### Renewal (time-bounded)

1. Generate a new payload with the new `expires_at` (typically +12 months for annual).
2. Sign with the same key and `key_id`.
3. Deliver and replace the existing `license.json` atomically where possible.
4. Restart the router or wait for `recheck_interval`.
5. Verify `/readyz` and authorized `/admin/license/status`.

### Volume Top-Up (`credit-pack-*`)

1. Confirm customer has reached the volume ceiling on the current license.
2. Generate a `credit-pack-*` payload with the new volume budget and the chosen `expires_at`.
3. Sign and deliver.
4. Replace `license.json`. The router resets the **license-wide** counters on license replacement (when `LastValidLicenseID` changes). Per-key counters are unaffected.
5. Verify `/readyz` and `/v1/usage` shows the new license in effect.

### Air-Gapped Customers

For air-gapped or offline customers:

- Pre-sign all anticipated license replacements before delivery.
- Deliver license files on approved physical media or secure file transfer.
- Document the expected renewal date in the customer's records.

## Support Playbook

### Common Errors

| Error | Typical operator action |
|---|---|
| `license-missing` | Verify the file is mounted at `server.license.path`; check permissions. |
| `license-invalid` | Verify the envelope signature is intact; check `key_id` matches the embedded public key. |
| `license-expired` | Issue a renewal license; replace the file. |
| `license-not-yet-valid` | Verify `not_before` and system clock. |
| `license-product-mismatch` | Verify license was issued for `genai-smart-router`. |
| `license-feature-forbidden` | Confirm feature is in the commercial plan; issue a new license if an upgrade was agreed. |
| `license-limit-exceeded` | Reduce configured usage or update the license (volume top-up, capacity increase). |
| `license-clock-rollback` | Correct system time and inspect the license state file. |
| `license-volume-exceeded` (new, #159) | Issue a `credit-pack-*` license for top-up. |
| `license-window-exceeded` (new, #159) | Confirm window/duration setting; issue higher-volume license or reduce usage. |
| `license-concurrency-exceeded` (new, #159) | Issue license with higher `max_concurrent` or scale usage. |
| `license-instance-limit-exceeded` (new, #159) | Verify instance fingerprint matches; issue multi-instance license. |
| `license-skin-forbidden` (new, #159) | Verify caller dialect is in `allowed_skins`; update license or routing config. |
| `license-admin-limit-exceeded` (new, #159) | Reduce admin count or upgrade license. |
| `license-retention-limit-exceeded` (new, #159) | Reduce `server.retention.days` or upgrade license. |

### Diagnostic Steps

1. Capture the `request_id` from the error response.
2. Inspect `request_usage.license_status` and `license_reason` for the affected caller.
3. If admin-authorized, query `/admin/license/status` for safe scalar license metadata.
4. If `license-invalid` or signature error, verify the file has not been modified in transit; re-issue if necessary.

### Replacement Audit Trail

Maintain a Metrum-side audit log (outside this repo) of:

- Customer ID and license ID.
- SKU and template.
- Issued / not-before / expires / grace.
- Signed-by key ID and operator.
- Replacement reason (initial issuance, renewal, top-up, upgrade, downgrade, revocation).
- Delivery channel and recipient.

## Secret-Handling Boundaries

- **Private signing keys** never leave Metrum signing infrastructure. They must not be committed to source, pasted in tickets, embedded in images, or copied to customer environments.
- **Customer license files** are deployment secrets. They must not be committed, shared between customers, or pasted in tickets.
- **Public verification keys** are embedded in the router binary and are safe to expose.
- **Logs and reports** must only emit safe scalar license metadata (`status`, `reason`, `license_id`, `customer_id`, `sku`, `key_id`, `expiry`, `grace_active`). Never log the payload, signature, or key bytes.

## Example License Payloads (placeholder)

The following are illustrative payloads for each template. Runtime fields not yet implemented are marked "(reserved by #159)".

### eval-72h

```json
{
  "schema_version": 1,
  "license_id": "lic_eval_...",
  "customer_id": "cust_...",
  "sku": "eval-72h",
  "features": ["routing", "usage_reporting"],
  "limits": {
    "max_total_tokens": 5000000,
    "max_total_requests": 5000,
    "max_callers": 3,
    "max_model_groups": 5
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2026-06-30T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

### enterprise-annual

```json
{
  "schema_version": 1,
  "license_id": "lic_annual_...",
  "customer_id": "cust_...",
  "sku": "enterprise-annual",
  "features": [
    "routing", "usage_reporting", "admin_reports", "admin_security_reports",
    "dynamic_score", "typescript_routing", "external_policy",
    "model_group_contracts", "retention_rollups", "private_upstreams",
    "pii_filtering", "usage_csv_export", "usage_baseline_export"
  ],
  "limits": {
    "max_callers": 100,
    "max_model_groups": 50,
    "max_admins": 10,
    "max_retention_days": 90,
    "max_instances": 1
  },
  "deployment": {
    "mode": "self_hosted",
    "instance_fingerprint_required": true
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

### credit-pack-5m

```json
{
  "schema_version": 1,
  "license_id": "lic_credit_5m_...",
  "customer_id": "cust_...",
  "sku": "credit-pack-5m",
  "features": ["routing", "usage_reporting"],
  "limits": {
    "max_total_tokens": 5000000,
    "max_total_requests": 100000,
    "max_callers": 50,
    "max_model_groups": 20
  },
  "issued_at": "2026-06-27T00:00:00Z",
  "not_before": "2026-06-27T00:00:00Z",
  "expires_at": "2027-06-27T00:00:00Z",
  "key_id": "metrum-license-ed25519-2026-01",
  "issuer": "metrum-ai"
}
```

## Cross-References

- Public license-protected deployments: `docs-site/docs/operations/license-protected-deployments.md`.
- Public error codes: `docs-site/docs/reference/errors.md`.
- Runtime license enforcement: `internal/router/license.go`.
- License signing helper: `cmd/router-license/main.go`.
- Commercial strategy: `COMMERCIALIZATION.md`.
- Open issues: #159 (license shape), #36 (SKU mapping), #39 (this runbook's primary issue), #158 (mandatory enforcement in release builds).