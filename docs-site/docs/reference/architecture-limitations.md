---
title: Architecture, Platforms, And Limitations
doc_type: reference
---

# Architecture, Platforms, And Limitations

GenAI Smart Router is a self-managed reverse proxy between authenticated AI
clients and operator-configured upstream providers or private model servers.

```mermaid
flowchart LR
  Client[Applications and agent clients] --> Ingress[TLS ingress]
  Ingress --> Router[GenAI Smart Router]
  Router --> Policy[Model-group eligibility and routing]
  Policy --> Provider[External provider APIs]
  Policy --> Private[Private OpenAI-compatible upstreams]
  Router --> State[(State and usage database)]
  Router --> Ops[Restricted metrics and admin reports]
```

The router authenticates callers, checks model-group access and request-shape
eligibility, selects a configured target, translates supported API dialects,
and records sanitized operational and usage data. Provider credentials stay on
the server side. Configuration remains deployment-owned.

## Supported Deployment Platforms

Release artifacts support Linux on `amd64` and `arm64`. The documented runtime
shapes are a standalone binary, Docker Compose, and Kubernetes. SQLite is the
single-writer default; PostgreSQL is the explicit choice for validated
multi-replica or externally managed database deployments. Operators provide
TLS ingress, DNS, storage, secret management, network policy, backups, and
provider accounts.

## Security Boundaries

Caller tokens and model allow lists protect proxy traffic. Administrative
reports require configured admin authentication and authorization. `/metrics`
is restricted to callers with metrics-admin permission; ordinary application
callers receive `403 metrics-forbidden`. Provider keys, caller secrets, token
hashes, prompts, raw images, tool payloads, and full configuration must not be
placed in public logs or issue reports.

The router does not replace host, cluster, database, ingress, identity-provider,
secret-manager, provider-account, or client security. Operators must validate
those controls and the exact upstream model/API combinations they enable.

## Explicit Limitations And Non-Goals

- Model-group names and provider availability are deployment-defined; the
  project does not guarantee access to any provider or model.
- Catalog metadata is not capability proof. Tools, images, API bridges, and
  large request shapes require direct upstream and router-level validation.
- The in-process response cache is per process and is cleared by restart.
- SQLite is not a shared multi-writer database and must not back horizontally
  scaled router replicas.
- Kubernetes examples are starting points, not a cluster installer or a
  substitute for deployment-specific security review.
- Hierarchical routing can be assembled through supported API boundaries, but
  there is no universal topology controller.
- The project does not provide provider uptime, model-quality, legal,
  compliance, or support-service guarantees.

See [Router Configuration](../configuration/router-config), [Security And
Governance](../security-governance/overview), and the [Upgrade
Guide](../release-notes/upgrade-guide).

