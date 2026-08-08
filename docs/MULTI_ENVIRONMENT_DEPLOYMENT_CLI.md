# Fleet deployment lifecycle CLI

`metrum-fleetctl` is the only #555 deployment authority. It uses typed AWS and
Kubernetes clients; it never invokes `aws`, `kubectl`, Helm, Terraform, Make,
or a shell command. It owns one normalized deployment-job registry and only
the deterministic `plan`, idempotent `deploy`, exact-job `status`, and
separately approved `delete` operations.

`metrum-smartrouterctl` is a one-release compatibility binary. It reports the
rename to `metrum-fleetctl` and exits; it has no lifecycle behavior. Customer
operators use the separate `smartrouterctl` local operations CLI described in
the customer runbook.

Fleet commands are shipped in binary tarballs only. They are intentionally
absent from standard Router Docker/Compose images. Run them on a separate
trusted administration host.

## Authority and secret boundary

The manifest is reference-only. It never accepts or emits provider keys, raw
Router tokens or hashes, license payloads, DSNs, kubeconfigs, full
configuration, secret values, or raw adapter errors. Protected profiles carry
non-secret policy: account/region/cluster, immutable release digest, approved
profiles, storage or RDS sizing, and ingress policy. Profile files are
mode-`0600`; a live profile is resolved only from `aws-ssm:///`.

Every Fleet status is bounded scalar evidence: job/instance/profile IDs,
environment, region/cluster, namespace, release/config revision, lifecycle
state, safe error class, retryability, action progress, and hostname only once
activation succeeds. It never contains DSNs, endpoints other than the
activated caller hostname, credentials, references, or configuration.

## Non-production lifecycle

```bash
metrum-fleetctl plan \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --output json

metrum-fleetctl deploy \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --registry /protected/tenant-deployments.sqlite \
  --output json

metrum-fleetctl status \
  --profile-ref file:///protected/profile.yaml \
  --job job-<opaque-id> \
  --registry /protected/tenant-deployments.sqlite \
  --output json
```

The default path is SQLite state with exactly one Router container and one
replica; it does not provision or bind RDS. The ordered SQLite lifecycle is
namespace, network policy, runtime-secret binding, license binding, state PVC,
one-replica Router, activation, then hostname. An explicit approved
`database_profile` branch inserts dedicated private RDS after network policy
and before runtime-secret/DSN-reference binding. Hostname publication is
impossible before activation. Reuse the same intent to resume; use a new
immutable config revision/intent to reconcile the same instance. Unknown RDS
or PVC outcomes require operator reconciliation.

Deletion requires the matching manifest, intent, and a mode-`0600`, expiring,
job-bound approval. `retain_pvc` controls PVC retention; `retain_database`
controls dedicated-RDS retention. Deletion never guesses ownership.

```bash
metrum-fleetctl delete \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --registry /protected/tenant-deployments.sqlite \
  --confirm-file /protected/delete-approval.json \
  --output json
```

## Dedicated RDS contract

An optional protected non-production `database_mode: dedicated-rds` profile
permits exactly its approved `database_profile`; a manifest must explicitly
select that profile. SQLite remains the default when the manifest omits
`database_profile`. The RDS branch requires private subnet/security-group
policy, RDS-Proxy disabled, instance class, storage, backup retention, and
managed master-credential policy. The plan exposes only a deterministic
`database_id` and `database_profile`; it never stores or emits a DSN or credential.
`tenant_deployment_rds.go` uses typed AWS RDS calls to observe ownership tags
and policy, provision only private encrypted PostgreSQL, and delete only
owned instances with a final snapshot. It returns scalar evidence only.

The typed adapter is unreachable from the default EKS constructor. It can be
attached only through a validated, time-bounded non-production RDS admission;
the shipped Fleet CLI does not create an admission. RDS mutation therefore
remains fail-closed pending independent credential-binding, ownership,
activation, disposable E2E, security, and operations evidence. Production
profiles remain rejected until #518.

```bash
metrum-fleetctl databases status --profile-ref file:///protected/profile.yaml \
  --job job-<opaque-id> --registry /protected/tenant-deployments.sqlite
metrum-fleetctl smoke run activation --profile-ref file:///protected/profile.yaml \
  --job job-<opaque-id> --registry /protected/tenant-deployments.sqlite
```

Both commands are bounded status readbacks; they do not create resources or
activate configuration.

## Validation

```bash
rtk go test ./internal/router ./cmd/metrum-fleetctl -run TenantDeployment -count=1
```
