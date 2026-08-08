# Customer Router Instance Operations Runbook

> **Internal runbook.** `metrum-fleetctl` is a binary-package-only Fleet lifecycle tool; `smartrouterctl` is the customer-local operations CLI and is included in Docker. This document grants no AWS, EKS, RDS, DNS, production, runtime-secret, credential, or Compose-to-EKS mutation authority.

## Purpose and ownership

Use this runbook to operate one customer router instance through onboarding, configuration/release updates, status inspection, activation, recovery, and eventual retirement.

At launch, one customer router instance maps to one isolated runtime identity and namespace, one approved state backend (single-writer PVC SQLite or an explicitly approved dedicated RDS profile), one Router replica, and one deployment-defined environment and region. Account and region are explicit profile data; no command, registry record, or fixture may infer a default region or derive a resource address from a customer name.

| Owner | Responsibility |
| --- | --- |
| Customer administrator | Supplies approved upstream/BYOK information through the protected onboarding path and accepts the activated instance. |
| Commercial/control-plane owner | Verifies entitlement and creates the authorized provisioning intent. #545 owns this durable customer job. |
| Platform operator | Uses the binary-package-only `metrum-fleetctl` lifecycle when its independent non-production gates permit it. It resolves approved AWS/EKS policy, applies only instance-owned Kubernetes resources, and publishes ingress only after activation. |
| Infra/Security approver | Approves account/region, network, KMS, IAM, durability, quota, DNS, and change-control policy before live execution. |
| Release approver | Owns protected production-like rehearsal, change window, canary/cutover, and recovery authorization under #518. |

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

Issue #555 has one strict reference-only manifest, one normalized deployment-job registry, and typed AWS/EKS contracts in `metrum-fleetctl`. It provides deterministic plan, idempotent ownership-safe create/resume, classified state, activation-before-hostname, bounded status, explicit PVC/RDS retention, and exact-job deletion. Customer-local `smartrouterctl` has no Fleet, cloud, cross-customer, config-activation, key-rotation, or license-signing authority.

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

Fleet remains blocked for live dedicated-RDS provisioning. The fake lifecycle
covers retry and retention; the typed live adapter returns
`rds_live_admission_required` until independent non-production RDS
credential-binding, ownership, activation, disposable E2E, security, and
operations evidence is accepted. Production profiles remain rejected until
#518. See [the Fleet lifecycle contract](MULTI_ENVIRONMENT_DEPLOYMENT_CLI.md)
and [CLI boundary ADR](ADR_FLEET_AND_CUSTOMER_CLI_BOUNDARIES.md).

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
<file-ref> --job <exact-job>` opens the existing local job registry read-only
and returns one safe lifecycle record. Neither mode probes Router health,
Kubernetes, AWS, RDS, DNS, credentials, or customer databases.

The future live status adapter must also be authorized and scoped to an
explicit profile/environment/instance. It must answer:


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
