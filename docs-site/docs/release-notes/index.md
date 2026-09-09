---
title: Release Notes
doc_type: reference
---

# Release Notes

Release notes help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. Each packaged router build also displays its router version and build timestamp on every docs page.

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide). For the docs package index, see [Releases](/docs/releases).

The entries below describe the validation contract for the package that embeds
this page. The version banner, `/docs/releases`, and `/version` are the
authoritative sources for its exact router version and build timestamp; do not
infer the running version from a date written in documentation.

## Next Package - Version Assigned At Packaging

### Highlights

- Same-dialect OpenAI Chat and Anthropic Messages requests proxy native
  upstream SSE, including tool and usage events. Responses and cross-dialect
  streaming remain router-encoded after a unary upstream call.
- `dynamic_score` adds caller-isolated, process-local conversation affinity
  with bounded fixed-TTL storage.
- TypeScript decisions use fresh VMs with per-group concurrency admission;
  external policy adds explicit `baseline`, `shadow`, and `enforce` modes with
  reusable cancellation-aware HTTP clients.
- The outbound Gemini `generateContent` codec supports evidence-gated unary
  text targets. It is not a caller endpoint and does not enable tools, images,
  reasoning, structured output, or streaming.
- Targets may declare a bounded diagnostic `region` label. It records the
  actual serving target but does not enforce or prove processing location.
- Fleet lifecycle implementation is isolated from the serving request-path
  package, and package validation excludes Fleet binaries from runtime images.
- A second preregistered fixed-model OCR outcome gate demonstrates
  scalar-only quality, latency, cost, and regression decisions.

### Operator Impact

- Config: review `dynamic_score.affinity`, `script_max_concurrent`,
  `external_policy.mode`, and optional `targets[].region`. New external
  policies should begin in `shadow`; `baseline` skips policy calls.
- Database: migration `2026090901` adds non-null text
  `request_usage.target_region` with an empty default.
- Streaming: reverse proxies must not buffer SSE. After the first native event,
  a later failure cannot change the committed `200`, append an error envelope,
  or fall back.
- Security: `allow_private_image_urls: true` bypasses all router image-URL
  admission and therefore requires independent egress controls.
- Script safety: concurrency limits do not preempt unbounded JavaScript after
  a VM starts.

### Caller Impact

- Native Chat/Messages streams can deliver lower time-to-first-event and
  preserve upstream event shapes. Clients must treat a missing terminal event
  as incomplete.
- Native PII-filtered streams preserve placeholders; buffered responses and
  router-generated Responses/bridge streams can restore them.
- Existing model-group authorization and request-shape eligibility remain
  authoritative. Affinity and external policy cannot widen access.

### Validation

- Run the documented native Chat and Messages text/tool/usage/cancellation
  smokes, plus Responses and bridge regressions.
- Exercise affinity hit, miss, expiry, ineligible replacement, caller
  isolation, restart, and multi-replica behavior.
- Load-test TypeScript admission/cancellation and compare external-policy
  `shadow` recommendation with the served baseline before promotion.
- Plan, apply, and verify migration `2026090901`; confirm serving-target region
  attribution after successful fallback.
- Keep Gemini targets catalog-only unless the exact provider/model/account has
  direct and restricted router text evidence.

### Rollback

- Disable affinity or restore the previous strategy; set external policy to
  `baseline`; restore the prior script/cap; remove Gemini or region-bearing
  targets from active groups; and restore the prior package/config.
- Native-stream rollback requires the prior package. Follow the migration
  contract before downgrading; restore the approved pre-migration database
  when the contract requires it.

## v1.0.2 - 2026-09-08

GenAI Smart Router v1.0.2 is a documentation packaging release. Caller-facing
routing, model groups, and API behavior are unchanged from v1.0.1.

### Highlights

- Embedded Docusaurus docs now link the navbar GitHub control and Source License
  footer entry to the public Apache-2.0 home at
  `https://github.com/Metrum-AI-Products/genai-smart-router`.
- The legal software-licenses page uses the same public repository LICENSE URL.

### Operator Impact

- Config: unchanged.
- Database: no schema or data migration is introduced by this release.
- License: unchanged.
- Metrics and reports: unchanged.

### Caller Impact

- API behavior: unchanged.
- Model groups: unchanged.
- Errors: unchanged.
- Docs: `/docs/` GitHub and LICENSE links point at the public repository.

### Validation

