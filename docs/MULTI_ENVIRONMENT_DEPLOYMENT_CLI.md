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

The ordered lifecycle is namespace, network policy, runtime-secret binding,
license binding, state backend, one-replica Router, activation, then hostname.
Hostname publication is impossible before activation. Reuse the same intent to
resume; use a new immutable config revision/intent to reconcile the same
instance. Unknown durable outcomes require operator reconciliation.

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

A protected non-production profile can select `database_mode: dedicated-rds`
with an approved database profile, instance class, storage, and backup
retention. The plan exposes only a deterministic `database_id` and
`database_profile`; it never stores or emits a DSN or credential. Fake-adapter
tests cover plan, retry, retention, and safe status behavior.

The typed live EKS adapter currently fails closed with
`rds_live_admission_required`. It cannot provision RDS until independent
non-production RDS credential-binding, ownership, activation, disposable E2E,
security, and operations evidence is approved. Production profiles remain
rejected until #518.

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
