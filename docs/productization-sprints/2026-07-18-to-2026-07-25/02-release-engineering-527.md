# 02 — Migration-Safe EKS Delivery Foundation (#527)

Plan version: **v1.0.0**  
Issue: [#527](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/527)  
Classification: **launch blocking**

## Design approach

Extend #507 and #516-#519 with one Git-native release train for router,
control-plane API/worker, console, and schema migrations. Keep dev, stage, and
prod as explicit overlays. Build once; promote immutable digests, not rebuilt
tags. Use Helm/Kustomize only where each is already canonical; do not create two
owners for the same manifest.

The pipeline stages are: source checks -> unit/integration -> image build -> SBOM
and provenance -> signature -> ephemeral contract environment -> migration
compatibility -> staging deploy -> synthetic customer journey -> soak/SLO gate ->
human production approval -> progressive rollout -> evidence bundle.

Database changes use expand/migrate/contract. A new binary must run against the
old schema during rollout and the old binary against the expanded schema during
rollback. Contract migrations run in a later release only after telemetry proves
no old reader/writer remains.

## Human interaction and configuration

SRE configures GitHub OIDC roles, ECR repositories, KMS signing references,
cluster/namespace allowlists, secret-manager references, alert destinations,
approval environment, migration lock timeout, rollout percentages, soak periods,
and rollback thresholds. Security approves least-privilege IAM and artifact
policy. No long-lived AWS credential is stored in GitHub.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). This issue owns
GitHub environment protection, GitHub-to-AWS OIDC trust, scoped build/promote IAM
roles, ECR repositories, KMS/cosign signing reference, evidence storage, EKS
cluster/namespace aliases, migration job role, and deployment notifications.
Non-secret config includes image repositories/digests, chart/schema versions,
approved clusters, rollout/soak/threshold values, required checks, and artifact
retention. CI receives no Stripe, provider, license-signing, grant-signing, or
production DB secret; stage smokes obtain bounded test references through their
service account.

## Deployment flow

1. CI produces signed images/SBOMs for every deployable component.
2. An ephemeral namespace installs Postgres-compatible dependencies and runs API,
   migration, ledger, router, and browser E2E tests.
3. Staging applies expand migrations, deploys workers/APIs, then data-plane pods.
4. Health gates verify `/version`, `/readyz`, console, lease/grant path,
   reconciliation lag, and one fully settled request.
5. Production promotes exact digests with a 0%/shadow, 1%, 10%, 50%, 100% gate
   appropriate to the component. Database workers use leader election/leases.
6. A breached gate halts promotion and rolls workloads/config back; data remains
   on the compatible expanded schema.

## Monitoring and rollback

Gate on readiness, 5xx, auth denials, grant denial anomalies, provider-attempt
rate, p95/p99 latency, webhook and settlement lag, reconciliation delta, stuck
provisioning, and UI synthetic success. Rollback restores previous image and
active config. Restore a database only for a proven destructive/data-corruption
event; ordinary application rollback must preserve current financial/usage rows.

## Test plan

* Unit-test release metadata, manifest rendering, policy checks, migration order,
  and forbidden mutable tags.
* Deploy the full stack into an ephemeral Kubernetes environment and run the
  #520 journey with fake provider/Stripe dependencies.
* Run old-app/new-schema and new-app/old-expanded-schema matrices on PostgreSQL.
* Tamper with an image, SBOM, signature, and manifest; each must be rejected.
* Inject failed readiness, SLO regression, migration lock, worker duplicate,
  secret absence, and failed smoke; prove no promotion and a healthy rollback.
* Restore a staging backup, replay outboxes, and prove ledger/request idempotency.
* Record workflow logs, artifact digest/signature verification, rendered diff,
  migration transcript, rollback replay, and before/after SLO screenshots.

## Definition of done

No component can reach production without immutable artifact, migration, security,
synthetic-flow, SLO, and human-approval evidence. Runbooks name owners for failed
migrations, partial provisioning, stuck settlement, and emergency rollback.
