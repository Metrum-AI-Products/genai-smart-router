# Package Bootstrap

This package contains a packaged Metrum AI Router runtime plus a small offline documentation set. The full external administrator documentation is embedded in the router binary and is available at `/docs/` after the service starts.

## What Is Included

Binary packages include:

- `bin/metrum-router`
- `bin/metrum-router-token-gen`
- `bin/metrum-router-usage-report`
- `bin/metrum-router-migrate`
- `bin/metrum-routerctl`
- `bin/metrum-genai-smartrouter-fleetctl`
- `bin/metrum-genai-smartrouter-fleet-sign`
- `bin/metrum-genai-smartrouter-license` (operator-side runtime-policy key and
  license tool; never included in the runtime image)
- `bin/metrum-genai-customer-lifecycle` (Metrum operator lifecycle CLI only; never for customer self-service)
- `bin/router` (one-release rename notice → `metrum-router`)
- `bin/router-token-gen` (one-release rename notice → `metrum-router-token-gen`)
- `bin/router-usage-report` (one-release rename notice → `metrum-router-usage-report`)
- `bin/router-migrate` (one-release rename notice → `metrum-router-migrate`)
- `bin/metrum-genai-smartrouterctl` (one-release rename notice → `metrum-routerctl`)
- `bin/smartrouterctl` (one-release rename notice → `metrum-routerctl`)
- `bin/metrum-fleetctl` (one-release rename notice → `metrum-genai-smartrouter-fleetctl`)
- `bin/metrum-smartrouterctl` (one-release rename notice → `metrum-genai-smartrouter-fleetctl`)
- `bin/metrum-fleet-sign` (one-release rename notice → `metrum-genai-smartrouter-fleet-sign`)
- `bin/router-license` (one-release rename notice → `metrum-genai-smartrouter-license`)
- `config/config.example.yaml`
- `config/env.example.json`
- `config/enterprise-license-skus.json`
- `config/scripts/router.ts`
- `caddy/Caddyfile`
- `LICENSE`
- `NOTICE`
- `THIRD_PARTY_NOTICES.md`
- `MODEL_LICENSES.md`
- `docs/`

Docker Compose packages include:

- `compose/docker-compose.yml`
- `compose/docker-compose.postgres-localhost.yml`
- `compose/Caddyfile.compose`
- `compose/.env.example`
- `compose/.env`
- `config/config.example.yaml`
- `config/env.example.json`
- `config/scripts/router.ts`
- `images/metrum-router-<version>-linux-<arch>.tar`
- `docs/`
- `LICENSE`
- `NOTICE`
- `THIRD_PARTY_NOTICES.md`
- `MODEL_LICENSES.md`

The saved Docker image includes `/app/bin/metrum-router`, `/app/bin/metrum-router-migrate`,
customer-local `/app/bin/metrum-routerctl`, and one-release rename notices for the
previous `router*` / `metrum-genai-smartrouterctl` / `smartrouterctl` names; Fleet
lifecycle binaries are excluded. Version-check the runtime with
`docker run --rm --entrypoint /app/bin/metrum-routerctl
metrum-router:<version>-linux-<arch> version`.

The standard Docker and Docker Compose images do not include
`metrum-genai-smartrouter-fleetctl`, `metrum-genai-smartrouter-fleet-sign`,
`metrum-genai-smartrouter-license`, or the one-release Fleet rename notices.
Docker-based Fleet operators run those tools from a binary package on a separate trusted administration host.
Fleet CLIs are distributed as prebuilt binaries only; operator hosts must not require a Go toolchain or product source tree.
Self-managed operators generate and retain their own Ed25519 keypair and use
`metrum-genai-smartrouter-license` from the trusted administration host to
issue the deployment's runtime-policy `license.json`. Metrum-managed issuance
may be offered as an optional commercial deployment service, but it is not
required by the open-source runtime. Keep private keys and real licenses out of
the package, runtime image, source control, logs, and tickets.

Docker and Compose packages use SQLite state with one Router container and one
replica by default; they neither provision nor bind RDS. Dedicated RDS is an
optional, binary-package-only Fleet branch selected by an explicit approved
`database_profile`. Its first disposable non-production E2E requires a
strictly scoped external admission file that Fleet consumes but never creates;
the follow-on review may be a single-maintainer self-review.

Choose `linux-amd64` for x86_64 hosts and `linux-arm64` for ARM64 hosts.

## Start Here

Use the quick-start document that matches the package:

- `BINARY_INSTALL.md` for Linux binary packages.
- `DOCKER_COMPOSE_INSTALL.md` for Docker Compose packages.
- `KUBERNETES_INSTALL.md` for Kubernetes deployment planning.
- `PACKAGE_VALIDATION.md` for package-content and runtime-health checks.
- `LICENSE.md` for the Apache License 2.0 terms covering all first-party
  content. No separate EULA applies.

At the package root, retain `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and
`MODEL_LICENSES.md` together. `LICENSE` governs first-party content;
`NOTICE` carries distributed notices; `THIRD_PARTY_NOTICES.md` records the
dependency and asset terms and distribution surfaces; and `MODEL_LICENSES.md`
records the boundaries for separately obtained models and datasets. The
inventory files do not turn unresolved terms into permissions, so review them
for the exact artifact and intended redistribution.

After the router is reachable, open:

```text
https://router.example.com/docs/
```

The embedded docs include the complete install, configuration, license, provider key, caller token, reporting, troubleshooting, upgrade, rollback, and security guidance.

## Required Deployment Inputs

Prepare these deployment-owned files before first startup:

- `config.yaml` copied from `config/config.example.yaml` and reviewed for the deployment.
- `env.json` copied from `config/env.example.json` or equivalent environment variables from a secret manager.
- An operator-generated `license.json` for operator-selected runtime policy in release
  builds. It is not a copyright or commercial-use license condition and does
  not narrow the rights granted by Apache-2.0 or replace the package-root legal
  files.
- A durable state directory for license state, router state, logs, and usage data.
- At least one caller token generated with `router-token-gen`.

Do not store provider keys, raw router tokens, license payloads, token hashes, full production configs, or private host details in tickets, public docs, screenshots, or package notes.

For non-sensitive package questions, use the repository's public question issue
form. Report vulnerabilities through the private path in `SECURITY.md`.
