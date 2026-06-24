---
title: Deployment Security Assessment
---

# Deployment Security Assessment

Use this checklist to assess a GenAI Smart Router deployment before production rollout or after a major routing, provider, auth, or telemetry change.

## Secrets And Authentication

Confirm:

- provider API keys are loaded server-side from the deployment environment or protected config files;
- caller tokens are distributed only to approved users, services, or validation jobs;
- runtime config stores caller token hashes, not raw caller token secrets;
- raw provider keys, raw router tokens, token hashes, and full production config are excluded from browser docs, logs, tickets, and announcements;
- metrics-admin access uses separate caller tokens with `metrics_admin: true`.

Acceptance checks:

- ordinary caller token can access allowed API paths;
- ordinary caller token receives `403 metrics-forbidden` on `/metrics`;
- metrics-admin token can scrape `/metrics`;
- `/v1/models` returns only groups allowed for the presented token.

## Diagnostics And Data Handling

Confirm diagnostic records exclude:

- raw prompts;
- raw image payloads;
- raw router tokens;
- token hashes;
- provider API keys;
- full upstream headers;
- unsanitized upstream response bodies.

Expected diagnostics include request IDs, selected provider/model, attempt summaries, status, latency, sanitized errors, token counts, image counters, cost fields, cache behavior, and fallback events.

## Network And Private Upstreams

For private vLLM, SGLang, Baseten-style, or other OpenAI-compatible upstreams:

- keep upstream endpoints on private network names where possible;
- expose the router as the governed ingress point;
- validate direct upstream reachability from the router host or network;
- validate router-level access from approved clients;
- keep private upstream tokens in the deployment environment;
- configure media-domain restrictions for VLM services that fetch image URLs.

## API Surface Review

Validate:

- `/readyz` and `/version` expose build metadata without secrets;
- `/v1/chat/completions`, `/v1/responses`, and `/v1/messages` enforce caller auth, allow lists, quotas, and target eligibility;
- `/v1/usage` returns caller-appropriate usage visibility;
- `/metrics` is restricted to metrics-admin tokens;
- no private host paths, SSH details, provider keys, or raw tokens appear in hosted docs.

## Dependency, Container, And Package Scans

Each deployment should follow the organization's security process for:

- container image vulnerability scanning;
- dependency scanning for packaged docs and runtime dependencies;
- verification that release images are built with the pinned Docker builder Go patch version, not a floating language image tag;
- secret scanning of release artifacts;
- static analysis or policy review for TypeScript routing scripts;
- review of third-party script dependencies before packaging;
- verification that the router does not install packages or fetch dependency code at runtime.

The Docker builder image is pinned to a patched Go toolchain tag so reachable Go standard-library advisories are controlled by the image patch version used for the release build.

As of 2026-06-24, the docs build still reports moderate npm audit advisories through Docusaurus' `gray-matter` dependency on `js-yaml@3`. Docusaurus has no patched dependency path for that finding yet. The affected package is used during documentation build and content parsing, not in the router request path; deployment reviews should record the residual, keep authored docs inputs trusted, and re-run `npm audit --prefix docs-site --audit-level=moderate` when Docusaurus publishes a fix.

## Security Sign-Off Record

Record:

- deployment version and build timestamp;
- config backup path;
- token policy summary;
- provider key storage method;
- metrics-admin token owner;
- private upstream network policy;
- scan tooling and result summary;
- accepted residual risks;
- rollback owner and rollback command path.
