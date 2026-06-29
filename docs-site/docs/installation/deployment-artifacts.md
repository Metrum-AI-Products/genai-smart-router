---
title: Deployment Artifacts
doc_type: howto
---

# Deployment Artifacts

GenAI Smart Router is delivered as release artifacts. Deployment hosts do not need the source checkout, Go, Node.js, Docusaurus, or build scripts.

## Artifact Types

| Artifact | Use when | Contents |
|---|---|---|
| Linux binary package | The platform team manages the service supervisor, TLS proxy, database, and filesystem layout. | Router binaries, config templates, Caddy example, package-safe bootstrap docs. |
| Docker Compose package | The platform team wants a packaged router image tar plus Compose files and a bundled Postgres service. | Saved image tar, Compose files, config templates, package-safe bootstrap docs. |
| Kubernetes manifests or Helm | The platform team operates Kubernetes and supplies reviewed manifests. | Obtained as a reviewed deployment artifact when needed; see [Deploy To Kubernetes](./kubernetes). |

Use `linux-amd64` for x86_64 hosts and `linux-arm64` for ARM64 hosts, including ARM64 cloud instances. Docker packages use the same architecture suffix and include exactly one saved image tar for that architecture.

## Embedded Docs And Admin Assets

The router binary embeds this Docusaurus documentation site. After startup, browser requests to `/docs/` show the full external administrator docs, including installation, configuration, licensing, provider keys, caller tokens, reporting, troubleshooting, upgrade, rollback, and security guidance.

Authenticated admin report assets, when enabled, are embedded separately and served under `/admin/reports/`. They are not part of public `/docs/` and require browser-admin authentication plus authorization.

## Offline Package Docs

Release tarballs include only a small package-safe Markdown bootstrap set under `docs/`. These files explain what is in the package, how to start it far enough to reach `/docs/`, validation commands, and support contact paths.

They intentionally do not include private production runbooks, source-maintenance notes, full source documentation, raw secrets, private host paths, SSH procedures, or full production configs.

## Package Selection Checklist

- Match the artifact architecture to the host CPU.
- Choose binary install when the deployment already has a service supervisor, database, and TLS proxy.
- Choose Docker Compose when a packaged image tar and Compose-managed Postgres fit the operating model.
- Use Kubernetes only with reviewed deployment manifests and the canonical Kubernetes install guide.
- Confirm `/readyz`, `/docs/`, `/v1/models`, and one caller smoke before sending production traffic.

Next steps:

- [Install From Linux Binaries](./binary)
- [Run With Docker Compose](./docker-compose)
- [Deploy To Kubernetes](./kubernetes)
- [Package Validation And Security Checks](./package-validation)
