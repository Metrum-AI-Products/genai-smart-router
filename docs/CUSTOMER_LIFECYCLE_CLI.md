# Customer Lifecycle CLI (`metrum-genai-customer-lifecycle`)

Internal operator CLI that automates a **3-step customer onboard** from one JSON intent, then wraps day-2 Fleet verbs. It **composes** existing packaged binaries; it does **not** replace `#555` `metrum-genai-smartrouter-fleetctl`, and it does **not** put Stripe into the router.

Operators and production hosts **do not have source trees**. Acceptance and day-2 work must use release/package binaries only (for example under `dist/bin/` or a release tarball), with `PATH` and/or `METRUM_FLEET_BIN_DIR` pointing at that directory. Do not rely on `go run` or mid-flight `kubectl` secret/config patches.

Related:

- Purchase / entitlement: `#921`, [`COMMERCE_STRIPE.md`](COMMERCE_STRIPE.md)
- Fleet mutation authority: `#555`, [`CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md`](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md)
- Staging commerce Deploy remains `#929` (EKS commerce-pod). Local ShellFleet + this CLI is the operator acceptance path for sandbox paid onboard.

## Ownership

| Step | Binary / API | Notes |
|---|---|---|
| Collect / validate | `metrum-genai-customer-lifecycle` | Hostname + BYOK + fleet-eks durable path gates |
| Pay | `metrum-genai-commerce` HTTP | Real Stripe Checkout is the pay gate; poll entitlement |
| License | `metrum-genai-smartrouter-license` + SSM | Publish signed `license.json` to `license_ref` |
| Provision | `metrum-genai-smartrouter-fleetctl customer bootstrap` | Explicit after paid+licensed |
| Status / smoke / update / delete | thin wrappers → fleetctl | Same refs as intent; update signs and redeploys |

## Packaged binaries

Build or unpack into a single directory, then export:

```bash
export METRUM_FLEET_BIN_DIR=/path/to/dist/bin
export PATH="${METRUM_FLEET_BIN_DIR}:${PATH}"

# Required companion binaries on PATH / METRUM_FLEET_BIN_DIR:
#   metrum-genai-customer-lifecycle
#   metrum-genai-smartrouter-fleetctl
#   metrum-genai-smartrouter-fleet-sign
#   metrum-genai-smartrouter-license
#   metrum-genai-commerce   # local pay gate only
```

## Intent JSON

Checked-in placeholder: [`examples/customer-lifecycle/onboard-acme.sandbox.example.json`](../examples/customer-lifecycle/onboard-acme.sandbox.example.json).

Required fields:

- `customer_id`, `hostname` (must be `{customer_id}.<approved-apps-domain>` per the protected profile domain policy; either may be derived from the other). Example: `acme.apps.example.test`
- `sku`, `customer_email`, `customer_alias`, `owner_user`, `project`
- `profile_ref` (`aws-ssm:///…`), `license_ref` (`aws-ssm:///…`), `runtime_bundle_ref` (`aws-ssm:///…` or `aws-secretsmanager:///…`)
- `config_file`, `sign_key` (Fleet lifecycle approval), `license_key`, `license_key_id`
- `commerce_base_url`
- **BYOK (required, fail-closed):**
  - `byok_env_file` — mode `0600` JSON of provider env vars (never committed)
  - `byok.provider`, `byok.api_key_env`, `byok.model`

Raw API keys must **never** appear in the intent JSON. Paths that look like Metrum `production-sync` or `env.production.json` are rejected.

Minimal instance config should route only the BYOK model (start from [`examples/commerce-customer-runtime/`](../examples/commerce-customer-runtime/)). That template already uses Fleet EKS durable paths:

- **Top-level** `state_path` under `/var/lib/smart-llmrouter/…` (not nested under `server:` — nesting is ignored and causes CrashLoopBackOff on relative `./.state-*`)
- logging, usage, and license state under `/var/lib/smart-llmrouter/…`
- `server.license.path` (and revocation path) under `/etc/smart-llmrouter-license/…`
- `server.diagnostics.retention_days` within the SKU `max_retention_days` (eval-72h is 7; the template sets 7 so readyz does not return `license-retention-limit-exceeded`)
- `providers.<byok>.api_key: ${<api_key_env>}` plus matching `api_key_env` so LoadConfig expands the BYOK credential into Authorization
- `project_memberships[].user_id` (not `user`) plus matching `users`/`projects` so the router account directory loads without CrashLoop
`validate-intent` and `onboard` **fail closed** if these paths are missing, relative, or still under `/app/config` / `/app/state` (Fleet `--rewrite-paths fleet-eks` only remaps `/app/state` and `/app/logs`). Runtime `env.json` is built from the BYOK file plus `ROUTER_HTTP_REFERER=https://{hostname}` — never Stripe/restic/commerce admin, never a wholesale production-sync copy.

