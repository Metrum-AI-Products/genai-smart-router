# Security Review Notes

This document records product security expectations for implementation and operations.

## Secrets

Never commit or print provider API keys, raw router tokens, token hashes, full production config, `env.json`, `ROUTER_TOKEN*.txt`, or production DB dumps.
Keep `env.example.json` placeholder-only and run `make secret-check` before publishing changes that touch runtime configuration or package contents. Real provider keys belong in ignored `env.json`, process environment, or the deployment secret manager.

Provider keys stay server-side. Caller tokens authenticate to the router and are checked before provider calls.

## Tenant And Caller Isolation

Caller tokens carry allow lists, quota policy, and metadata such as user, project, and environment. `/v1/models` is filtered to the presented token's allowed model groups.

`/metrics` is global operational telemetry. It must only be accessible to tokens configured with `metrics_admin: true`; ordinary caller tokens must use `/v1/usage` or generated usage reports.

## Diagnostics And Redaction

Request diagnostics may contain request ID, caller metadata, selected route/provider/model, status/error class, token counts, latency, sanitized upstream error class/message, and cost fields.

Diagnostics must not contain raw prompts, raw images, raw router tokens, token hashes, provider API keys, full upstream headers, or unsanitized upstream response bodies.

Model-group `pii_filter` may redact configured request text before routing policy, cache keys, and upstream calls. TypeScript and external policy request contexts are built from the redacted request, including raw payload mirrors. PII-filter usage metadata must stay scalar and safe: applied flag, mode, replacement count, and matched-rule count only. Raw matched values and placeholder mappings must remain in memory for the request lifecycle unless a separate governed content-capture feature explicitly enables durable storage.

## Docs And Examples

Public docs must not hardcode the current Metrum-managed production URL as the product endpoint. Use deployment placeholders except for historical case studies or explicitly labeled hosted-deployment examples.

Model group names are deployment-defined. Public docs may show names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` only as examples or historical deployment names.

## Production Change Safety

For production changes, take timestamped backups, use structured config edits, keep local production snapshot synchronized, verify local and remote config hashes, run relevant real smokes, update deployment notes, and clean temporary files and stale packages.

## Dependency Scans

Release Docker builds must use the pinned Go builder image from `Dockerfile`; do not replace it with a floating `golang:<minor>-alpine` tag during packaging. When Go standard-library advisories are reported by `govulncheck`, verify both the local toolchain and the Docker builder image patch version.

For hosted docs, run `npm audit --prefix docs-site --audit-level=moderate` after dependency updates. As of 2026-06-24, remaining moderate npm audit findings are Docusaurus-transitive `gray-matter` usage of `js-yaml@3` with no patched Docusaurus dependency path available. The docs dependency tree is used to build trusted repository documentation and is not part of the router API request path.
