# Multi-environment deployment CLI safe contract

`metrum-smartrouterctl` contains one ADR-0012 control-plane contract with two
local relational registries:

- the established tenant inventory used by `register`, `observe-schema`,
  `quota-reserve`, and bounded drift `status`; and
- the #555 fake-first deployment job registry used by deterministic `plan`,
  idempotent `deploy`, exact-job `status`, and approved `delete`.

Both registries contain normalized scalar records only. They do not store
credentials, DSNs, Router tokens or hashes, provider keys, license payloads,
full configuration, kubeconfigs, or raw adapter errors. The CLI is shipped in
binary tarballs and is not included in the standard Docker or Docker Compose
image. Docker-based operators run it from an extracted binary package on a
separate trusted administration host.

## Fake-first deployment lifecycle

The shipped deployment lifecycle is intentionally `local-fake`. It proves the
manifest, plan, idempotency, state, retry, activation gate, status, retention,
and deletion contracts without contacting AWS, Kubernetes, RDS, DNS, a
provider, a license service, or a production host. A protected profile using
any scheme other than `file://` fails closed because live adapters are not yet
enabled.

The protected local profile must be a regular mode-`0600` YAML or JSON file. It
names the exact non-production account alias, region, cluster alias, namespace
prefix, hostname suffix, approved immutable release digest, resource profile,
and database profile. It contains policy, not credentials:

```yaml
api_version: metrum.ai/smartrouter-profile/v1
profile_id: approved-local
environment: nonproduction
account_alias: test-account
region: us-west-2
cluster_alias: test-cluster
namespace_prefix: router
hostname_suffix: apps.example.test
approved_release_digest: registry.example.test/router@sha256:<64-lowercase-hex>
approved_resource_profile: small
approved_database_profile: isolated-small
```

Caller intent is a strict reference-only YAML or JSON manifest. Unknown fields,
multiple documents, oversized documents, mutable image tags, unapproved
profiles, malformed references, and credential-shaped content are rejected.
Secret-manager references are accepted as inputs but are never copied into the
plan, job, attempts, resources, status, or CLI errors:

```yaml
api_version: metrum.ai/smartrouter-deployment/v1
customer_id: customer-a
stage: test
release: latest-approved
resource_profile: small
database_profile: isolated-small
upstream_config_ref: aws-secretsmanager:///smart-router/test/customer-a/upstreams
config_revision: revision-17
license:
  request_ref: aws-ssm:///smart-router/test/customer-a/license-request
  validity: 168h
```

Review a deterministic plan without creating the registry or applying DDL:

```bash
metrum-smartrouterctl plan \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --output json
```

Create or resume the same local fake job:

```bash
metrum-smartrouterctl deploy \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --registry /protected/tenant-deployments.sqlite \
  --output json
```

The deterministic lifecycle is `requested -> provisioning -> validating ->
ready`. A classified safe failure is `failed` and retryable from its exact
stage. An unknown adapter outcome after the dedicated database or state PVC
record exists becomes `operator_required`; it cannot resume until reconciliation
establishes ownership. Exact safe retries use normalized resource evidence
instead of recreating completed resources. The ordered fake adapter contract is
namespace, network policy, dedicated database, runtime-secret binding,
license binding, state PVC, Router workload, activation, then hostname.
Hostname publication cannot run until activation passes.
The plan exposes both a job ID and an instance ID. The job ID binds one intent
and desired manifest. The instance ID, namespace, hostname, and resource
ownership are derived from the protected profile, customer, and stage, so a new
config revision with a new intent reconciles the same isolated instance instead
of allocating a second namespace, database, or PVC. A superseded job cannot
delete resources now managed by a newer lifecycle.


Inspect exactly one job. This opens the existing registry read-only and never
creates an absent file:

```bash
metrum-smartrouterctl status \
  --profile-ref file:///protected/profile.yaml \
  --job job-<opaque-id> \
  --registry /protected/tenant-deployments.sqlite \
  --output json
```

Deletion requires the same manifest and intent plus a regular mode-`0600`
approval file bound to the exact job, expiring within 24 hours, and explicitly
choosing database and PVC retention:
The approval is recorded before cleanup begins, remains resumable while cleanup
is incomplete, and is consumed only after every non-retained resource reaches
`deleted`. A completed delete is an idempotent exact-job read.


```json
{
  "api_version": "metrum.ai/smartrouter-delete-approval/v1",
  "job_id": "job-<opaque-id>",
  "action": "delete",
  "expires_at": "<RFC3339 time within the next 24 hours>",
  "retain_database": true,
  "retain_pvc": true,
  "nonce": "approval-a"
}
```

```bash
metrum-smartrouterctl delete \
  --profile-ref file:///protected/profile.yaml \
  --manifest deployment.yaml \
  --intent-id intent-a \
  --registry /protected/tenant-deployments.sqlite \
  --confirm-file /protected/delete-approval.json \
  --output json
```

The hostname is disabled first. Resources are then removed in reverse order;
the database and state PVC are retained when the approval says so. A failed
delete becomes `operator_required` and never guesses ownership.

## Existing inventory commands

`register` records one isolated router instance plus one explicit dedicated RDS
allocation ID. Separate expected and independently observed schema versions are
required. Only `dedicated_instance` placement is accepted and RDS Proxy remains
disabled.

`observe-schema` updates only the current schema observation in an existing
registry. `quota-reserve` performs a fake local quota admission using a bounded
`rsv-<lowercase-canonical-uuid>` idempotency key. Inventory `status`, when
called without `--job` or `--profile-ref`, remains bounded to 1–100 rows and
read-only.

`promote` and `rollback` remain disabled. The fake-first deployment lifecycle
does not authorize or perform live resource creation, configuration update,
customer handoff, promotion, rollback, or cleanup. Live adapters require the
approved protected profile resolver, disposable non-production EKS E2E,
independent security/operations review, and explicit execution authorization.

## Validation and rollout

Run all credential-free local lifecycle suites:

```bash
rtk make test-tenant-deploy-all
```

The component targets are `test-tenant-deploy-contract`,
`test-tenant-deploy-adapters`, `test-tenant-deploy-security`, and
`test-tenant-deploy-activation`. The activation suite uses a local mock API to
verify readiness, authenticated model and chat access, usage visibility, admin
metrics access, and ordinary-caller `/metrics` `403`. It does not constitute
provider-backed or EKS evidence.

Rollback this slice by restoring the previous binary and preserving the private
SQLite registry for inspection. Do not connect live AWS, Kubernetes, DNS, RDS,
license, Secret, or provider adapters until the remaining gates pass. This
registry remains separate from Router usage persistence and #507 migrations.