## Commands

```bash
# Structural + BYOK + fleet-eks path checks (no Stripe / AWS mutation)
metrum-genai-customer-lifecycle validate-intent --intent /protected/acme/onboard.json

# 3-step onboard: validate → real Checkout + license SSM → fleetctl bootstrap
metrum-genai-customer-lifecycle onboard --intent /protected/acme/onboard.json

# Resume after Checkout completed in another terminal / browser
metrum-genai-customer-lifecycle onboard --intent /protected/acme/onboard.json \
  --from-step pay --skip-checkout

metrum-genai-customer-lifecycle status --intent /protected/acme/onboard.json
metrum-genai-customer-lifecycle smoke --intent /protected/acme/onboard.json

# Day-2: publish runtime bundle AND signed Fleet redeploy (one verb)
metrum-genai-customer-lifecycle update-config --intent /protected/acme/onboard.json \
  --patch-file /protected/acme/patch.yaml
metrum-genai-customer-lifecycle update-config --intent /protected/acme/onboard.json --refresh-byok

metrum-genai-customer-lifecycle export-usage --intent /protected/acme/onboard.json \
  --out-dir /protected/acme/usage-export --token-file /protected/acme/CALLER_TOKEN.txt
metrum-genai-customer-lifecycle delete --intent /protected/acme/onboard.json
```

Safe scalar progress is stored under `~/.local/share/metrum-fleet/<customer_id>/customer-lifecycle-state.json` (mode `0600`).

### Pay gate

Step 2 creates a **real** Stripe Checkout session via `metrum-genai-commerce` and prints the URL. Entitlement advances only after Stripe marks the session paid and the verified webhook updates commerce state. Do not forge webhook signatures as acceptance proof. For `$0` eval SKUs, complete the hosted Checkout (browser or Stripe test payment APIs that still produce a genuine paid session + webhook).

### `update-config` / `--refresh-byok`

Always: rebuild instance env when requested → `publish-runtime-bundle` (`--rewrite-paths fleet-eks`, **without** `--strip-callers` so day-2 BYOK refresh preserves granted callers) → optional `update-config` patch or `write-manifest` → **signed** `customer create --sign-with-key`. Publish-and-return without redeploy is not a successful day-2 update for operators without source.

## License publish (step 2)

After commerce entitlement reaches `active`, `provision_queued`, or `provisioned`:

1. Render/sign with `metrum-genai-smartrouter-license issue` (existing signing root; no new key authority).
2. `ssm:PutParameter` SecureString at `license_ref` (`aws-ssm:///…`) using **operator IAM**.
   The lifecycle CLI clears any inherited Fleet STS session env before PutParameter so a
   prior `fleetctl` assume-role cannot AccesDenied the write. The Fleet lifecycle role
   remains read-oriented for deploy.

Fleet `EnsureLicenseBinding` resolves that parameter into the tenant `router-license` Secret as `license.json`. Operator IAM must allow `PutParameter` on the customer license path.

## Local ShellFleet vs EKS commerce-pod

| Path | Purpose |
|---|---|
| Local packaged `metrum-genai-commerce` + `stripe listen` + this CLI | Operator sandbox E2E; explicit fleetctl bootstrap after pay+license |
| `#929` commerce Deployment in `smartrouter-commerce` | Staging/production purchase pod; separate from this CLI acceptance path |

Do not treat `COMMERCE_FLEET_MODE=shell` auto-fulfillment as the greenfield acceptance path for BYOK customers; prefer `onboard` → explicit bootstrap.

## Security

- Never print/commit BYOK values, router tokens, license private keys, Stripe secrets, or full production config.
- `export-usage` writes report JSON only into a mode-`0700` directory; it does not export BYOK.
- Keep `/protected/**` and workspace token/license files out of git.
