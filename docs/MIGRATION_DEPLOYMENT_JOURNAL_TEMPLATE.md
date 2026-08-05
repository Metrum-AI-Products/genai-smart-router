# Migration Deployment Journal Template

Use this pre-deploy template with [Data migration framework](DATA_MIGRATIONS.md). It contains placeholders only; do not paste credentials, DSNs, tokens, full configuration, private endpoints, customer data, SQL, or backup contents. The form exists so an approved change can be reconstructed without requiring the database to remember a meeting.

## Change record

- Change reference: `<approved-change-reference>`
- Planned UTC window: `<start-utc>` to `<end-utc>`
- Package version: `<package-version>`
- Migration scope and IDs: `<scope-and-ids>`
- Execution class: `<online|maintenance|non-transactional-maintenance>`
- Rollback class: `<compatible-without-restore|restore-required>`
- Serving policy: `deployment-job`

## Preconditions

- [ ] `router-migrate --version` matches the package intended for service.
- [ ] `router-migrate --action=plan` produced a compatible, reviewed result.
- [ ] Router traffic is drained or stopped as required by the execution class.
- [ ] PostgreSQL: approved consistent backup reference `<backup-reference>` is recorded; restore path, free space, permissions, and version compatibility were checked.
- [ ] SQLite: exclusive downtime is active; every process is stopped; SQLite-safe backup, integrity check, and source/backup free-space checks passed.
- [ ] Metrics-admin and browser-admin read-only access are available for verification where enabled.

## Deployment-job evidence

- Apply result: `<safe-state-and-time>`
- Verify result: `<safe-state-and-time>`
- Status result: `<safe-state-and-time>`
- Data-job state, if bound: `<safe-state-and-progress>`
- Metrics-admin migration status: `<safe-result>`
- Read-only Operations / Data migrations report: `<safe-result>`

## Release and recovery decision

- Serving startup approved only after all gate results are compatible/current/verified: `<yes|no>`
- If recovery is required: `<approved-restore-reference-and-next-safe-step>`
- Rollback decision: `<no-rollback|package-config-only|restore-approved-snapshot-then-earlier-package>`
- Post-start health and caller smoke summary: `<safe-status-summary>`
