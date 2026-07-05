---
title: Release Notes
doc_type: reference
---

# Release Notes

Release notes help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. Each packaged router build also displays its router version and build timestamp on every docs page.

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide). For the docs package index, see [Releases](/docs/releases).

The entries below describe customer-visible package changes, compatibility impact, validation expectations, and rollback considerations for shipped router releases.

## Current Package - 2026-06-29

### Highlights

- Browser docs now identify the router release they document on every page.
- Release notes are part of the shipped docs and describe the package operators are evaluating.
- The docs build checks for release-note safety patterns and required version metadata.
- Package, Docker, and operational docs consistently point operators to `/version`, `/readyz`, `/v1/models`, release notes, and rollback artifacts during upgrade validation.

### Operator Impact

- Config: no router runtime config change is required.
- Database: no usage database migration is required.
- License: no license replacement is required.
- Metrics and reports: no metrics schema or admin report API change is required.
- Docs operations: packaged docs now include release-note coverage for routing, auth, model metadata, deployment, reporting, CLI behavior, licensing, and caller-visible API behavior when those areas change.

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
