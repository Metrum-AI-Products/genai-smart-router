# Release and Change-Management Evidence Record

## Scope

This sanitized evidence record supports parent issue #961 and subissue #968 (release and change management). It records repository-visible release-note, upgrade, and announcement-process evidence collected on 2026-09-03. It does not assert that a particular release was approved, deployed, or announced outside the repository.

## Evidence collected

- **2026-09-03 — `docs/DOCS_MAINTENANCE.md` (lines 47–66):** identifies the public release-notes and upgrade documentation as customer-safe material aligned with package/release scripts and internal operational sources. It defines the release-note workflow, required entry content, and prohibited sensitive content.
- **2026-09-03 — `docs-site/docs/release-notes/index.md` (lines 8–15, 72–92):** describes the release-note purpose; identifies the documentation banner, releases index, and `/version` as the authoritative version/build-time sources; and defines validation and rollback expectations.
- **2026-09-03 — `docs-site/docs/release-notes/upgrade-guide.md` (lines 8–24, 61–107):** assigns maintenance-window, approval, and rollback policy to the deployment operator; specifies pre-upgrade backup and migration-gate expectations; and defines post-upgrade validation and rollback steps.
- **2026-09-03 — supplied Google tracker/diagram reference:** anonymous access returned HTTP 401. No private Google content was accessed or retained.

## Demonstrated facts

- The repository documents a release-note process: draft from available release tags, then review and hand-edit the entry before publication so it covers highlights, operator impact, caller impact, validation, and rollback.
- Public release notes are required to include available version/date/build information, applicable operational and caller-visible changes, validation evidence, and rollback guidance; they must exclude credentials, protected configuration, private signing details, customer-specific license data, and internal source-control workflow details.
- The packaged documentation banner, releases index, and `/version` are the documented authoritative sources for the exact running router version and build timestamp; documentation dates alone are not sufficient.
- The upgrade process requires an operator-defined maintenance window, approval process, and rollback policy, along with backups, migration gating where applicable, readiness/model/API validation, and rollback validation.

## Evidence gaps and compliance-owner handoff

- No repository-visible approval ticket, change advisory record, release calendar entry, or signed deployment authorization establishes that a specific release passed organizational approval. The **change-management/control owner** should provide the approved change record, approver identity/role, implementation window, risk assessment, and closure decision through the governed system of record.
- The supplied externally held Google tracker/diagram could not be inspected because anonymous access returned HTTP 401. The **Google Workspace artifact owner** should provide a sanitized export, access-controlled review, or a non-secret immutable reference and its access decision; do not place the diagram or private Google content in this record.
- No standalone repository changelog was located during collection. The **release owner** should identify the authoritative changelog or release registry, if one exists outside the repository, and preserve a sanitized version/date/change reference.

## Sanitization

This record contains repository paths, public-style documentation route names, control descriptions, and the anonymous-access result only. It omits credentials, tokens, token hashes, customer data, private production URLs, protected configuration, private Google content, announcement message identifiers, and diagram contents.
