# Commerce Stripe (internal / #921)

This note covers the Stripe sandbox purchase path that verifies payment and may enqueue an isolated non-prod SQLite Fleet bootstrap. It does **not** claim that public self-serve checkout is shipped for customers; #42 owns customer-facing commercial copy.

## Ownership

- **#921** owns purchase app (`metrum-genai-commerce`), Stripe test catalog sync (`metrum-genai-commerce-catalog`), webhook verification, relational entitlement/order/event/customer tables, commercial SKU↔Stripe Product/Price mapping from `docs/enterprise-license-skus.json`, admin SQLite APIs, and the commerce-sandbox Kubernetes overlay.
- **#555** owns Fleet lifecycle (`metrum-genai-smartrouter-fleetctl customer bootstrap` and related verbs).
- **#42** owns customer-facing commercial/package wording after entitlement and legal approval.
- Production Stripe keys and production Fleet profiles are out of scope for slice 1.

## Env split (instance vs commerce vs ops)

Router instance env is the same shape as any customer deployment: provider keys + `ROUTER_HTTP_REFERER` only. Never put Stripe, restic, backup, or commerce admin secrets into instance `env.json` or into Fleet runtime bundles.

| Tracked example | Ignored runtime file | Purpose |
|---|---|---|
| `env.example.json` | `env.json` | Instance/router provider keys |
| `commerce.env.example.json` | `commerce.env.json` | Stripe, commerce admin token, Fleet bootstrap refs |
| `ops.env.example.json` | `ops.env.json` | Restic/backup + work-dashboard port |

Customer bootstrap template: `examples/commerce-customer-runtime/` (instance-only `env.example.json` + minimal `config.example.yaml`).

Migrate a mixed local `env.json` (including nested `STRIPE_KEYS.SANDBOX_KEYS`) without printing secrets:

```bash
rtk python3 scripts/migrate_env_split.py --input env.json --dry-run
rtk python3 scripts/migrate_env_split.py --input env.json --force
```

`--force` is required to overwrite existing outputs. Unit coverage: `rtk python3 scripts/migrate_env_split_test.py`.

Commerce CLIs load `./commerce.env.json` from the working directory; **shell environment wins** over file values. They do not read router `env.json`.

## Catalog desired state

Checked-in `docs/enterprise-license-skus.json` is the desired state. Self-serve SKUs carry:

- `billing_kind`, `stripe_mode`, `self_serve_stripe`, `auto_provision_instance`
- `price_placeholder` (engineering assumption only; do not publish dollar amounts in `docs-site/`)
- `stripe.product` + `stripe.price` with `lookup_key` equal to `sku` and `unit_amount` in cents matching `amount_usd`

Join key: catalog `sku` = Product/Price `metadata.sku` = Price `lookup_key`.

## Catalog CLI

```bash
# Prefer commerce.env.json for STRIPE_*; shell still wins.
export STRIPE_SECRET_KEY=sk_test_...
metrum-genai-commerce-catalog plan  --mode test --catalog docs/enterprise-license-skus.json
metrum-genai-commerce-catalog apply --mode test --catalog docs/enterprise-license-skus.json
metrum-genai-commerce-catalog status --mode test --catalog docs/enterprise-license-skus.json
```

Rules:

- `--mode live` is a hard error in slice 1.
- `--mode test` requires `STRIPE_SECRET_KEY` to start with `sk_test_`.
- Apply is idempotent: create/update Product; on amount/recurring change, archive the old Price and create a new one with `transfer_lookup_key`.
- Optional `--prune-unmanaged` archives active Prices whose `metadata.sku` is not in the catalog.

`status` exits non-zero when drift remains. Bring `status` to zero drift before enabling Checkout against that Stripe account.

## Purchase service

```bash
export STRIPE_SECRET_KEY=sk_test_...
export STRIPE_WEBHOOK_SECRET=whsec_...
# Optional admin + Fleet (from commerce.env.json or shell):
# export COMMERCE_ADMIN_TOKEN=...
# export COMMERCE_FLEET_ENABLED=1
# export COMMERCE_FLEET_MODE=fake|shell
metrum-genai-commerce \
  --addr :8091 \
  --catalog docs/enterprise-license-skus.json \
  --db tmp/metrum-commerce.sqlite \
  --success-url 'http://127.0.0.1:8091/success' \
  --cancel-url 'http://127.0.0.1:8091/cancel'
```

