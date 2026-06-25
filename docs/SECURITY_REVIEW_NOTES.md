# Security Review Notes

This document records product security expectations for implementation and operations.

## Secrets

Never commit or print provider API keys, raw router tokens, token hashes, full production config, `env.json`, `ROUTER_TOKEN*.txt`, or production DB dumps.
Keep `env.example.json` placeholder-only and run `make secret-check` before publishing changes that touch runtime configuration or package contents. Real provider keys belong in ignored `env.json`, process environment, or the deployment secret manager.

Provider keys stay server-side. Caller tokens authenticate to the router and are checked before provider calls.

Browser-admin HTTP Basic authentication is separate from router caller tokens. It is disabled by default, must use bcrypt password hashes from deployment secrets or environment variables, and must run over HTTPS in production. If TLS terminates at a reverse proxy, `X-Forwarded-Proto: https` is trusted only from configured proxy CIDRs. Basic Auth establishes a subject such as `basic:admin`; it does not grant broader admin permissions by itself. `/metrics` remains caller-token protected with `metrics_admin: true`.

Admin browser reports under `/admin/reports/*` are disabled by default. When enabled, they require Basic Auth identity and Casbin authorization for `admin:reports` on every page, JSON API, request drilldown, static asset request, and Markdown export. Ordinary router caller tokens receive `403 reports-forbidden`. Report responses expose safe scalar usage, cost, latency, cache, fallback, provider/model, public token ID, caller metadata, and sanitized diagnostic fields only; they must not include raw provider keys, raw router tokens, token hashes, raw prompts, raw images, raw tool outputs, full config, or unsanitized upstream bodies.

## Tenant And Caller Isolation

Caller tokens carry allow lists and quota policy, while identity is validated through explicit `users`, `projects`, and `project_memberships` config sections. Each key references an `owner_user`, project, and environment. `/v1/models` is filtered to the presented token's allowed model groups, and disabled keys are rejected after token match with a safe `403 key-disabled` error.

`/metrics` is global operational telemetry. It must only be accessible to tokens configured with `metrics_admin: true`; ordinary caller tokens must use `/v1/usage` or generated usage reports.

Content-capture maintenance is separate from metrics access. Delete-by-request and retention purge endpoints require `content_admin: true`; metrics-admin tokens do not imply content-admin privileges.

## Diagnostics And Redaction

Request diagnostics may contain request ID, caller metadata, selected route/provider/model, status/error class, token counts, latency, sanitized upstream error class/message, and cost fields.

Diagnostics must not contain raw prompts, raw images, raw router tokens, token hashes, provider API keys, full upstream headers, or unsanitized upstream response bodies.

Model-group `pii_filter` may redact configured request text before routing policy, cache keys, and upstream calls. TypeScript and external policy request contexts are built from the redacted request, including raw payload mirrors. PII-filter usage metadata must stay scalar and safe: applied flag, mode, replacement count, and matched-rule count only. Raw matched values and placeholder mappings must remain in memory for the request lifecycle unless a separate governed content-capture feature explicitly enables durable storage.

Governed content capture is opt-in and disabled by default. When enabled, captured request, response, and upstream-error content is stored in separate relational tables keyed by `request_id`, with retention timestamps and audit rows for delete and purge operations. Captured content is redacted before storage with built-in secret patterns and configured regex rules. Header capture is allowlist-only and must not include authorization, API-key, token, secret, cookie, or key-like headers. The current foundation does not implement KMS/encryption-at-rest or content export/read APIs; enabling `content_capture.encryption.enabled` is rejected until that support exists.

## Docs And Examples

Public docs must not hardcode the current Metrum-managed production URL as the product endpoint. Use deployment placeholders except for historical case studies or explicitly labeled hosted-deployment examples.

Model group names are deployment-defined. Public docs may show names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` only as examples or historical deployment names.

## Production Change Safety

For production changes, take timestamped backups, use structured config edits, keep local production snapshot synchronized, verify local and remote config hashes, run relevant real smokes, update deployment notes, and clean temporary files and stale packages.

## Dependency Scans

Release Docker builds must use the pinned Go builder image from `Dockerfile`; do not replace it with a floating `golang:<minor>-alpine` tag during packaging. When Go standard-library advisories are reported by `govulncheck`, verify both the local toolchain and the Docker builder image patch version.

For hosted docs, run `npm audit --prefix docs-site --audit-level=moderate` after dependency updates. As of 2026-06-24, remaining moderate npm audit findings are Docusaurus-transitive `gray-matter` usage of `js-yaml@3` with no patched Docusaurus dependency path available. The docs dependency tree is used to build trusted repository documentation and is not part of the router API request path.
