# Customer EKS lifecycle CLI

`metrum-smartrouterctl` is the only customer EKS lifecycle surface. It uses
typed AWS SDK and Kubernetes API clients; it never invokes `aws`, `kubectl`,
Helm, Terraform, Make, or a shell command.

- the #555 deployment job registry used by deterministic `plan`, idempotent
  `deploy`, exact-job `status`, and approved `delete`.

Both registries contain normalized scalar records only. They do not store
credentials, DSNs, Router tokens or hashes, provider keys, license payloads,
full configuration, kubeconfigs, or raw adapter errors. The CLI is shipped in
binary tarballs and is not included in the standard Docker or Docker Compose
image. Docker-based operators run it from an extracted binary package on a
separate trusted administration host.

## Real deployment lifecycle

`deploy` resolves a protected `aws-ssm:///` profile, verifies the selected
non-production EKS cluster, and uses an IAM-authenticated Kubernetes client.
It creates or resumes only resources labelled with its derived instance owner.
It refuses production profiles, foreign ownership, HPA, any replica count
other than one, and PVC modes other than `ReadWriteOnce`.

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
  approved_state_profile: sqlite-rwo-small
  storage_class: gp3
  state_storage_gib: 20
  ingress_class_name: nginx
  ingress_namespace: ingress-nginx
  tls_secret_name: shared-wildcard-tls
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
state_profile: sqlite-rwo-small
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

Create or resume the same EKS job:

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
stage. An unknown outcome after the PVC exists becomes `operator_required`; it
cannot resume until an operator establishes ownership. Exact safe retries use
normalized resource evidence instead of recreating completed resources. The
ordered adapter contract is namespace, network policy, runtime-secret binding,
license binding, PVC, one-replica Router workload, activation, then hostname.
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
choosing PVC retention:
The approval is recorded before cleanup begins, remains resumable while cleanup
is incomplete, and is consumed only after every non-retained resource reaches
`deleted`. A completed delete is an idempotent exact-job read.


```json
{
  "api_version": "metrum.ai/smartrouter-delete-approval/v1",
  "job_id": "job-<opaque-id>",
  "action": "delete",
  "expires_at": "<RFC3339 time within the next 24 hours>",
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
the state PVC is retained when the approval says so. A failed
delete becomes `operator_required` and never guesses ownership.

`plan` is read-only. `deploy` creates the namespace, policy, runtime and
license Secret bindings, a one-replica SQLite `ReadWriteOnce` PVC-backed
Deployment, then validates readiness before publishing the exact hostname
Ingress. `status` opens the job store read-only. `delete` requires a fresh,
job-bound approval and removes only owned resources; a PVC is retained unless
the approval explicitly opts out. There is no RDS provisioning, DSN, RDS proxy,
or quota lifecycle in this command.

## Validation and rollout

Run all credential-free local lifecycle suites:

```bash
rtk go test ./internal/router ./cmd/metrum-smartrouterctl -run TenantDeployment -count=1
```

The EKS E2E is intentionally opt-in and runs the packaged CLI against a
disposable non-production profile. It must be performed by an independently
authorized operator and record only resource IDs, states, and safe error
classes. The deployment registry remains separate from Router usage persistence.
