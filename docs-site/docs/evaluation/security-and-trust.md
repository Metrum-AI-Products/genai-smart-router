---
title: Security And Trust
---

# Security And Trust

GenAI Smart Router is designed to keep provider credentials, private upstream endpoints, routing policy, and operational telemetry under deployment control while giving applications a stable API.

## What Stays Server-Side

- Provider API keys.
- Private vLLM, SGLang, Baseten-style, or other upstream endpoint credentials.
- Full routing config and target weights.
- Caller token hashes.
- Metrics-admin credentials.
- TypeScript routing script files and external policy service authentication.

Callers receive a router endpoint, a router-issued token, and the model groups their token may request.

## Caller Access Controls

Router-issued caller tokens can encode:

- allowed model groups;
- user, project, environment, and client metadata for reporting;
- RPM and TPM limits;
- concurrency limits;
- daily, monthly, and lifetime request or token budgets;
- metrics-admin privilege when an operator token is intentionally created.

The `/v1/models` response is filtered by the caller token. Requests for unlisted groups fail before an upstream provider key is used.

## Diagnostics And Data Handling

Usage and diagnostics are designed for operational triage without storing sensitive request content by default.

Expected diagnostic fields include request IDs, selected provider/model, model group, status, attempt summaries, latency, sanitized errors, token counts, image counters, cost fields, cache behavior, and fallback events.

Diagnostic rows exclude raw prompts, raw image payloads, raw router tokens, token hashes, provider API keys, full upstream headers, and unsanitized upstream response bodies.

## PII Filtering

Model groups can redact configured text patterns before target selection, cache-key generation, routing-policy inputs, and upstream calls. Placeholder mappings are kept in memory for the request lifecycle unless a separate governed content-capture feature is explicitly enabled.

Regex filtering is a practical gateway control, not a full legal or compliance-grade detector. Deployments that need stronger detection should integrate a governed DLP or privacy service and validate the exact data flow.

See [PII Filtering](../configuration/pii-filtering).

## Metrics Isolation

`/metrics` exposes global operational telemetry and must be restricted to caller tokens configured with `metrics_admin: true`. Normal application caller keys receive `403 metrics-forbidden` and should use `/v1/usage` or generated reports for their own usage visibility.

## Private Upstreams

Enterprise-hosted inference services can remain on private network names while applications call the router. The router can mix those internal services with external providers in one model group for migration, overflow, or fallback.

For VLM services that fetch image URLs, configure upstream media-domain controls so the model server cannot fetch arbitrary internal URLs.

## Deployment Evidence To Review

Before production rollout, ask for:

- dependency and container scan summaries;
- secret-scan results for release artifacts;
- provider-key storage method;
- caller-token policy summary;
- metrics-admin token owner;
- private-upstream network policy;
- diagnostics redaction verification;
- rollback criteria and owner.

See [Deployment Security Assessment](./deployment-security-assessment) for the detailed checklist.
