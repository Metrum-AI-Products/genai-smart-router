# Metrum EKS production operations

Canonical operator runbook for Metrum engineering production. All image,
config, ownership, verification, rollback, and decommission steps for tenant
`llm-api` live here. Dated cutover evidence remains in `deployment.md` (ledger
only).

Customer self-hosted Docker Compose stays a supported **customer** option in
`docs/DOCKER_DEPLOYMENT.md`. Do not use Compose for Metrum production changes.

## Topology (current)

| Field | Value |
| --- | --- |
| Cluster | shared `metrum` EKS (`us-east-1`) |
| Tenant / namespace | `llm-api` |
| Database | SQLite on the tenant PVC (one replica; no horizontal scale while file-backed) |
| Primary hostname | `https://llm-api.apps.metrum.ai` |
| Production aliases | `https://llm-api-engg.metrum.ai`, `https://llm-api.metrum.ai` |
| DNS | Both aliases CNAME to `llm-api.apps.metrum.ai` → EKS ingress (cutover 2026-09-01) |
| Live revision (as of cutover ledger) | `5b382c7` — deploying a newer digest is a separate change |
| Trusted proxy CIDR | `192.168.0.0/16` (never the legacy Compose-only `172.18.0.0/16`) |

| Surface | Role |
| --- | --- |
| Fleet EKS tenant `llm-api` | Sole writer for production traffic |
| Retained EC2 Compose host | Rollback window only (containers stood down); not a concurrent writer |
| Compose Postgres archives | Forensics only; EKS SQLite did not inherit Compose usage history |

## Protected production profile authority

| Field | Value |
| --- | --- |
| Profile SSM ref | `aws-ssm:///metrum/smartrouter/profiles/production` |
| `profile_id` | `metrum-production` |
| `environment` | `production` |
| Manifest stage | `production` (required; `write-manifest --stage production`) |
| Example fixture | `deploy/release/fixtures/metrum-production-profile.example.yaml` |
| Runtime bundle | `aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle` |
| License request | protected SSM license-request ref for `llm-api` (never print payload) |

Profile must include:

- `hostname_suffix: apps.metrum.ai` (primary `llm-api.apps.metrum.ai`)
- `approved_alias_hostnames` for `llm-api-engg.metrum.ai` and `llm-api.metrum.ai`
- `tls_secret_name: apps-metrum-ai-wildcard-tls` for the primary hostname
- per-alias `tls_secret_name: llm-api-metrum-ai-tls`
- pinned `approved_release_digest` for the immutable router image

Store the live profile in SSM only. Never commit secrets, runtime bundles,
license payloads, or signing keys.

Metrum production on Fleet was authorized 2026-09-01 (maintainer self-review
record below). Disposable non-production customers continue to use the staging
profile; they must not reuse the production profile ref.

## Prerequisites

Authenticate to AWS and the `metrum` EKS cluster **before** operator commands.
Login is never part of numbered rollout steps. If identity checks fail, stop
with a secret-free “authenticate first, then retry” message.

```bash
rtk aws sts get-caller-identity
rtk kubectl config current-context   # must be metrum cluster
```

### Read-only discovery (safe scalars only)

```bash
# DNS: aliases should CNAME to llm-api.apps.metrum.ai → ingress ELB
rtk dig +short llm-api-engg.metrum.ai CNAME
rtk dig +short llm-api.metrum.ai CNAME
rtk dig +short llm-api.apps.metrum.ai

rtk kubectl get svc -A -l app.kubernetes.io/name=ingress-nginx
rtk kubectl -n llm-api get ingress router
rtk kubectl -n llm-api get secret
rtk kubectl get certificate -A 2>/dev/null || true
```

Record only: ingress external hostname, pod CIDR (expect `192.168.0.0/16`),
TLS Secret names (`apps-metrum-ai-wildcard-tls`, `llm-api-metrum-ai-tls`), and
namespace/owner labels. Never print Secret values, DSNs, tokens, or full config.

## One-time ownership transition

Live tenant `llm-api` was first deployed under the **staging** nonproduction
owner. Promoting the same namespace/PVC to the production profile requires a
one-time `ownership_transition` on the protected production profile so Fleet
relabels owned resources without deleting the SQLite PVC.

