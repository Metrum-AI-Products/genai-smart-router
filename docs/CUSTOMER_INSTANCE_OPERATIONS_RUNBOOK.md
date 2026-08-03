# Customer Router Instance Operations Runbook

> **Source-only internal runbook.** The operator CLI described here is the planned #581 contract, not a shipped command surface. This document does not authorize AWS, EKS, RDS, DNS, or production mutations. Until the policy gates and implementation evidence below are complete, use the approved existing deployment runbooks for live operations.

## Purpose and ownership

Use this runbook to operate one customer router instance through onboarding, configuration/release updates, status inspection, activation, recovery, and eventual retirement.

At launch, one customer router instance maps to one isolated runtime identity and namespace, one dedicated private PostgreSQL RDS instance, and one deployment-defined environment and region. Account and region are explicit profile data; no command, registry record, or fixture may infer a default region or derive a resource address from a customer name.

| Owner | Responsibility |
| --- | --- |
| Customer administrator | Supplies approved upstream/BYOK information through the protected onboarding path and accepts the activated instance. |
| Commercial/control-plane owner | Verifies entitlement and creates the authorized provisioning intent. #545 owns this durable customer job. |
| Platform operator | Uses the future #581 reusable deploy/status/promote/rollback primitives only with a reviewed runtime profile and explicit confirmation. |
| Infra/Security approver | Approves account/region, network, KMS, IAM, durability, quota, DNS, and change-control policy before live execution. |
| Release approver | Owns protected production-like rehearsal, change window, canary/cutover, and recovery authorization under #518. |

#581 owns the reusable operator primitives and RDS lifecycle contract. #555 consumes them inside customer provisioning; it remains responsible for the customer job, license, secrets/BYOK, DNS/TLS, namespace/Linkerd/ingress, compensation, and handoff. The #507 migration ledger remains the per-database source of truth.

## Before any live action

The following are mandatory fail-closed preflight conditions. An absent condition means record a safe blocked status and escalate through the sprint planner; do not bypass it manually, select shared placement, or reuse another customer's resources.

| Required gate | Evidence to record safely |
| --- | --- |
| Approved runtime profile | Protected profile revision; explicit cloud-account reference, region, environment, instance alias, allowed domain policy, EKS target, and secret references. |
| Authorized identity | Read-only proof of the dedicated least-privilege IAM/runtime identity and namespace authorization; never use a root credential. |
| Supply-chain and release inputs | Immutable image digest, reviewed config revision, compatible license revision, and previous known-good manifest/evidence reference. |
| Data compatibility | #507 forward-only migration compatibility and an approved recovery plan; no automatic database rollback. |
| Capacity | Reservation and fresh recheck for the explicit account+region. Cross-region automated backups are disabled at launch; no DR-copy reservation, destination region, or copy KMS key is assumed. |
| Network and crypto | Private-only RDS endpoint, approved subnets/security groups, regional KMS key reference, TLS-only DB access, least-privilege database/runtime roles, and secret-manager references only. |
| Durability | Explicit same-region automated-backup/PITR or explicitly approved no-automated-backup policy; recovery-point class, retention, deletion/final-snapshot behavior, and restore expectations. |
| Activation and promotion | Applicable #517 non-production evidence, #554 activation profile/evidence, and #518 approval for protected production-like promotion. |
| Public/customer traffic | #13 security remediation and the approved public/customer release gate. |

RDS Proxy is disabled at launch. Cross-region backup is never implicit: it is disabled at launch and requires a later approved destination account/region profile, destination-region KMS policy, copy grant, account-wide capacity reservation, retention, and recovery evidence. A future proxy or shared database placement also requires an approved ADR, policy, adapter tests, and a separate rollout decision.

## Onboard a new customer router instance

This is the target workflow for the future control plane. It is intentionally a checklist rather than command syntax until #581 ships with tested interfaces.

1. Verify the commercial entitlement and record an explicit, authorized provision intent. Select an approved account+region, environment, customer instance alias, domain policy, and deployment template. Do not put customer identifiers, provider keys, licenses, or full config in GitHub evidence.
2. Create or resume the #555 durable provisioning job. Its first read-only status must identify the selected profile revision and safe intent reference.
3. Run the #581 preflight against exactly that profile and instance. It must reject unknown capacity, an unavailable dedicated placement, missing policy evidence, unapproved namespace access, or an incompatible migration/release.
4. Reserve capacity transactionally and create the isolated runtime and dedicated RDS only after the fresh pre-create check passes. Every retry is idempotent and scoped to the instance record; an incomplete run must report a classified safe state instead of guessing whether to create again.
5. Have #555 deliver customer secrets/BYOK through the secret manager, attach the instance-bound license, configure DNS/TLS and namespace/Linkerd/ingress, and retain only references in status evidence.
6. Run the protected sandbox/activation path. #554 must produce passing, versioned activation evidence before the job becomes `ready`, before public ingress is enabled, or before a caller credential is handed off.
7. Notify the customer administrator with the approved instance URL and protected credential-delivery path. Do not include raw credentials, provider keys, license files, or complete configuration in notification or status output.

