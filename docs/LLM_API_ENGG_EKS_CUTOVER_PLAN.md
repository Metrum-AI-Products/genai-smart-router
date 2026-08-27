# `llm-api-engg.metrum.ai` EKS cutover planning runbook

This internal runbook is the planning deliverable for
[#725](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/725).
It grants no production, staging, cloud, DNS, database, configuration, Secret,
or rollout authority. `https://llm-api-engg.metrum.ai` remains served by and
authoritative on the EC2 Docker Compose deployment until a separately approved
cutover completes and its rollback window closes. Existing EKS deployments are
validation-only and must not become the hostname target or receive ordinary
production traffic under this plan.

The protected execution owner must consume, without duplicating:

- [#507](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/507)
  for forward-only migration compatibility, backup/restore, state integrity,
  and rollback classification;
- [#518](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/518)
  for protected rehearsal, production-profile authorization, two distinct
  approvers, canary, cutover, rollback, and evidence gates;
- [#555](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/555)
  for the single Fleet lifecycle, exact-host activation, isolated namespace,
  private encrypted PostgreSQL, PVC, license/config binding, and activation
  suite.

Do not create a second provisioner, registry, migration runner, activation
authority, or DNS workflow. Fleet remains the lifecycle writer; the protected
#518 workflow remains the promotion authority.

## Planning artifacts

Copy these checked-in templates into an owner-only protected change workspace:

- `deploy/release/fixtures/llm-api-engg-cutover-plan.template.json`
- `deploy/release/fixtures/llm-api-engg-cutover-evidence.template.json`

The plan schema is
`deploy/release/llm-api-engg-cutover-plan.schema.json`. It binds the exact
hostname, immutable image, configuration and license revisions, #507 migration
plan, 5% canary, DNS/TLS transition, acceptance matrices, rollback window, and
evidence checksums. The checked-in template deliberately records
`production_profile_authorized: false`; it is a no-go planning example, not a
release approval.

The evidence schema is
`deploy/release/llm-api-engg-cutover-evidence.schema.json`. Store one JSON
object per check, or newline-delimited objects when the protected evidence
system supports NDJSON. Records allow only safe scalar status, time, surface,
latency bucket, count/checksum, artifact reference, and observer ID. Never add
raw commands, SQL, logs, manifests, configuration, environment values, cloud
responses, prompts, model output, tool schemas, images, tokens or token hashes,
provider keys, Basic credentials, DSNs, Secret references, private keys, or
customer data.

Validate local planning copies offline:

```bash
rtk python3 scripts/validate_llm_api_engg_cutover_plan.py \
  --manifest /protected/change/llm-api-engg-cutover-plan.json \
  --evidence-record /protected/change/preflight-check.json
```

Success is only `planning_contract_valid_no_apply`. It never means go, approval,
or permission to execute. A real evidence store must independently hash-pin,
authenticate, retain, and authorize its records.

## Immutable topology and authority invariants

- Canonical production hostname: `llm-api-engg.metrum.ai`.
- Source before cutover: one authoritative EC2 Compose writer.
- Target: one Fleet-owned EKS Router replica using `Recreate`, one
  `ReadWriteOnce` state PVC, and a separate private encrypted PostgreSQL
  database with TLS hostname verification.
- No uncontrolled overlap: prove source write freeze before import or target
  write activation. Never run EC2 and EKS as concurrent writers.
- Production identities, namespace, runtime Secret/attestation, caller policy,
  database, license binding, PVC/state, image approval, and audit records are
  separate from staging.
- The delivery identity must not read/list/watch Secrets. Runtime values stay
  in the privileged Secret path and never enter a plan or evidence record.
- Use fresh discovery for exact ingress proxy CIDRs, ingress class, TLS,
  NetworkPolicy, and selected Linkerd posture. Do not reuse a Compose-only
  proxy range or infer production settings from staging.
- `/metrics` remains global and available only to a caller with
  `metrics_admin: true`; ordinary caller keys must receive
  `403 metrics-forbidden`, with no caller, token, or tenant labels.
- Preserve stored request-time costs and normalized relational diagnostics.
  Never reprice, reset, delete, double-count, or grant additional quota or
  license entitlement during reconciliation.

Any changed image digest, configuration/license revision, migration plan,
target profile, canary scope, DNS plan, rollback plan, or evidence checksum
invalidates approval and returns the plan to Stage 0.

## Stage 0 — design and production-shaped rehearsal

Planning/rehearsal checklist:

- [ ] #507 declares the exact schema/data compatibility range, migration IDs,
  jobs, lock/statement timeout policy, recovery rules, and rollback class.
- [ ] A protected logical backup and restore rehearsal succeeds against
  production-shaped synthetic/sanitized data.
- [ ] Rehearse source write freeze, export/import, state transfer,
  reconciliation, target readiness, bounded canary, DNS reversal, and restore.
- [ ] #555 fake-adapter, contract, security, activation, idempotency, ownership,
  retention, and disposable non-production E2E evidence passes.
- [ ] Supply-chain review binds one immutable image digest to provenance, SBOM,
  scan, signature, architecture, configuration fingerprint, and rollback image.
- [ ] Security review proves least-privilege RBAC, no workload Secret verbs,
  Pod security, private RDS/TLS, egress policy, exact ingress policy, and
  selected service-mesh policy.
- [ ] Each exposed group has an owner, workload contract, API dialects,
  modalities, tool surfaces, objective success threshold, cost/latency target,
  promotion criterion, and rollback criterion.
- [ ] Two distinct production approvers, independent observer, change owner,
  communications owner, stop authority, window, and rollback deadline are
  named in the protected system.

Any missing or mismatched item is no-go. Rehearsal evidence does not authorize
production mutation.

## Stage 1 — dark target preparation

This stage may run only under separate #518/#555 production authorization.
Prepare the exact isolated target without public-hostname traffic. Verify:

- [ ] one desired/available Router replica, `Recreate`, RWO PVC, resource
  limits, read-only filesystem, non-root security context, and no autoscaling;
- [ ] production-specific namespace, service account, RBAC, runtime
  Secret/attestation, license, configuration, database, state, and caller
  policy;
- [ ] private encrypted PostgreSQL, CA mount, `verify-full` behavior, bounded
  connection policy, compatible migration status, backup retention, and restore
  permission;
- [ ] exact-host Ingress remains absent or unreachable by ordinary production
  callers until activation and protected traffic approval pass;
- [ ] `/healthz`, `/readyz`, `/version`, diagnostics, reporting, and authorized
  observability produce only sanitized output.

## Stage 2 — write freeze and forward-only migration

Execution remains outside this repository-side plan. The protected record must
prove, in order:

1. communications and stop authority are active;
2. new mutable source traffic is drained or rejected by the approved method;
3. the Compose source has stopped accepting writes;
4. protected backup verification passes;
5. the #507 non-serving plan/apply/jobs/verify-serving/status sequence succeeds;
6. governed quota/license/router state moves through its versioned atomic
   procedure;
7. target row counts, safe aggregates, checksums, request-time cost totals,
   attempts/errors, finalized rollups, and report queries reconcile;
8. no source writer is re-enabled before an explicit rollback decision.

Do not run a reverse migration. Partial, duplicate, interrupted, stale, wrong
version, checksum-mismatched, timed-out, or unreconciled migration is no-go and
uses the documented #507 recovery path.

## Stage 3 — limited canary

The #518 default is one explicitly approved caller/route scope, at most 5% for
at least 30 minutes. Expansion requires a recorded pass for:

- [ ] readiness and persistence with exactly one writer;
- [ ] OpenAI Chat, OpenAI Responses, Anthropic Messages, streaming, usage
  chunks, tool calls/tool choice, images where contracted, request caps,
  retries/fallbacks, bridge translation, and safe errors;
- [ ] actual Codex CLI and Claude Code CLI text/tool/image workflows where
  contracted, asserting outcomes rather than quiet process exit;
- [ ] quotas, license states, usage/cost, reports, diagnostics, latency, TTFB,
  throughput, attempts, and fallback parity;
- [ ] metrics-admin success and ordinary-caller `403 metrics-forbidden`;
- [ ] zero high-severity reconciliation drift, authorization regression,
  missing writes, unbounded latency/error increase, or raw payload logging.

Any breach invokes stop authority; do not expand while investigating.

## Stage 4 — exact-host DNS and TLS transition

Only after canary acceptance:

- [ ] bind the reviewed exact Host rule and certificate for
  `llm-api-engg.metrum.ai`;
- [ ] verify authoritative and representative recursive resolver results,
  TTL/negative-cache assumptions, certificate hostname/chain, and expected
  ingress destination;
- [ ] prove staging and unintended hostnames do not route to production;
- [ ] preserve the isolated, write-frozen EC2 source and approved reversal plan
  throughout the rollback window;
- [ ] record only safe record type/status/timestamp/checksum evidence, never
  private zone contents or cloud credentials.

DNS success alone does not transfer authority.

## Stage 5 — observation and authority transfer

Expand only through pre-approved gates. The change owner may declare EKS
production authority only after every prior gate, client owner, independent
observer, reconciliation check, report/diagnostic check, and rollback
feasibility check records pass against unchanged bindings. Until that explicit
declaration, Compose remains authority.

## Stage 6 — rollback or separately approved cleanup

Rollback decision checklist:

- [ ] stop EKS traffic and writes before restoring source write authority;
- [ ] select the #507-compatible authoritative data/state recovery decision;
- [ ] restore the approved database/state when the rollback class requires it;
- [ ] restore the known-good Compose package/configuration without reverse
  schema migration;
- [ ] reverse DNS and verify TLS, APIs, CLIs, metrics isolation, quotas,
  licenses, usage, reports, and diagnostics;
- [ ] reconcile any canary-window writes and retain sanitized incident
  evidence.

EC2 destruction, storage deletion, backup expiry, credential revocation, alert
removal, and cost cleanup require a separate approved cleanup issue after the
rollback window and recovery retention requirements close. #725 does not grant
cleanup authority.

## Repeatable go/no-go record

For every stage, the protected change record must capture:

- immutable plan ID and hash; image/config/license/migration/traffic/DNS
  bindings; stage; UTC start/end; owner; independent observer;
- every checklist check ID with one evidence record and hash;
- thresholds, observed safe scalar result, pass/fail/blocked decision, and
  sanitized error class;
- approver references and expiry enforced by the protected workflow;
- stop/rollback decision, authority before/after, and next permitted stage.

Immediate no-go conditions include missing/expired approval, pending #507 or
#518 gate, changed binding, stale/unknown backup, failed restore rehearsal,
wrong proxy CIDR, TLS/RDS failure, RBAC/Secret overreach, more than one replica,
dual writer, quota/license mismatch, report or API/client regression, metrics
isolation failure, DNS ambiguity, secret-bearing evidence, or unavailable
rollback.

## Failure-injection matrix

Rehearsal must inject and record expected fail-closed behavior for EKS
readiness/rollout failure, ingress 4xx/5xx, certificate mismatch, RDS loss,
provider timeout/rate limit/5xx, provider 400 request-shape incompatibility,
API/CLI failure, migration interruption, quota/license integrity failure,
report failure, DNS propagation failure, and rollback failure. Each case names
safe telemetry, stop action, recovery owner, rollback trigger, and proof that no
secret or raw request/response was retained.

## Documentation closeout

After an actually authorized cutover, update operator runbooks and
`deployment.md` with only approved safe facts: authority transfer time,
hostname, immutable image/config/migration identifiers, backup/rollback state,
sanitized acceptance results, and cleanup issue. Public `docs-site/` should
change only when shipped caller-visible availability, API behavior, or generic
rollback guidance changes; never publish this deployment's private identities,
paths, cloud details, evidence locations, or operational commands.
