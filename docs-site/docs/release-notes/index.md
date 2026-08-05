---
title: Release Notes
doc_type: reference
---

# Release Notes

Release notes help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. Each packaged router build also displays its router version and build timestamp on every docs page.

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide). For the docs package index, see [Releases](/docs/releases).

The entry below describes the validation contract for the package that embeds
this page. The version banner, `/docs/releases`, and `/version` are the
authoritative sources for its exact router version and build timestamp; do not
infer the running version from a date written in documentation.

## Current Package

### Highlights

- The package embeds documentation for its own runtime build and exposes the
  same version and build timestamp through the docs banner and `/version`.
- Caller-facing routing supports deployment-defined model groups across the
  configured OpenAI Chat, OpenAI Responses, and Anthropic Messages surfaces.
- Capability and request-shape eligibility keep tools, images, reasoning,
  structured outputs, output caps, bridges, and large payloads on targets
  validated for those exact surfaces.
- Usage, diagnostics, cost, latency, attempt, fallback, traffic-shaping, and
  governed admin-report surfaces use safe scalar operational evidence.

### Operator Impact

- Config: compare the packaged `config.example.yaml` with the reviewed runtime
  config. Do not copy sample provider/model routes directly into production.
- Database: follow the package's migration policy and release-specific
  deployment record. The current migration contract is: `2026071901` creates
  schema version 1/data version 0 through an online explicit baseline;
  `2026072301` advances schema version 2/data version 0 and is
  `restore-required`; `2026080501` advances data version 1 and is also
  `restore-required` and requires its restart-safe
  `historical-usage-validation-v1` job to reach `validated` before service
  startup. Run the non-serving deployment-job gate and retain the approved
  pre-migration backup when required.
- License: verify the installed license permits the enabled features and
  deployment shape.
- Credentials: preserve the protected provider environment file or
  secret-manager state; never place provider keys in release evidence.
- Metrics and reports: retain `/metrics` isolation for metrics-admin subjects
  and verify report authorization after upgrade.

### Caller Impact

- API behavior: validate every caller API skin and client workflow affected by
  the package or config change.
- Model groups: callers must discover their allowed deployment-defined groups
  through authenticated `/v1/models`.
- Errors: preserve structured caller-facing error types and request IDs; use
  sanitized attempt and diagnostic rows for root-cause analysis.
- Client compatibility: run the actual Codex and Claude Code CLIs when routing,
  tools, images, auth, or model metadata changed.

### Validation

- Confirm the docs banner, `/docs/releases`, and `/version` agree on the
  expected version and build timestamp.
- Run `/readyz`.
- Run authenticated `/v1/models` with each affected caller class.
- Run representative Chat, Responses, Messages, streaming, tool, image, and
  output-cap smokes for every changed surface.
- Run metrics-admin and ordinary-caller `/metrics` authorization checks when
  metrics are enabled.
- Run admin report and license-status checks when those features are enabled.

### Rollback

- Restore the previous package and reviewed runtime config.
- Restore the previous license input only when the license changed.
- Never run a reverse migration. Preserve the current usage database only when
  the release contract allows package/config rollback without restore; for a
  `restore-required` contract, restore the approved pre-migration snapshot
  before deploying the earlier package.
- Repeat `/readyz`, `/version`, `/v1/models`, and the failed caller/client smoke
  before returning traffic.
