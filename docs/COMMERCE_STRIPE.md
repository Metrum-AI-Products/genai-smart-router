# Commerce Stripe (internal / #921)

This note covers the slice-1 Stripe sandbox purchase path that verifies payment and may enqueue an isolated non-prod SQLite Fleet bootstrap. It does **not** claim that public self-serve checkout is shipped for customers; #42 owns customer-facing commercial copy.

## Ownership

- **#921** owns purchase app (`metrum-genai-commerce`), Stripe test catalog sync (`metrum-genai-commerce-catalog`), webhook verification, relational entitlement/order/event tables, commercial SKU↔Stripe Product/Price mapping from `docs/enterprise-license-skus.json`, and the commerce-sandbox Kubernetes overlay.
- **#555** owns Fleet lifecycle (`metrum-genai-smartrouter-fleetctl customer bootstrap` and related verbs).
- **#42** owns customer-facing commercial/package wording after entitlement and legal approval.
- Production Stripe keys and production Fleet profiles are out of scope for slice 1.

## Catalog desired state

Checked-in `docs/enterprise-license-skus.json` is the desired state. Self-serve SKUs carry:

- `billing_kind`, `stripe_mode`, `self_serve_stripe`, `auto_provision_instance`
- `price_placeholder` (engineering assumption only; do not publish dollar amounts in `docs-site/`)
- `stripe.product` + `stripe.price` with `lookup_key` equal to `sku` and `unit_amount` in cents matching `amount_usd`

Join key: catalog `sku` = Product/Price `metadata.sku` = Price `lookup_key`.

## Catalog CLI

```bash
export STRIPE_SECRET_KEY=sk_test_...   # shell wins over any env.json loader
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
metrum-genai-commerce \
  --addr :8091 \
  --catalog docs/enterprise-license-skus.json \
  --db commerce.sqlite \
  --success-url 'https://example.test/success' \
  --cancel-url 'https://example.test/cancel'
```

Endpoints:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/checkout/sessions` | Create Checkout Session (`mode` from catalog; line item from resolved Price) |
| `POST` | `/webhooks/stripe` | Verify signature; handle `checkout.session.completed` and `invoice.paid` idempotently on `evt_` id |
| `GET` | `/v1/entitlements/status` | Safe scalar entitlement/fulfillment status |
| `GET` | `/healthz` | Liveness |

Fail closed when Price `metadata.sku` is missing/unknown or `unit_amount`/interval disagree with the catalog. Duplicate `evt_` deliveries must not create a second entitlement or Fleet job.

Fleet bootstrap is optional and off by default. Set `COMMERCE_FLEET_ENABLED=1` plus explicit `COMMERCE_FLEET_*` refs to shell out to `metrum-genai-smartrouter-fleetctl customer bootstrap`. The purchase Deployment must not mount router `config.yaml`, provider `env.json`, or `license.json`, and must not receive tenant-namespace RBAC.

## Sandbox Kubernetes overlay

`deploy/kubernetes/overlays/commerce-sandbox/` deploys purchase (+ optional fulfillment) into namespace `smartrouter-commerce`. It does not patch the metrum-staging router Deployment. Secret manifests are examples only; never commit live Stripe keys.

## Refund / dispute

Refund or dispute should mark entitlement `revoked_pending`. Do not auto-delete the Fleet instance; cleanup remains signed Fleet `customer delete`.