On any failure, stop the customer handoff. Retry only the classified safe stage. Compensation or cleanup is separately authorized and must not delete an RDS instance, snapshot, or customer data by default.

## Update configuration or release

Each proposed change is one instance-scoped, explicit operation. The future release manifest binds the instance/profile revision, immutable image digest, configuration revision, license compatibility, migration set, dedicated-RDS reference, prior known-good manifest, and approval/evidence references. It contains no credentials, endpoints, DSNs, or full configuration.

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
| Tenant RDS recovery point | #581 | An immutable recovery-point/snapshot reference for one tenant's dedicated RDS. It is not a raw database download or tenant cloud credential. |
| Release manifest | #581 and #517 | Immutable image/config/license/migration and safe evidence references. It is a release record, not a data backup. |

### Operator recovery primitive (planned #581)

An operator may create or inspect a tenant RDS recovery point only with an explicit tenant, source router-instance, account/region/environment profile, backup class, idempotency/intent ID, and approved policy reference. The future primitive must fail closed for an unapproved profile, capacity/retention violation, tenant mismatch, unknown source instance, or missing authorization.

A restore never overwrites a serving tenant database. It names an immutable recovery-point ID and an explicit **recovery router instance** for the same tenant. The registry resolves the target to a new dedicated private RDS allocation in an allowed profile; arbitrary EKS clusters, arbitrary RDS instances, cross-tenant targets, and unapproved account/region targets are rejected. The result starts as a non-serving `recovery_candidate` with ingress and caller handoff disabled.

The candidate then follows #507-compatible migration/compatibility checks, #7 configuration reconciliation as a draft where required, and #554 activation verification. Only a separately authorized promotion/cutover can make it serving. Database recovery is never an automatic rollback or a silent replacement of live customer data.

### Tenant-admin request boundary (planned #545 child work)

An eligible customer owner/admin may request or inspect a per-tenant recovery point through the control plane, subject to entitlement, configured on-demand limit, retention, cost, and audit policy. The portal never exposes AWS, Kubernetes, database, KMS, snapshot, endpoint, DSN, or credential access.

An admin may request a restore, but the launch default is a non-serving recovery candidate. A customer request cannot self-cut over a serving instance. Final cutover follows the configured protected approval rule and #518 production authority. #555 owns the durable restore/recovery job, secret/config/license/namespace/ingress wiring, and compensation; #554 supplies the required revalidation evidence.

Store policy, requests, attempts, immutable artifact references, restore approvals, recovery targets, retention/hold/cleanup events, and audit evidence relationally with tenant, source instance, profile scope, actor, and policy-revision foreign keys. Status surfaces only safe scalar references, timestamps, state, and error class.

## Inspect status and verify service

The planned status command is read-only, bounded, authorized, and scoped to an explicit profile/environment/instance. It must answer the following without probing or mutating every customer database:

| Question | Safe status/evidence |
| --- | --- |
| What is deployed? | Instance alias, environment/region aliases, manifest/release/config revision, and migration version. |
| Is it serving? | Readiness, bounded uptime/observed timestamp, activation state, safe DNS/ingress health, and sanitized error class. |
| Is the database aligned? | Dedicated-DB connectivity summary, desired versus last-observed migration state, observation time, and drift count. |
| Is policy satisfied? | Profile revision, placement mode `DEDICATED_INSTANCE`, RDS Proxy disabled, backup-policy state, and capacity reservation state. |
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

- [#581 deployment CLI and dedicated-RDS lifecycle](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/581)
- [#555 customer provisioning orchestration](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/555)
- [#507 forward-only migration framework](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/507)
- [#592 tenant-admin protected RDS recovery requests](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/592)
- [License Operations Runbook](LICENSE_OPERATIONS.md)
- [Deployment Runbook](DEPLOYMENT.md)
- [Production Runbook](PRODUCTION_RUNBOOK.md)
