---
title: Installation
doc_type: howto
---

# Installation

GenAI Smart Router is installed from a release package. The package contains the router runtime and embedded product documentation; it does not require build tooling on the target host.

Use `installation/` for work before the first production request lands: artifact selection, package validation, license and config placement, security review, and first smoke tests. Use `operations/` for work after traffic lands: scaling, runtime tuning, observability, reporting, and recurring troubleshooting.

Release artifacts are validated before handoff. The package validator rejects platform archive metadata, internal runbooks, unexpected files, local state, raw secrets, and architecture mismatches; Docker Compose packages also include a saved image tar for the selected architecture.

## Choose A Deployment Shape

Read the matrix left to right as an ownership checklist. The deployment shape decides who terminates TLS, who operates Postgres, how upgrades roll out, where telemetry is handed off, and how customer or team environments are isolated. If a row assigns a responsibility to the customer platform, make sure that owner is named in the rollout plan before package handoff.

| Deployment shape | TLS termination responsibilities | Database responsibilities | Upgrade flow | Operational telemetry handoff | Isolation model | Recommended use case | Known limitations |
|---|---|---|---|---|---|---|---|
| Docker Compose | Deployment-owned reverse proxy or packaged Caddy example; production TLS policy remains customer-owned. | Compose can run router-managed Postgres for simple deployments; production teams may still point at customer-managed Postgres. | Load the packaged image tar, set `SMART_LLMROUTER_VERSION`, run `docker compose up -d`, then smoke. | Container logs, optional host Prometheus scrape, and usage DB reports; host log shipping is customer-owned. | One router config governs callers, projects, environments, model groups, and admin domains. | Fast customer-managed install on one host with packaged runtime and a bundled database option. | Single-host operating model unless the customer adds external Postgres, load balancing, and state planning. |
| Linux binary | Customer-owned TLS proxy or service mesh in front of the binary. | Customer-managed Postgres DSN and filesystem state. | Replace binaries under the process supervisor, restore config inputs, restart, then smoke. | Process logs, host log collection, optional Prometheus, and usage DB reports. | One process/config boundary; use separate instances for hard environment or team isolation. | Environments with existing supervisors, hardened host images, database standards, and TLS infrastructure. | More customer-owned wiring for service files, logs, filesystem permissions, and rollback. |
| Kubernetes | Cluster ingress, Gateway API, service mesh, or external load balancer terminates TLS. | External Postgres is recommended; Secrets or external secret managers provide DSNs and credentials. | Push immutable image tag, render reviewed manifests, run dry-run, apply, watch rollout, then smoke. | Pod logs, cluster Prometheus, optional OTel collector, admin reports, and usage DB reports. | Namespace, RBAC, NetworkPolicy, Secrets, and separate router instances for stronger tenant boundaries. | Platform teams standardizing router deployment inside cluster-native controls. | Requires reviewed manifests, registry flow, secret management, network policy, and database operations. |
| Metrum-managed instance | Metrum-managed endpoint and TLS for the contracted evaluation or dedicated service. | Defined in the managed-service plan; report exports and data boundaries are contract-specific. | Metrum-managed rollout with customer acceptance smokes and documented rollback evidence. | Agreed report extracts, request IDs, and operational summaries; customer receives safe evidence, not private host access. | Dedicated deployment or evaluation endpoint according to the contract. | Fast evaluation, pilot, or private managed deployment when customer infrastructure is not the first step. | Operational knobs, provider custody, retention, and network controls depend on the managed-service agreement. |

Start here by shape:

- [Deployment Artifacts](/docs/installation/deployment-artifacts) for artifact selection and package contents.
- [Docker Compose Install](/docs/installation/docker-compose) for single-host Compose installation.
- [Binary Install](/docs/installation/binary) for service-supervisor installation.
- [Deploy To Kubernetes](/docs/installation/kubernetes) for cluster-owned manifests and rollout.
- [Package Validation And Security Checks](/docs/installation/package-validation) for handoff and audit checks.

All deployment shapes use the same runtime configuration model:

- router YAML config for providers, model groups, callers, usage storage, limits, and admin surfaces;
- environment-backed provider credentials;
- a Metrum-issued `license.json` for normal release builds;
- durable state for license checks and usage data;
- a TLS-terminating reverse proxy in front of the router for production traffic.

## Required Inputs

Before installation, collect the following deployment-owned values:

| Input | Example placeholder | Notes |
|---|---|---|
| Router base URL | `https://llm-api.example.com` | Public or private endpoint exposed to clients. |
| Router config path | `/app/config/config.yaml` | Container path or host path, depending on package type. |
| Provider key env file | `/app/config/env.json` | Stores provider credentials outside the public docs and outside source control. |
| License file | `/app/config/license.json` | Issued by Metrum; do not edit its contents. |
| License state path | `/app/state/license-state.json` | Must survive restarts. |
| Usage database DSN | `postgres://router:replace-with-password@db:5432/router?sslmode=disable` | Use a strong deployment-owned password. |
| Admin identity | `basic:admin` or OIDC subject | Required for reports, license status, and operational APIs. |

Do not place raw router tokens, provider API keys, token hashes, private signing material, or full production config files in public tickets, public docs, release notes, or browser screenshots.

## Installation Flow

1. Unpack the release package for the host architecture.
2. Create the config, state, and log directories with permissions limited to deployment operators.
3. Install `config.yaml`, `env.json`, and the issued `license.json`.
4. Start the router with Docker Compose or the local process supervisor.
5. Verify `/readyz`, `/docs/`, `/version`, `/v1/models`, and one caller request.
6. Enable admin reports, metrics scraping, and log collection only for authorized operational subjects.
7. Record the deployed router version, build timestamp, config checksum, and rollback artifact in the deployment change record.

## Smoke Script

Use placeholders in automation and support examples:

```bash
export ROUTER_BASE_URL="https://llm-api.example.com"
export ROUTER_TOKEN="replace-with-router-token"

router_smoke() {
  : "${ROUTER_BASE_URL:?set ROUTER_BASE_URL}"
  : "${ROUTER_TOKEN:?set ROUTER_TOKEN}"

  curl -fsS "$ROUTER_BASE_URL/readyz"
  curl -fsS "$ROUTER_BASE_URL/version"
  curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
    "$ROUTER_BASE_URL/v1/models"
}

router_smoke
```

After the core smoke passes, run one small request through every API shape in scope: OpenAI Chat, OpenAI Responses or Codex CLI, Anthropic Messages or Claude Code, tool calls, image/VLM requests, streaming, and structured outputs where advertised.

## Rollback Checklist

Restore inputs in this order, then rerun the smoke script:

1. Stop or drain new traffic at ingress, load balancer, supervisor, Compose, or Kubernetes rollout controls.
2. Restore the previous router binary or image tag.
3. Restore the previous `config.yaml`.
4. Restore the previous provider-key environment file, normally `env.json`, from the approved secret store or backup.
5. Restore the previous `license.json` only when the rollback requires the prior license envelope.
6. Restore license state and router state files from trusted backup if the failed rollout changed state compatibility.
7. Restore or point back to the previous usage database backup when a migration or DSN change caused the failure.
8. Restart or roll out the previous runtime and verify `/readyz`, `/version`, `/v1/models`, one caller request, admin reports when enabled, and metrics-admin access when configured.

For license setup and renewal, see [Licensing](/docs/licensing). For operational metrics and request tracing after traffic starts, see [Observability](/docs/operations/observability). For failed smoke tests, see [Troubleshooting](/docs/troubleshooting).
