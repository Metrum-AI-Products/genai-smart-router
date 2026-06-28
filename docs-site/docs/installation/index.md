---
title: Installation
---

# Installation

GenAI Smart Router is installed from a release package. The package contains the router runtime and embedded product documentation; it does not require the source repository on the target host.

Use this section for customer-managed deployments where an operator receives a binary or Docker Compose release artifact from Metrum.

## Choose A Package

| Deployment shape | Use when | Start here |
|---|---|---|
| Docker Compose | The deployment host can run Docker and Compose, and the team wants the packaged router plus managed Postgres service. | [Docker Compose Install](/docs/installation/docker-compose) |
| Linux binary | The team manages the process supervisor, database, TLS proxy, and filesystem layout directly. | [Binary Install](/docs/installation/binary) |

Both deployment shapes use the same runtime configuration model:

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
5. Verify `/readyz`, `/version`, `/v1/models`, and one caller request.
6. Enable admin reports, metrics scraping, and log collection only for authorized operational subjects.
7. Record the deployed router version, build timestamp, config checksum, and rollback artifact in the deployment change record.

## Smoke Commands

Use placeholders in automation and support examples:

```bash
export ROUTER_BASE_URL="https://llm-api.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/version"

curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

For license setup and renewal, see [Licensing](/docs/licensing). For operational metrics and request tracing, see [Observability](/docs/operations/observability). For failed smoke tests, see [Troubleshooting](/docs/troubleshooting).
