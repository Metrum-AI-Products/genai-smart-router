---
title: Release Notes
---

# Release Notes

Release notes should help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. They should describe shipped runtime behavior without exposing private deployment details.

## What To Track Per Release

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

## Customer-Safe Release Note Template

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

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide).