- `/readyz` and `/version`: confirm `v1.0.2` and the new build timestamp.
- `/docs/`: confirm navbar and footer GitHub hrefs use
  `github.com/Metrum-AI-Products/genai-smart-router` and do not use
  `sysadmin-metrum-ai`.
- Completion smoke: run one request for an actively used model group.

### Rollback

- Restore the previous router package (`v1.0.1`) and the previous reviewed
  config.
- No reverse migration is required; preserve the usage database.

## v1.0.1 - 2026-09-08

GenAI Smart Router v1.0.1 is a maintenance release focused on admin browser
report availability. Caller-facing routing, model groups, and API behavior are
unchanged from v1.0.0.

### Highlights

- Deployments that require admin browser reports can now declare the
  reverse-proxy networks in front of the router. A deployment whose
  `server.admin_auth.basic.trusted_proxy_cidrs` does not cover those networks is
  rejected before rollout instead of serving an admin surface that refuses every
  correct credential.
- Admin authentication documentation explains the failure mode where Basic Auth
  returns `401` for a valid password because the forwarded-HTTPS check runs
  before the password comparison.

### Operator Impact

- Config: when `allow_insecure_http` is `false`, confirm that
  `server.admin_auth.basic.trusted_proxy_cidrs` contains the network your reverse
  proxy or ingress controller connects from. In Kubernetes this is the cluster
  pod network, which usually differs from a local kind or Docker bridge range.
- Database: no schema or data migration is introduced by this release.
- License: unchanged.
- Metrics and reports: `/metrics` isolation and admin report authorization are
  unchanged.

### Caller Impact

- API behavior: unchanged.
- Model groups: unchanged.
- Errors: unchanged.

### Validation

- `/readyz` and `/version`: confirm the new router version and build timestamp.
- Admin reports: an unauthenticated request returns `401` with a Basic challenge,
  an incorrect password returns `401`, and an authorized request loads the report
  shell and a SQL-backed summary.
- Completion smoke: run one request per actively used model group.

### Rollback

- Restore the previous router package and the previous reviewed config.
- No reverse migration is required; preserve the usage database.

## v1.0.0 - 2026-09-07

GenAI Smart Router v1.0.0 is the first public open-source release package.
Use the docs banner, `/docs/releases`, and `/version` for the exact router
version and build timestamp of the package that embeds this page.

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
- Operations: `metrum-genai-smartrouterctl` is available for customer-local safe config,
  token-file, license, model, and aggregate-usage operations. Fleet lifecycle
  authority moved to binary-package-only `metrum-genai-smartrouter-fleetctl`; the old
  `metrum-fleetctl` / `metrum-smartrouterctl` / `smartrouterctl` names are
  one-release rename notices.
- Self-hosted serving: validated NVIDIA and AMD Instinct reference paths cover
  private in-cluster OpenAI-compatible model servers while keeping the router
  workload GPU-free.

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
- CLI packaging: confirm customer Docker images include `metrum-genai-smartrouterctl` and
  exclude `metrum-genai-smartrouter-fleetctl`. Fleet RDS mutation remains fail-closed pending
  recorded non-production evidence and a qualified reviewer's approval, which a
  single-maintainer deployment may supply itself. Managed production profile
  authority is operator-owned and is not granted by the customer package.

### Caller Impact

- API behavior: validate every caller API skin and client workflow affected by
  the package or config change.
- Model groups: callers must discover their allowed deployment-defined groups
  through authenticated `/v1/models`.
- Errors: preserve structured caller-facing error types and request IDs; use
  sanitized attempt and diagnostic rows for root-cause analysis.
- Client compatibility: run the actual Codex and Claude Code CLIs when routing,
  tools, images, auth, or model metadata changed.

### Compatibility and Known Limitations

- Model groups are deployment-defined; callers discover their allowed groups
  through authenticated `/v1/models` rather than relying on fixed names.
- Capability eligibility is request-shape-specific. A target is used only for
  dialects, tools, modalities, bridges, and payload sizes validated for it.
- AMD and NVIDIA local-serving support depends on the exact operator, driver,
  serving image, model, parser, hardware, and request shape documented by the
  deployment's validation evidence.
- The docs build retains an unpatched build-time image metadata dependency risk.
  It is not executable in the published browser bundle. The v1.0.0 launch
  decision accepts this risk through 2026-10-04 with reviewed documentation
  inputs, bounded build jobs, and static generated docs as the serving boundary;
  the accountable owner must recheck or remediate it by that date.

### Validation

- Confirm the docs banner, `/docs/releases`, and `/version` agree on the
  expected version and build timestamp for the deployed package.
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