| Identity | Value |
| --- | --- |
| Known live (source) owner | `instance-fdcca5e10ce3f145c4a7` |
| Source profile / stage | `staging-fleet-nonprod` / `nonproduction` |
| Target owner | `instance-2278b384bf563c59df11` |
| Target profile / stage | `metrum-production` / `production` |

Owner IDs are derived as
`instance-` + first 20 hex chars of
`SHA-256(profile_id || customer_id || stage)`.

### `ownership_transition` fields (profile YAML)

Set on the protected production profile for the transition window only:

```yaml
ownership_transition:
  customer_id: llm-api
  source_profile_id: staging-fleet-nonprod
  source_stage: nonproduction
  source_instance_id: instance-fdcca5e10ce3f145c4a7   # optional; must match derived source
  change_reference: "#1052"                           # non-secret change id (issue/PR)
  expires_at: 2026-09-10T00:00:00Z                    # RFC3339; must be in the future
```

Rules:

- Allowed only on `environment: production` profiles.
- `customer_id` must match the sealed manifest customer.
- Source identity must differ from the target production owner.
- `change_reference` is required; no secret-shaped strings or URLs.
- Remove `ownership_transition` from the protected profile after the promotion
  succeeds so routine deploys do not re-enter transition.

### Transition deploy

```bash
export FLEET_PROFILE_REF='aws-ssm:///metrum/smartrouter/profiles/production'
export FLEET_RUNTIME_BUNDLE_REF='aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle'
export FLEET_LICENSE_REF='aws-ssm:///metrum/smartrouter/fleet/llm-api/license-request'

metrum-genai-smartrouter-fleetctl customer write-manifest \
  --customer-id llm-api \
  --profile-ref "$FLEET_PROFILE_REF" \
  --runtime-bundle-ref "$FLEET_RUNTIME_BUNDLE_REF" \
  --license-ref "$FLEET_LICENSE_REF" \
  --stage production \
  --revision-prefix llm-api-ownership-transition

metrum-genai-smartrouter-fleetctl customer create \
  --customer-id llm-api \
  --sign-with-key /protected/lifecycle_approval_private_key.b64
```

Confirm status reports the target `instance-2278b384bf563c59df11`, namespace
`llm-api` remains ready, and the PVC/owner labels match the target. Then strip
`ownership_transition` from the SSM production profile.

## Routine image or config deploy

Pin one immutable image digest in the production profile’s
`approved_release_digest`. Publish runtime-bundle updates to Secrets Manager
first when config or `env.json` changes. Validate config locally without
printing secrets:

```bash
rtk python3 scripts/prepare_fleet_production_bundle.py /protected/runtime-config.production-identical.yaml
rtk go test ./cmd/... ./internal/...
```

Then:

```bash
export FLEET_PROFILE_REF='aws-ssm:///metrum/smartrouter/profiles/production'
export FLEET_RUNTIME_BUNDLE_REF='aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle'
export FLEET_LICENSE_REF='aws-ssm:///metrum/smartrouter/fleet/llm-api/license-request'

metrum-genai-smartrouter-fleetctl customer write-manifest \
  --customer-id llm-api \
  --profile-ref "$FLEET_PROFILE_REF" \
  --runtime-bundle-ref "$FLEET_RUNTIME_BUNDLE_REF" \
  --license-ref "$FLEET_LICENSE_REF" \
  --stage production \
  --revision-prefix llm-api-<purpose>

metrum-genai-smartrouter-fleetctl customer create \
  --customer-id llm-api \
  --sign-with-key /protected/lifecycle_approval_private_key.b64
```

Day-2 config patches may use `customer update-config` / `publish-runtime-bundle`
then the same `write-manifest --stage production` + signed `create` activation.
Never mutate live Secrets or Deployments out of band.

| Task | Command surface |
| --- | --- |
| Status | `metrum-genai-smartrouter-fleetctl customer status --customer-id llm-api --profile-ref "$FLEET_PROFILE_REF"` |
| Smoke | `metrum-genai-smartrouter-fleetctl customer smoke --customer-id llm-api ...` |
| Config / image | `write-manifest --stage production` + signed `customer create` |
| List callers | `customer list-callers` with production refs (safe JSON only) |

## Verification

