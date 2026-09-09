---
title: Upgrade Guide
doc_type: reference
---

# Upgrade Guide

Use this guide for customer-managed package upgrades. The exact maintenance window, approval process, and rollback policy belong to the deployment operator.

## Pre-Upgrade Checklist

1. Read the release notes for operator impact, database changes, license changes, and caller-visible behavior.
2. Confirm the page banner and `/version` identify the expected router version and build timestamp.
3. Confirm the package architecture matches the host.
4. Back up runtime config, provider env file, license file, license state, and usage database; verify the backup can be restored within the planned window and has sufficient free space.
5. Record the current router version and build timestamp from `/version`.
6. Confirm `/readyz`, `/v1/models`, metrics, and admin reports are healthy before the change.
7. Prepare a rollback package and the previous reviewed config.

## Required Migration Gate

For a package whose release contract includes a migration, stop or drain the serving router and follow the packaged `docs/DATA_MIGRATIONS.md` runbook: `router-migrate plan → approved backup → apply → all required data jobs validated → verify → status → serve`. `deployment-job` never applies application schema at startup; it validates the ledger and fails closed until the contract is current, compatible, and every required bound data job is validated—including the post-apply, pre-resume pending state. PostgreSQL production uses this non-serving job, not `auto-safe`; SQLite requires exclusive downtime, SQLite-safe backup and integrity verification, and free-space checks. Checkpoint ordinal `0` is not completion evidence.

Use `--dsn-env=ROUTER_USAGE_DB_DSN` for PostgreSQL and the documented non-serving container `--entrypoint` for Compose. Do not put a database connection string in commands, tickets, screenshots, or logs.

Releases containing usage migration `2026090901` add optional diagnostics
through the non-null text column
`request_usage.target_region` with an empty default for historical and
unlabelled rows. Plan and verify this migration through the same non-serving
gate; do not hand-add or backfill location claims. Package rollback follows
the migration contract and may require restoring the approved pre-migration
database snapshot.

## Docker Compose Upgrade

```bash
docker load -i images/smart-llmrouter-<version>-linux-<arch>.tar

cd compose
cp .env .env.backup
# Edit SMART_LLMROUTER_VERSION to the exact loaded image tag.
docker compose config >/dev/null
docker compose up -d
docker compose ps
```

Validate:

```bash
export ROUTER_BASE_URL="https://<router-host>"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/version"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

## Binary Upgrade

1. Stop or drain traffic according to the service manager and reverse proxy policy.
2. Install the new `smart-llmrouter` and operational CLI binaries.
3. Apply reviewed config or license changes.
4. Restart the service.
5. Validate readiness and caller/API smokes.

Example validation is the same as Docker Compose.

## Post-Upgrade Validation

Run:

- `/readyz`;
- `/version`;
- browser docs and release notes for the expected router version;
- `/v1/models` for at least one application caller;
- one completion smoke per changed model group or API skin;
- `/metrics` with a metrics-admin caller when metrics are enabled;
- `/admin/reports/api/summary?since=24h` when browser reports are enabled;
- `router-usage-report --since 1h` when usage reporting is enabled.

Watch for:

- elevated 5xx responses;
- router-side quota/rate-limit spikes;
- upstream 429/5xx attempts;
- fallback rate changes;
- latency and throughput regressions;
- license status changes;
- usage database write failures.

## Rollback

Rollback should restore the previous package, previous reviewed config, and previous valid license file when those inputs changed. Restart the router and repeat the same readiness, model, metrics, and report smokes.

Package rollback never runs a reverse migration. Use database restore whenever the release migration contract says `restore-required`; restore the approved pre-migration snapshot before deploying the earlier package. When the contract is compatible without restore, preserve the current usage database so request history remains intact. The database does not process a retrospective amendment merely because the package changed its mind.

## Most Recent Upgrade Flow

For v1.2.0, review the LRP follow-up and external-policy notes in the release
notes. New conversation-key, feedback, verifier-hint, signed-bundle, usage
import, selection-constraint, and uncertainty/explore knobs are opt-in; leave
them disabled unless the deployment has validated them. No new usage migration
is required solely for these LRP follow-ups.

For v1.1.0, review the migration and streaming changes in the release notes.
LRP is optional: install the separate locked Python/uv service from the release
source archive and validate its bundle before configuring a deployment-defined
staging group with `strategy: external`. Begin in shadow mode, compare the
recommendation with the served target, and require representative quality, cost
and latency evidence before enforcement. Roll back LRP by restoring the prior
strategy or using external-policy baseline mode; preserve the previous validated
bundle and configuration. See the [LRP guide](/docs/routing/learned-routing-policy)
for authenticated service setup, deadline tuning and the worked scenario.

For a docs-only package update:

1. Read the current release note and verify it calls out no config, database, license, model-group, or caller API changes.
2. Deploy the package using the Docker Compose or binary flow above.
3. Check `/version` and confirm the browser docs banner shows the same router version and build timestamp.
4. Open [Releases](/docs/releases) and [Release Notes](/docs/release-notes/) from the deployed router docs.
5. Run `/readyz`, `/v1/models`, and one representative completion smoke.
6. Roll back by restoring the previous package if the docs bundle or runtime health check is wrong.

For a behavior-changing package update:

1. Read the release note sections for operator impact, caller impact, validation, and rollback.
2. Apply reviewed config or license updates before restarting the router.
3. Run the release-specific model-group, API skin, metrics, report, and license smokes.
4. Compare latency, fallback, upstream errors, usage writes, and license status against the pre-upgrade baseline.
5. Roll back using the package, config, license, and database instructions in that release note.
