---
title: Release Notes
doc_type: reference
---

# Release Notes

Release notes help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. Each packaged router build also displays its router version and build timestamp on every docs page.

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide). For the docs package index, see [Releases](/docs/releases).

## Current Package - 2026-06-29

### Highlights

- Browser docs now identify the router release they document on every page.
- Release notes are part of the shipped docs instead of a placeholder-only template.
- The docs build checks for release-note safety patterns and required version metadata.
- Package, Docker, and operational docs consistently point operators to `/version`, `/readyz`, `/v1/models`, release notes, and rollback artifacts during upgrade validation.

### Operator Impact

- Config: no router runtime config change is required.
- Database: no usage database migration is required.
- License: no license replacement is required.
- Metrics and reports: no metrics schema or admin report API change is required.
- Docs operations: release notes must be updated before publishing a package that changes routing, auth, model metadata, deployment, reporting, CLI behavior, licensing, or caller-visible API behavior.

### Caller Impact

- API behavior: OpenAI-compatible and Anthropic-compatible API behavior is unchanged.
- Model groups: no model-group names, eligibility rules, or routing weights change in this docs-only release.
- Errors: no caller-facing error codes change in this docs-only release.
- Docs UX: callers and operators can confirm the documented router release from the page banner and the `<meta name="docs-version">` tag.

### Validation

- `/readyz`: unchanged; run as part of package rollout.
- `/version`: confirms the running router version and full UTC build timestamp.
- `/v1/models`: unchanged; smoke with a test caller after deployment.
- Completion smoke: unchanged; run for any model group changed by the package being deployed.
- Docs build: validates the version banner, metadata tags, releases page, release-note entries, and forbidden release-note patterns.

### Rollback

- Restore the previous router package.
- Restore the previous reviewed config and license file only if they changed with the package.
- Preserve the current usage database unless a release-specific note explicitly calls out a non-reversible schema or data migration.

## Release Note Fields

| Field | Purpose |
|---|---|
| Release version | Identifies the package installed on the deployment. |
| Build timestamp | Confirms the running binary matches the shipped artifact. |
| Package architecture | Confirms `linux-amd64` or `linux-arm64` package selection. |
| Config changes | Shows whether operators must update model groups, callers, admin policy, database settings, or license paths. |
| Database changes | Identifies migration, backup, and rollback considerations. |
| License changes | Identifies new feature gates, limits, or license replacement needs. |
| Client-visible behavior | Explains API, error, model metadata, or routing changes. |
| Validation | Lists readiness, model, report, metrics, and caller smokes. |
| Rollback | Names the prior package and config artifact to restore. |

## Template

```md
## <version> - <YYYY-MM-DD>

### Highlights

- ...

### Operator Impact

- Config: ...
- Database: ...
- License: ...
- Metrics and reports: ...

### Caller Impact

- API behavior: ...
- Model groups: ...
- Errors: ...

### Validation

- `/readyz`: passed
- `/version`: passed
- `/v1/models`: passed for a test caller
- Completion smoke: passed for `<model-group>`
- Admin report smoke: passed when enabled

### Rollback

- Restore package `<previous-version>`.
- Restore the previous reviewed config and license file if they changed.
- Restore database backup only if the release note explicitly calls for it.
```

Do not include private hostnames, SSH details, raw tokens, token hashes, provider API keys, full production config, private signing details, or customer-specific license payloads in public release notes.
