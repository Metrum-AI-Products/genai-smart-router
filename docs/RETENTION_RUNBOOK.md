# Retention Runbook

This runbook covers the first production-useful commercial retention slice for the usage database.

## Design

Retention policy is configured under `server.retention` and persisted to scalar relational tables:

- `retention_policy_versions` and `retention_policy_rules` store config-derived policy snapshots.
- `retention_jobs` stores each status or run invocation.
- `retention_job_table_results` stores per-table candidate, held, eligible, blocked, and deleted counts.
- `legal_holds` and `legal_hold_audit_events` store active/released holds and create/release audit events.

The schema remains relational only. Do not add JSON, arrays, blobs, raw prompts, raw images, raw tool outputs, bearer tokens, provider keys, token hashes, or packed multi-value text fields.

Known data classes are `usage_diagnostics`, `decision_telemetry`, `security_access_events`, `content_capture`, and `usage_detail`. Defaults are conservative: retention is disabled, `dry_run` defaults to true, and `usage_detail` is disabled with `require_finalized_rollup: true`.

Legal holds block deletion by `data_class`, optional `request_id`, and optional `[start_ts,end_ts)` range. Empty `request_id` applies to the whole data class and range. Empty timestamps make the hold open-ended on that side.

## Status

Run a dry-run status job from reviewed config:

```bash
router-usage-report --retention-status --config /app/config/config.yaml
```

Status jobs force dry-run behavior even if the reviewed config has `dry_run: false`. They create or reuse the active policy version, count candidates, subtract active legal holds, and write table results without deleting rows.

## Batch Run

Run one reviewed retention batch:

```bash
router-usage-report --retention-run --config /app/config/config.yaml
```

`--retention-run` follows `server.retention.dry_run`.

- With `dry_run: true`, it writes status counts only.
- With `dry_run: false`, it deletes at most `batch_size` eligible rows per supported table.
- Supported delete classes are `usage_diagnostics`, `content_capture`, and `usage_detail`.
- Content-capture deletion removes the selected capture row and its allowed-header child rows in the same retention transaction.
- Unsupported classes are recorded as `blocked_not_implemented` with zero deleted rows.
- `usage_detail` deletes are blocked as `blocked_rollup_required` unless finalized daily rollups continuously cover the candidate window.

Finalized rollups are immutable. Retention checks rollup coverage but does not update or delete rollup rows.

## Rollout

1. Keep `server.retention.enabled: false` until usage DB migrations have run in the target environment.
2. Enable retention with `dry_run: true` and reviewed class retention windows.
3. Run `--retention-status` and inspect `retention_job_table_results`.
4. Create any required legal holds before enabling deletion.
5. For `usage_detail`, finalize daily rollups for the full candidate window.
6. Change only the reviewed deployment config to `dry_run: false` after backup and approval.
7. Run `--retention-run` and verify deleted counts are within the configured batch size.
8. Repeat batch runs only after checking the latest job and operational dashboards.

## Rollback

Set `server.retention.dry_run: true` or `server.retention.enabled: false` and deploy/reload the reviewed config. This stops generic retention deletes. Deleted raw rows are not restored by the router; use database backups if restoration is required.

## Deferred

Archive/export before delete, scheduler support, full legal-hold admin APIs,
browser write workflows, and generic purge execution for decision telemetry
and security access events remain future work. Governed content capture also
retains its dedicated maintenance endpoints.
