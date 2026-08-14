# Customer Router Instance Operations Runbook

> **Internal runbook.** `metrum-genai-smartrouter-fleetctl` is a binary-package-only Fleet lifecycle tool; `metrum-genai-smartrouterctl` is the customer-local operations CLI and is included in Docker. This document grants no AWS, EKS, RDS, DNS, production, runtime-secret, credential, or Compose-to-EKS mutation authority.

## Purpose and ownership

Use this runbook to operate one customer router instance through onboarding, configuration/release updates, status inspection, activation, recovery, and eventual retirement.

At launch, one customer router instance maps to one isolated runtime identity and namespace, one approved state backend (single-writer PVC SQLite or an explicitly approved dedicated RDS profile), one Router replica, and one deployment-defined environment and region. Account and region are explicit profile data; no command, registry record, or fixture may infer a default region or derive a resource address from a customer name.

| Owner | Responsibility |
| --- | --- |
| Customer administrator | Supplies approved upstream/BYOK information through the protected onboarding path and accepts the activated instance. |
| Commercial/control-plane owner | Verifies entitlement and creates the authorized provisioning intent. #545 owns this durable customer job. |
| Platform operator | Uses the binary-package-only `metrum-genai-smartrouter-fleetctl` lifecycle when its non-production gates permit it. It resolves approved AWS/EKS policy, applies only instance-owned Kubernetes resources, and publishes ingress only after activation. |
| Infra/Security approver | Approves account/region, network, KMS, IAM, durability, quota, DNS, and change-control policy before live execution. |
| Release approver | Owns protected production-like rehearsal, change window, canary/cutover, and recovery authorization under #518. |

