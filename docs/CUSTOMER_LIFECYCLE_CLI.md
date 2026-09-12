# Customer Lifecycle CLI (`metrum-ai-router-customer-lifecycle`)

Internal operator CLI that automates a **customer onboard** from one JSON intent, then wraps day-2 Fleet verbs. It **composes** existing packaged binaries; it does **not** replace `#555` `metrum-ai-router-fleetctl`. Payment/checkout is **out of band** and is not performed by this CLI.

Operators and production hosts **do not have source trees**. Acceptance and day-2 work must use release/package binaries only (for example under `dist/bin/` or a release tarball), with `PATH` and/or `METRUM_FLEET_BIN_DIR` pointing at that directory. Do not rely on `go run` or mid-flight `kubectl` secret/config patches.

Related:

- Fleet mutation authority: `#555`, [`CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md`](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md)
- Package boundaries: [`ADR_FLEET_AND_CUSTOMER_CLI_BOUNDARIES.md`](ADR_FLEET_AND_CUSTOMER_CLI_BOUNDARIES.md)

## Ownership

| Step | Binary / API | Notes |
|---|---|---|
| Collect / validate | `metrum-ai-router-customer-lifecycle` | Hostname + BYOK + fleet-eks durable path gates |
| License | `metrum-ai-router-license` + SSM | Publish signed `license.json` to `license_ref` (payment out of band) |
| Provision | `metrum-ai-router-fleetctl customer bootstrap` | Explicit after licensed |
| Status / smoke / update / delete | thin wrappers → fleetctl | Same refs as intent; update signs and redeploys |

## Packaged binaries

Build or unpack into a single directory, then export:

```bash
export METRUM_FLEET_BIN_DIR=/path/to/dist/bin
export PATH="${METRUM_FLEET_BIN_DIR}:${PATH}"

# Required companion binaries on PATH / METRUM_FLEET_BIN_DIR:
#   metrum-ai-router-customer-lifecycle
#   metrum-ai-router-fleetctl
#   metrum-ai-router-fleet-sign
#   metrum-ai-router-license
```

## Intent JSON

Checked-in placeholder: [`examples/customer-lifecycle/onboard-acme.sandbox.example.json`](../examples/customer-lifecycle/onboard-acme.sandbox.example.json).

Required fields:

- `customer_id`, `hostname` (must be `{customer_id}.<approved-apps-domain>` per the protected profile domain policy; either may be derived from the other). Example: `acme.apps.example.test`
- `sku`, `customer_email`, `customer_alias`, `owner_user`, `project`
- `profile_ref` (`aws-ssm:///…`), `license_ref` (`aws-ssm:///…`), `runtime_bundle_ref` (`aws-ssm:///…` or `aws-secretsmanager:///…`)
- `config_file`, `sign_key` (Fleet lifecycle approval), `license_key`, `license_key_id`
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
`validate-intent` and `onboard` **fail closed** if these paths are missing, relative, or still under `/app/config` / `/app/state` (Fleet `--rewrite-paths fleet-eks` only remaps `/app/state` and `/app/logs`). Runtime `env.json` is built from the BYOK file plus `ROUTER_HTTP_REFERER=https://{hostname}` — never restic/ops admin secrets, never a wholesale production-sync copy.

## Commands

```bash
# Structural + BYOK + fleet-eks path checks (no AWS mutation)
metrum-ai-router-customer-lifecycle validate-intent --intent /protected/acme/onboard.json

# Onboard: validate → license SSM → fleetctl bootstrap (payment out of band)
metrum-ai-router-customer-lifecycle onboard --intent /protected/acme/onboard.json

# Resume after license published
metrum-ai-router-customer-lifecycle onboard --intent /protected/acme/onboard.json \
  --from-step provision

metrum-ai-router-customer-lifecycle status --intent /protected/acme/onboard.json
metrum-ai-router-customer-lifecycle smoke --intent /protected/acme/onboard.json

# Day-2: publish runtime bundle AND signed Fleet redeploy (one verb)
metrum-ai-router-customer-lifecycle update-config --intent /protected/acme/onboard.json \
  --patch-file /protected/acme/patch.yaml
metrum-ai-router-customer-lifecycle update-config --intent /protected/acme/onboard.json --refresh-byok

metrum-ai-router-customer-lifecycle export-usage --intent /protected/acme/onboard.json \
  --out-dir /protected/acme/usage-export --token-file /protected/acme/CALLER_TOKEN.txt
metrum-ai-router-customer-lifecycle delete --intent /protected/acme/onboard.json
```

Safe scalar progress is stored under `~/.local/share/metrum-fleet/<customer_id>/customer-lifecycle-state.json` (mode `0600`).

### Payment

Payment and checkout are **out of band**. This CLI does not create Checkout sessions or poll a commerce entitlement API. `--from-step pay` fails closed with `payment is out of band`. Operators issue/publish the license after commercial entitlement is confirmed outside this tree.

### `update-config` / `--refresh-byok`

Always: rebuild instance env when requested → `publish-runtime-bundle` (`--rewrite-paths fleet-eks`, **without** `--strip-callers` so day-2 BYOK refresh preserves granted callers) → optional `update-config` patch or `write-manifest` → **signed** `customer create --sign-with-key`. Publish-and-return without redeploy is not a successful day-2 update for operators without source.

## License publish

1. Render/sign with `metrum-ai-router-license issue` (existing signing root; no new key authority).
2. `ssm:PutParameter` SecureString at `license_ref` (`aws-ssm:///…`) using **operator IAM**.
   The lifecycle CLI clears any inherited Fleet STS session env before PutParameter so a
   prior `fleetctl` assume-role cannot AccesDenied the write. The Fleet lifecycle role
   remains read-oriented for deploy.

Fleet `EnsureLicenseBinding` resolves that parameter into the tenant `router-license` Secret as `license.json`. Operator IAM must allow `PutParameter` on the customer license path.

## Security

- Never print/commit BYOK values, router tokens, license private keys, or full production config.
- `export-usage` writes report JSON only into a mode-`0700` directory; it does not export BYOK.
- Keep `/protected/**` and workspace token/license files out of git.
