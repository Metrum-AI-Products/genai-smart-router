# Package Bootstrap

This package contains a packaged GenAI Smart Router runtime plus a small offline documentation set. The full external administrator documentation is embedded in the router binary and is available at `/docs/` after the service starts.

## What Is Included

Binary packages include:

- `bin/router`
- `bin/router-token-gen`
- `bin/router-usage-report`
- `bin/router-migrate`
- `bin/smartrouterctl`
- `bin/metrum-fleetctl`
- `bin/metrum-smartrouterctl` (one-release rename notice)
- `config/config.example.yaml`
- `config/env.example.json`
- `config/scripts/router.ts`
- `caddy/Caddyfile`
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
- `images/smart-llmrouter-<version>-linux-<arch>.tar`
- `docs/`

The saved Docker image includes `/app/bin/router-migrate` and the customer-local
`/app/bin/smartrouterctl`; Fleet lifecycle binaries are excluded. Version-check
the runtime with `docker run --rm --entrypoint /app/bin/smartrouterctl
smart-llmrouter:<version>-linux-<arch> version`.

The standard Docker and Docker Compose images do not include
`metrum-fleetctl` or the compatibility command. Docker-based Fleet operators run
`metrum-fleetctl` from a binary package on a separate trusted administration host.

Choose `linux-amd64` for x86_64 hosts and `linux-arm64` for ARM64 hosts.

## Start Here

Use the quick-start document that matches the package:

- `BINARY_INSTALL.md` for Linux binary packages.
- `DOCKER_COMPOSE_INSTALL.md` for Docker Compose packages.
- `KUBERNETES_INSTALL.md` for Kubernetes deployment planning.
- `PACKAGE_VALIDATION.md` for package-content and runtime-health checks.

After the router is reachable, open:

```text
https://router.example.com/docs/
```

The embedded docs include the complete install, configuration, license, provider key, caller token, reporting, troubleshooting, upgrade, rollback, and security guidance.

## Required Deployment Inputs

Prepare these deployment-owned files before first startup:

- `config.yaml` copied from `config/config.example.yaml` and reviewed for the deployment.
- `env.json` copied from `config/env.example.json` or equivalent environment variables from a secret manager.
- A Metrum-issued `license.json` for release builds.
- A durable state directory for license state, router state, logs, and usage data.
- At least one caller token generated with `router-token-gen`.

Do not store provider keys, raw router tokens, license payloads, token hashes, full production configs, or private host details in tickets, public docs, screenshots, or package notes.

For help with package selection or rollout planning, contact `contact@metrum.ai`.