Default local DB path is `tmp/metrum-commerce.sqlite` (`tmp/` is gitignored). For a durable operator path use something like `~/.local/share/metrum-commerce/commerce.sqlite` and pass `--db` explicitly.

Endpoints:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/checkout/sessions` | Create Checkout Session (`mode` from catalog; line item from resolved Price) |
| `POST` | `/webhooks/stripe` | Verify signature; handle `checkout.session.completed` and `invoice.paid` idempotently on `evt_` id |
| `GET` | `/v1/entitlements/status` | Safe scalar entitlement/fulfillment status |
| `GET` | `/healthz` | Liveness |
| `GET` | `/success`, `/cancel` | Local Checkout redirect pages |

Fail closed when Price `metadata.sku` is missing/unknown or `unit_amount`/interval disagree with the catalog. Duplicate `evt_` deliveries must not create a second entitlement or Fleet job. Credit-pack SKUs with `auto_provision_instance: false` create entitlements but never call Fleet.

### Fleet after payment

Fleet bootstrap is optional and off by default (`COMMERCE_FLEET_ENABLED` unset → no runner; auto-provision jobs stay `queued`).

When `COMMERCE_FLEET_ENABLED=1`:

| `COMMERCE_FLEET_MODE` | Refs complete? | Runner |
|---|---|---|
| `fake` | any | `FakeFleetRunner` (CI / local proof) |
| `shell` | yes | `ShellFleetRunner` → `metrum-genai-smartrouter-fleetctl customer bootstrap` |
| `shell` | no | falls back to `FakeFleetRunner` |
| empty | yes | `ShellFleetRunner` |
| empty | no | `FakeFleetRunner` |

Required shell refs: `COMMERCE_FLEET_PROFILE_REF`, `COMMERCE_FLEET_LICENSE_REF`, `COMMERCE_FLEET_CONFIG_FILE`, `COMMERCE_FLEET_ENV_FILE`, `COMMERCE_FLEET_SIGN_KEY`. Shell bootstrap always passes `--rewrite-paths fleet-eks`, refuses missing refs, optionally parses safe hostname/job scalars from CLI JSON, and never logs token file contents.

Unique `fleet_customer_id` = sanitize(customer alias) or `c{entitlementID}`.

The purchase Deployment must not mount router `config.yaml`, provider `env.json`, or `license.json`, and must not receive tenant-namespace RBAC. Fleet customer runtime env must remain instance-only (no Stripe/restic).

### Admin SQLite APIs

Header: `Authorization: Bearer $COMMERCE_ADMIN_TOKEN` (`COMMERCE_ADMIN_TOKEN` from `commerce.env.json` or shell). If the token is unset, admin routes return 401.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/admin/orders` | List recent orders |
| `GET` | `/v1/admin/entitlements` | List entitlements |
| `GET` | `/v1/admin/customers` | List customers |
| `GET` | `/v1/admin/fulfillment-jobs` | List fulfillment jobs |
| `POST` | `/v1/admin/fulfillment/{id}/resume` | Re-run queued/failed Fleet bootstrap |

Admin responses are safe scalars only (no Stripe secrets, tokens, or provider keys).

## Sandbox Kubernetes overlay

`deploy/kubernetes/overlays/commerce-sandbox/` deploys purchase (+ optional fulfillment) into namespace `smartrouter-commerce`. It does not patch the metrum-staging router Deployment. Secret manifests are examples only; never commit live Stripe keys.

The overlay ConfigMap copies `enterprise-license-skus.json` from this directory (kept in sync with `docs/enterprise-license-skus.json`). After catalog edits, refresh the overlay copy before render:

```bash
cp docs/enterprise-license-skus.json deploy/kubernetes/overlays/commerce-sandbox/enterprise-license-skus.json
kubectl kustomize deploy/kubernetes/overlays/commerce-sandbox >/tmp/commerce-sandbox.yaml
```

Staging apply requires: (1) MFA/SSO session for EKS, (2) a published commerce image digest replacing `registry.example.com/metrum-genai-commerce:sandbox`, (3) out-of-band Secret `commerce-stripe` with test keys, (4) namespace bootstrap RBAC for `smartrouter-commerce` (delivery role is router-overlay scoped and does not create this namespace by default).

## Refund / dispute

Refund or dispute should mark entitlement `revoked_pending`. Do not auto-delete the Fleet instance; cleanup remains signed Fleet `customer delete`.