```bash
rtk curl -fsS https://llm-api-engg.metrum.ai/readyz
rtk curl -fsS https://llm-api.metrum.ai/readyz
rtk curl -fsS https://llm-api.apps.metrum.ai/readyz
rtk curl -fsS https://llm-api-engg.metrum.ai/version

# Authenticated /v1/models and group smoke with a protected caller token file
# Ordinary-caller /metrics must return 403 metrics-forbidden
# Codex CLI + Claude Code CLI when routing or API compatibility changed
```

Record only safe scalars: HTTP status, version/digest, model-group count,
request ID, sanitized error class. Never retain tokens, token hashes, provider
keys, prompts, or full config. Append a dated note to `deployment.md` after
successful production changes.

## Rollback

### While the EC2 Compose host is retained (current window)

DNS already points at EKS. Emergency hostname rollback to Compose remains
available until EC2 decommission completes:

1. Restore DigitalOcean A records for `llm-api-engg.metrum.ai` and
   `llm-api.metrum.ai` to the retained EC2 public IP.
2. `docker compose start router caddy` on that host.
3. Do not run EC2 and EKS as concurrent writers for the same usage authority.

Application/config rollback on EKS (preferred for digest/config mistakes while
DNS stays on EKS): redeploy the previous known-good `approved_release_digest`
and runtime-bundle revision via `write-manifest --stage production` + signed
`create`.

### After EC2 decommission

DNS rollback to EC2 is retired. Rollback is:

1. Prior Fleet `approved_release_digest` + matching signed intent, and/or
2. SQLite recovery from an approved PVC/restic backup into a non-serving
   recovery path, then a separately authorized serving cutover.

Do not treat `customer delete` as routine rollback; delete is separately
approved tenant retirement.

## Decommission criteria (EC2 Compose)

Decommission the retained Compose host only when all are true:

1. Production aliases have served stably from EKS for the agreed rollback window.
2. Ownership transition to `metrum-production` / `instance-2278b384bf563c59df11`
   is complete and `ownership_transition` is removed from the profile.
3. Compose usage archive and restic snapshot evidence are retained
   (`scripts/archive_compose_usage.sh` already used at cutover).
4. Operators have rehearsed Fleet digest rollback and SQLite backup restore.
5. Change record authorizes instance stop/terminate; update this runbook and
   `deployment.md` to drop DNS-to-EC2 rollback language.

Until then: keep the EC2 instance available for emergency DNS rollback; keep
Compose router/Caddy stopped so they are not concurrent writers.

## TLS notes

- `*.apps.metrum.ai` wildcard does **not** cover `llm-api-engg.metrum.ai` or
  `llm-api.metrum.ai`.
- Alias TLS Secret `llm-api-metrum-ai-tls` in namespace `llm-api` (cert-manager
  DNS-01 preferred).
- After TLS or alias changes, redeploy or resume Fleet so hostname enablement
  reconciles Ingress rules.

## Production promotion record

Maintainer self-review authorizing Metrum production on Fleet (2026-09-01 UTC):

- Profile: `metrum-production` (`environment: production`)
- Tenant: `llm-api` / namespace `llm-api`
- Alias hostnames: `llm-api-engg.metrum.ai`, `llm-api.metrum.ai`
- Database: SQLite on tenant PVC (dedicated Postgres deferred)
- Code gate: `environment: production` accepted by Fleet profile validator
- Ingress: multi-host reconciliation in `EnableHostname`
- Evidence: `rtk go test ./internal/router/...`, `rtk make test-tenant-deploy-all`
- Cutover ledger: `deployment.md` (2026-09-01 section)

## Related

- `docs/CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md` — Fleet customer verbs; staging
  profile for disposable nonprod; production path for `llm-api` points here
- `docs/DOCKER_DEPLOYMENT.md` — customer Compose only
- `deployment.md` — dated evidence ledger (not an executable runbook)
- Historical stubs: `docs/EKS_STAGING_MIGRATION.md`,
  `docs/EKS_STAGING_CICD.md`, `docs/LLM_API_ENGG_EKS_CUTOVER_PLAN.md`,
  `docs/LLM_API_EKS_SQLITE_PARALLEL.md`, `docs/EKS_PRODUCTION_DISCOVERY.md`,
  `docs/PRODUCTION_PROMOTION_CONTRACT.md`