These are responsibilities, not headcount. A single-maintainer deployment may
hold every non-production role, and the same person may implement and review the
change. The obligation that survives is the recorded evidence in [Recorded
security and operations review](#recorded-security-and-operations-review), not a
second signature. Split the roles across separate people and additional scoped
EKS roles when more than one qualified person is available or a customer
contract requires separation of duties. Production cutover authorization stays
with #518 regardless of team size.

Human authorization is role-based rather than username-based. Any user whose
organization-controlled federated identity is assigned the approved operator
role may configure a local AWS profile that assumes the protected deployment
role and use the same lifecycle commands. The CLI accepts the profile name, not
credentials, and independently verifies the exact account, assumed-role name,
protected target policy, immutable runtime/image attestation, fail-closed
admission policy and binding, EKS access entry, and least-privilege RBAC before
cluster selection or mutation. The human role has no Secret, admission-policy
mutation, or resource-deletion authority; destructive cleanup remains a
separate approved bootstrap/recovery action. Removing the user's
identity-provider assignment or the source role's exact `sts:AssumeRole` grant
revokes access without changing the
CLI or customer instance. See the credential-free profile and verification
procedure in [EKS staging migration](EKS_STAGING_MIGRATION.md#one-time-authorization-bootstrap).

Issue #555 has one strict signed reference-only deployment intent, one normalized GORM+SQLite deployment-job registry with tenant/license inventory tables, and typed AWS/EKS contracts in `metrum-genai-smartrouter-fleetctl`. It provides deterministic plan, idempotent ownership-safe create/resume, classified state, activation-before-hostname, bounded exact-job status, registry-local `tenants`/`licenses` inventory, explicit PVC/RDS retention, and exact-job deletion. Live `plan`, `deploy`, and `delete` load the mode-`0600` signed intent, whose protected `aws-ssm:///` profile reference is authenticated before cloud access; `file://` is limited to the local fake `plan` contract. Deployment `status` accepts an exact job plus protected profile reference. Customer-local `metrum-genai-smartrouterctl` has no Fleet, cloud, cross-customer, config-activation, key-rotation, or license-signing authority.

The Fleet-only manifest carries `runtime_bundle_ref`, not raw runtime files. It
is an `aws-ssm:///` or `aws-secretsmanager:///` reference without query data.
The resolver reads the protected JSON bundle only in memory; it must contain
exactly `config.yaml` as a YAML mapping and `env.json` as a JSON string map.
The owned `router-runtime` Secret contains only those keys and is mounted
read-only at `/app/config`; `metrum-genai-smartrouter-license` remains separate. No bundle
value or reference belongs in a plan, registry, status, error, ticket, or
evidence record.

## Before any live action

The following are mandatory fail-closed preflight conditions. An absent condition means record a safe blocked status in the #555 durable provisioning job and escalate to the commercial/control-plane owner. That owner coordinates the listed Infra/Security or Release approver when the missing gate requires their decision; operators must not bypass the gate manually, select shared placement, or reuse another customer's resources.

| Required gate | Evidence to record safely |
| --- | --- |
| Approved runtime profile | Protected profile revision; explicit cloud-account reference, region, environment, instance alias, allowed domain policy, EKS target, and secret references. |
| Authorized identity | Read-only proof of the dedicated least-privilege IAM/runtime identity and namespace authorization; never use a root credential. |
| Workload admission | Exact reviewed policy/binding spec hashes, deny-on-missing parameter, immutable approved router/sidecar image attestation, representative rejected bypasses, and proof the delivery role cannot mutate policy state or delete durable resources. |
| Supply-chain and release inputs | Immutable image digest, reviewed config revision, compatible license revision, and previous known-good manifest/evidence reference. |
| Data compatibility | #507 forward-only migration compatibility and an approved recovery plan; no automatic database rollback. |
| Capacity | Reservation and fresh recheck for the explicit account+region. Cross-region automated backups are disabled at launch; no DR-copy reservation, destination region, or copy KMS key is assumed. |
| Network and crypto | Private-only RDS endpoint, approved subnets/security groups, regional KMS key reference, TLS-only DB access, least-privilege database/runtime roles, and secret-manager references only. |
| Durability | Explicit same-region automated-backup/PITR or explicitly approved no-automated-backup policy; recovery-point class, retention, deletion/final-snapshot behavior, and restore expectations. |
| Activation and promotion | Applicable #517 non-production evidence, #554 activation profile/evidence, and #518 approval for protected production-like promotion. |
| Public/customer traffic | #13 security remediation and the approved public/customer release gate. |

RDS Proxy is disabled at launch. Cross-region backup is never implicit: it is disabled at launch and requires a later approved destination account/region profile, destination-region KMS policy, copy grant, account-wide capacity reservation, retention, and recovery evidence. A future proxy or shared database placement also requires an approved ADR, policy, adapter tests, and a separate rollout decision.

## First disposable E2E admission

The first disposable non-production EKS/RDS E2E is the narrow exception to the
post-E2E review ordering. After #818's least-privilege preflight passes, an
authorized qualified maintainer may obtain an out-of-band, mode-`0600` RDS
admission document for that one E2E. The maintainer may also be the
implementer and later self-reviewer. This is not a second review, an approval
chain, a new CLI verb, or a production authorization.

Run the existing side-effect-free `metrum-genai-smartrouter-fleetctl plan` first. The external
document must use `api_version:
metrum.ai/smartrouter-rds-admission/v1`, `action: disposable-e2e`, and bind
only the plan's `profile_id`, non-production `environment`,
`database_profile`, `job_id` (which is derived from the exact intent),
`namespace`, and `manifest_sha256`. It also carries a safe issuer-role alias,
opaque approval ID, UTC approval and expiry timestamps within 24 hours, and an
Ed25519 signature. It contains no credentials, DSN, endpoint, secret
reference, runtime configuration, license payload, or signing key.

The approved profile supplies only the non-secret
`lifecycle_approval_public_key` that verifies the document. `metrum-genai-smartrouter-fleetctl`
never creates, updates, prints, or persists either an admission or signing
material. It consumes `--rds-admission-file` only after validating private
file mode, strict JSON schema, non-secret content, signature, exact plan
binding, and expiry. A dedicated-RDS `deploy` requires it before the lifecycle
registry or AWS/EKS clients are opened. A `delete` that deletes the dedicated
RDS requires the same still-valid admission in addition to its existing
job-bound deletion approval. Invalid, stale, unsigned, or differently scoped
documents leave the typed RDS adapter unattached.

### Admission issuance under a single maintainer

One qualified maintainer may hold multiple non-production responsibilities, but
each is exercised through a separately scoped federated session:

| Session | Permitted activity | Must not hold |
| --- | --- | --- |
| Delivery | Read the approved SSM profile, complete the #818 preflight, and run the existing Fleet lifecycle command. | Admission-signing key or service authority; policy/RoleBinding mutation; general resource deletion. |
| Admission issuer | Verify the safe deterministic plan fields and produce one signed, mode-`0600` admission outside Fleet. | EKS/RDS delivery, runtime-secret, registry, or production-cutover authority. |
| Reviewer | Record the checklist evidence after the first E2E. | Any additional signature requirement merely because the reviewer is also the implementer. |

The same person may use these roles in sequence. The admission issuer can be a
protected signing service or an approved isolated signing workflow; the private
key never enters the Fleet process, manifest, profile, registry, logs, or
evidence. Record only the issuer-role alias and the admission's opaque ID and
digest. This preserves an external, independently constrained admission
without introducing a second approver or a new Fleet command.

The authorized sequence is: #818 repair and passing preflight; local suite
evidence and deterministic plan; external scoped admission; disposable E2E
including failure/retry and confirmed cleanup; then the recorded
single-reviewer review; then the separately authorized production-like
non-production rehearsal. #518 remains the sole production-cutover authority.

## Recorded security and operations review

Live non-production EKS/RDS mutation under #555 needs one qualified reviewer,
not an approval chain. One maintainer may review their own change. Except for
the narrowly admitted first disposable E2E above, the written review is created
after the evidence exists and before a production-like non-production rehearsal.
A review that cannot cite the items below fails, and an unreviewed rehearsal
stays forbidden.

Record these safe scalar values in the #555 durable provisioning job:

| Review item | Recorded evidence |
| --- | --- |
| Reviewer and time | Reviewer identity or role alias, UTC review timestamp, and whether the reviewer also implemented the change. |
| Target scope | Approved profile ID, environment, account/region reference, cluster reference, namespace prefix, hostname, customer/instance/intent IDs, and explicit non-production classification. |
| Immutable inputs | Resolved `repository@sha256:...` digest, config revision, license revision, and the source commit that produced the package. |
| Local gates | Passing contract, fake-adapter, security, and activation suite results with counts and run timestamps. |
| Disposable proof | Disposable non-production EKS E2E result, including the injected failure/retry case and confirmed cleanup. |
| Isolation checks | Namespace/RBAC/NetworkPolicy denial, cross-namespace denial, wrong-Host and default-backend denial, and ordinary-caller `/metrics` `403 metrics-forbidden`. |
| Secret handling | Confirmation that runtime bundle values, DSNs, credentials, tokens, token hashes, and license payloads are absent from argv, plans, registry rows, statuses, logs, and evidence. |
| Data decision | PVC and dedicated-RDS retention decision, backup/PITR class, rollback trigger, and who may approve cleanup. |
| Outcome | Approved, approved with conditions, or rejected, plus the exact conditions and the next authorized action. |

Never record credentials, DSNs, kubeconfigs, raw router tokens, token hashes,
license payloads, full Router configuration, prompts, or upstream response
bodies in the review. Add a second reviewer only when another qualified person
is available or a customer contract requires separation of duties. Production
cutover requires #518's stricter protected gates in addition to this review.

## Existing EKS staging repair lifecycle

The current Metrum staging deployment is an integration target, not a customer
instance provisioned by the customer #555 CLI. Its canonical live repair
procedure is [EKS staging migration runbook: Staging Repair And Validation
Lifecycle](EKS_STAGING_MIGRATION.md#staging-repair-and-validation-lifecycle).
Use that procedure in order: open a change record; establish the exact
MFA/federated assumed role; capture the pre-repair baseline; run protected
preflight and plan; classify the failure; reconcile only reviewed desired
state; restore the selected ingress/Linkerd boundary; run authenticated API,
CLI, metrics-isolation, and relational-usage smokes; sanitize and preserve
asciinema evidence; review and announce; then roll back or clean up.

Do not use a previously successful smoke as present readiness evidence. As of
2026-08-07, the reviewed staging delivery role passed `make eks-preflight`, and
the public readiness endpoint returned HTTP 200 with `ok=true`. This is
staging-only repair evidence, not a disposable customer EKS proof for #555:
customer handoff and production authorization remain gated on the approved
customer profile, disposable E2E, and the complete activation record. Keep
exact API-surface compatibility evidence current; #856, #857, and #858 track
the outstanding Anthropic-text and Codex-tool eligibility/retry findings.

## Onboard a new customer router instance

The customer-local CLI is intentionally separate from Fleet authority:

```bash
smartrouterctl config validate --config /etc/smart-llmrouter/config.yaml
smartrouterctl config diff --from current.yaml --to candidate.yaml
smartrouterctl callers generate --owner-user example-admin --project example \
  --allow <group-from-v1-models> --token-out /protected/caller.token
smartrouterctl status --config /etc/smart-llmrouter/config.yaml
smartrouterctl license status --config /etc/smart-llmrouter/config.yaml
smartrouterctl models list --config /etc/smart-llmrouter/config.yaml
smartrouterctl usage summary --config /etc/smart-llmrouter/config.yaml
```

`callers generate` creates a token once in a new mode-`0600` file and returns
only safe caller metadata. It refuses an existing output path and never prints
the raw token or token hash. The output requires the approved configuration
controller to activate; the CLI cannot activate config, rotate keys, sign
licenses, access AWS/EKS/RDS, or operate another customer.

The default customer deployment is SQLite state with exactly one Router
container and one replica; it does not provision or bind RDS. Dedicated RDS is
optional and requires an explicit approved `database_profile` manifest branch.
Omit `database_profile` for SQLite even when the protected profile's
`database_mode` is `dedicated-rds`.

Protected profiles also carry `approved_compute_profiles`. Manifests may set
optional `compute_profile` (default `t3a.medium`). That name selects Kubernetes
scheduling/resource policy only; it is not free-form EC2/`instance_type`
mutation and does not create node groups. Exact-job
`metrum-genai-smartrouter-fleetctl status` reports customer/instance ownership and compute
scalars for Fleet-labelled objects only
(`app.kubernetes.io/managed-by=metrum-fleetctl` and
`metrum.ai/smartrouter-instance=<instance_id>`). Status is not cluster inventory.
Cleanup of disposable customers uses signed `metrum-genai-smartrouter-fleetctl delete` (or the
lifecycle helper) with a fresh job-bound approval—never direct kubectl.

For repeatable non-production SQLite customer instances (`acme3`, `acme4`, …)
use `metrum-genai-smartrouter-fleetctl customer` from a release binary package
`bin/` directory on `PATH` (or set `METRUM_FLEET_BIN_DIR`). Sibling binaries
`metrum-genai-smartrouter-fleet-sign` and `router-token-gen` must be available
beside it. Do not `go build` / `go run` on operator hosts; packaged CLIs are
binaries only. The former Python helpers
`scripts/fleet_customer_lifecycle.py` and
`scripts/fleet_sqlite_customer_deploy.py` are one-release rename notices only.
Default create uses the production-identical runtime bundle (same
upstream provider keys as the Metrum reference) and omits `database_profile`
so Fleet stays on SQLite + `auto-safe` rewrite at secret bind time.

```bash
# Create (hostname https://{id}.apps.metrum.ai)
metrum-genai-smartrouter-fleetctl customer create --customer-id acme4

# Status / smoke
metrum-genai-smartrouter-fleetctl customer status --customer-id acme4
metrum-genai-smartrouter-fleetctl customer smoke --customer-id acme4

# Grant a caller and activate it (Fleet publishes per-customer SM bundle + redeploy)
metrum-genai-smartrouter-fleetctl customer grant-caller --customer-id acme4 \
  --owner-user acme-admin --project acme --allow high \
  --token-out ~/.local/share/metrum-fleet/acme4/CALLER_TOKEN_ADMIN.txt

# Update live config (YAML patch merged into runtime bundle, then redeploy)
metrum-genai-smartrouter-fleetctl customer update-config --customer-id acme4 \
  --patch-file /protected/acme4-config-patch.yaml

# Delete (signed Fleet delete; SQLite path; no kubectl)
metrum-genai-smartrouter-fleetctl customer delete --customer-id acme4
```

Optional recreate helper:

```bash
metrum-genai-smartrouter-fleetctl customer create --customer-id acme4 --delete-first
```

`metrum-genai-smartrouterctl callers generate` remains the customer-local draft tool and
returns `activation: configuration-controller-required`. On Metrum-managed EKS
SQLite customers, Fleet `customer grant-caller` / `customer update-config` is the
configuration controller: it writes
`aws-secretsmanager:///smartrouter/fleet/customers/<id>/runtime-bundle` with the
operator IAM identity (CreateSecret/PutSecretValue), then deploys with the Fleet
lifecycle role (read + EKS). Do not use kubectl or one-off migrate Jobs.

Artifacts stay under `~/.local/share/metrum-fleet/<customer_id>/` (mode `0700`).

The typed AWS RDS adapter enforces private/encrypted/no-proxy policy, ownership
tags, and final-snapshot deletion. Its default EKS constructor remains
fail-closed; the first disposable E2E may attach it only through the external
time-bounded admission described in [First disposable E2E
admission](#first-disposable-e2e-admission). The Fleet CLI consumes but never
creates that document. Credential-binding, ownership, activation, disposable
E2E, security, and operations evidence is then recorded through [Recorded
security and operations review](#recorded-security-and-operations-review).
Production profiles remain rejected until #518. See [the Fleet lifecycle
contract](MULTI_ENVIRONMENT_DEPLOYMENT_CLI.md) and [CLI boundary
ADR](ADR_FLEET_AND_CUSTOMER_CLI_BOUNDARIES.md).

On any live failure, stop customer handoff. Retry only the classified safe
stage. Compensation or cleanup is separately authorized and must not delete an
RDS instance, snapshot, PVC, or customer data by default.

## Update configuration or release

Each proposed change is one instance-scoped, explicit operation. The release manifest binds the instance/profile revision, immutable image digest, configuration revision, license compatibility, PVC-backed state policy, prior known-good manifest, and approval/evidence references. It contains no credentials, endpoints, DSNs, or full configuration.

1. Create a reviewed draft configuration using protected secret references. Validate YAML/contract shape and caller/model access; keep new provider/model/API-skin combinations in a smoke or staging group until exact request-shape validation passes.
2. Render and plan against the named profile/environment/instance. Confirm the target identity, manifest diff, configuration fingerprint, migration class, rollback classification, and expected caller-visible impact.
3. Run exact-surface staging checks: readiness, allowed `/v1/models`, applicable Chat/Responses/Messages, streaming, tools, image behavior, quota/license behavior, report/admin authorization, and ordinary-caller `/metrics` `403`.
4. Promote only through the approved source-to-target manifest/evidence handoff. A config or application rollback returns to a known-good manifest; it never implies a database/data rollback.
5. Record sanitized outcome, release/config/migration versions, safe error class, and next action. Keep raw request bodies, provider keys, router tokens, token hashes, customer content, and full config out of evidence.

## Backup and recovery requests

Do not use the phrase "backup to a cluster." An EKS cluster is a runtime placement, not a database-backup destination. The system keeps three separate artifacts with separate retention and authorization:

| Artifact | Canonical owner | Contents and recovery behavior |
| --- | --- | --- |
| Configuration backup | #7 and #545 | Versioned configuration, safe secret references, entitlement-compatible metadata, and audit linkage. It never contains raw BYOK, caller tokens/hashes, DSNs, or Kubernetes Secrets. Restore creates a draft and requires revalidation. |
| Tenant PVC recovery record | #555 | A protected record for one tenant's retained SQLite PVC. It is not a raw database download or tenant cloud credential. |
| Release manifest | #581 and #517 | Immutable image/config/license/migration and safe evidence references. It is a release record, not a data backup. |

### Operator recovery primitive (planned #581)

An operator may create or inspect a tenant RDS recovery point only with an explicit tenant, source router-instance, account/region/environment profile, backup class, idempotency/intent ID, and approved policy reference. The future primitive must fail closed for an unapproved profile, capacity/retention violation, tenant mismatch, unknown source instance, or missing authorization.

A restore never overwrites a serving tenant state volume. It names an explicit **recovery router instance** for the same tenant and an approved retained PVC recovery record. Arbitrary EKS clusters, arbitrary PVCs, cross-tenant targets, and unapproved account/region targets are rejected. The result starts as a non-serving `recovery_candidate` with ingress and caller handoff disabled.

The candidate then follows #507-compatible migration/compatibility checks, #7 configuration reconciliation as a draft where required, and #554 activation verification. Only a separately authorized promotion/cutover can make it serving. Database recovery is never an automatic rollback or a silent replacement of live customer data.

### Tenant-admin request boundary (planned #545 child work)

An eligible customer owner/admin may request or inspect a per-tenant recovery point through the control plane, subject to entitlement, configured on-demand limit, retention, cost, and audit policy. The portal never exposes AWS, Kubernetes, database, KMS, snapshot, endpoint, DSN, or credential access.

An admin may request a restore, but the launch default is a non-serving recovery candidate. A customer request cannot self-cut over a serving instance. Final cutover follows the configured protected approval rule and #518 production authority. #555 owns the durable restore/recovery job, secret/config/license/namespace/ingress wiring, and compensation; #554 supplies the required revalidation evidence.

Store policy, requests, attempts, immutable artifact references, restore approvals, recovery targets, retention/hold/cleanup events, and audit evidence relationally with tenant, source instance, profile scope, actor, and policy-revision foreign keys. Status surfaces only safe scalar references, timestamps, state, and error class.

## Inspect status and verify service

The shipped command has two read-only status modes. Inventory `status` remains
bounded to 1–100 local rows and reports tenant/stage identity, schema drift,
release digest, and dedicated placement. Deployment `status --profile-ref
aws-ssm:///... --job <exact-job>` accepts only a protected profile reference,
opens the exact local job registry read-only, and performs scoped read-only EKS
observation for that profile. Neither mode reads Router health, RDS state,
credentials, configuration, or customer databases.

The status adapter is authorized and scoped to an explicit
profile/environment/instance. It answers:


| Question | Safe status/evidence |
| --- | --- |
| What is deployed? | Instance alias, environment/region aliases, manifest/release/config revision, and migration version. |
| Is it serving? | Readiness, bounded uptime/observed timestamp, activation state, safe DNS/ingress health, and sanitized error class. |
| Is the database aligned? | Dedicated-DB connectivity summary, desired versus last-observed migration state, observation time, and drift count. |
| Is policy satisfied? | Profile revision, `ReadWriteOnce` PVC storage policy, one-replica enforcement, and retained-state recovery policy. |
| What should be checked next? | Latest approved evidence reference, required verification, and a safe next action or escalation. |

For a newly activated instance, verify:

- readiness and version endpoint;
- authenticated `/v1/models` for each caller class;
- the API surfaces and modalities promised to that customer;
- tenant/network isolation and ordinary-caller `/metrics` `403`;
- report/admin authorization and a sanitized usage/latency evidence window;
- license state and configuration fingerprint;
- the approved durability/recovery rehearsal and activation evidence.

## Failure, rollback, and recovery

Treat application/config rollback and database/data recovery as separate operations.

- For release/config failure, stop expansion, retain sanitized evidence, and return only application/config traffic to the previous known-good manifest after approval.
- For migration, RDS, secret, DNS, Linkerd, activation, backup, or restore failure, halt handoff. Retry only an idempotent safe stage; otherwise use the approved restore/reconcile plan.
- Never automatically drop a database, delete a snapshot, revoke a customer instance, or infer that a failed deploy can undo durable data.
- After a rollback or recovery, rerun readiness, caller API, isolation, license, activation, and reporting checks before returning the instance to `ready`.
- Escalate an unknown state, failed recovery, policy gap, or authorization denial with the issue number, safe evidence, impact, and the exact decision needed.

## Promotion and retirement boundary

#581 preflights and records source-to-target manifest/evidence handoff. #518 alone authorizes protected production rehearsal, change windows, canary/cutover, and protected rollback. Routine customer onboarding does not grant production cutover authority.

Customer retirement or incident cleanup requires a separately confirmed plan covering customer notification, traffic disablement, retention, license/caller access, and RDS/snapshot disposition. Do not treat a normal failed provisioning attempt as authorization to delete durable resources.

## Related records

- [#555 customer EKS lifecycle](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/555)
- [#555 customer provisioning orchestration](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/555)
- [#507 forward-only migration framework](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/507)
- [#592 tenant-admin protected RDS recovery requests](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/592)
- [License Operations Runbook](LICENSE_OPERATIONS.md)
- [Deployment Runbook](DEPLOYMENT.md)
- [Production Runbook](PRODUCTION_RUNBOOK.md)
