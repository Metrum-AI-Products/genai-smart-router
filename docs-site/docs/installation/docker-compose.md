---
title: Docker Compose Install
doc_type: howto
---

# Docker Compose Install

The Docker Compose package is the usual customer-managed installation path. It includes a prebuilt router image tarball, Compose files, and example runtime configuration. The target host needs Docker Engine and Docker Compose, but it does not need Go, Node.js, Docusaurus, or the source repository.

For package selection and architecture guidance, start with [Deployment Artifacts](./deployment-artifacts). For package inspection and security checks, see [Package Validation And Security Checks](./package-validation).

## Package Layout

A release package follows this shape:

```text
smart-llmrouter-<version>-docker-linux-<arch>/
  compose/
    docker-compose.yml
    docker-compose.postgres-localhost.yml
    Caddyfile.compose
    .env
    .env.example
  config/
    config.example.yaml
    env.example.json
    scripts/
      router.ts
  images/
    smart-llmrouter-<version>-linux-<arch>.tar
  docs/
```

The shipped Compose file bind-mounts `./config`, `./state`, and `./logs` relative to the `compose/` directory. Prepare those runtime directories from the package templates before starting the service.

Use `docker-linux-amd64` for x86_64 hosts and `docker-linux-arm64` for ARM64 hosts. Release validation checks that the package has exactly one image tar for the selected architecture, required compose/config/docs files, required router binaries in the saved image layers, and no AppleDouble metadata, internal runbooks, raw secrets, or local state files.

```bash
cd smart-llmrouter-<version>-docker-linux-<arch>
docker load -i images/smart-llmrouter-<version>-linux-<arch>.tar

cd compose
mkdir -p config state logs
cp ../config/config.example.yaml config/config.yaml
cp ../config/env.example.json config/env.json
cp -R ../config/scripts config/scripts
```

Review `.env` or copy `.env.example` to `.env` if your package does not include a populated `.env`. Fill deployment-owned values, use a strong database password, and pin `SMART_LLMROUTER_VERSION` to the exact image tag from the package. Do not use `latest`.

Generate at least one caller token and replace the placeholder caller hashes in `config/config.yaml` before first startup:

```bash
docker run --rm \
  --entrypoint /app/bin/router-token-gen \
  smart-llmrouter:<version>-linux-<arch> \
  generate \
  --owner-user example-admin \
  --project example-project \
  --env prod \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Save the printed raw token for the caller through an approved secret channel. In `config/config.yaml`, add or verify the referenced `users`, `projects`, and `project_memberships` entries, then replace the placeholder `callers:` entries with the generated caller YAML. A fresh template with `REPLACE_WITH_SHA256_HEX_OF_*` values is intentionally not startable.

Edit `config/config.yaml` for container paths and the packaged Postgres service before startup:

```yaml
server:
  listen: ":8080"
  logging:
    path: /app/logs/requests.jsonl
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}

state_path: /app/state/router-state.json
```

## Configure Secrets

Place provider credentials in `config/env.json`. Keep this file outside source control and restrict filesystem permissions.

```json
{
  "OPENAI_API_KEY": "replace-with-provider-key",
  "OPENROUTER_API_KEY": "replace-with-provider-key"
}
```

Place the issued `license.json` in `config/license.json`, matching `server.license.path`. Keep `server.license.state_path` on durable storage.

Then set ownership and permissions for the container runtime user. The packaged image runs as UID/GID `65532`; the config directory must be readable and traversable, and state/log directories must be writable by that ID.

```bash
sudo chown -R 65532:65532 config state logs
chmod 0750 config config/scripts state logs
chmod 0400 config/env.json config/license.json
```

## Start And Validate

```bash
docker compose config >/dev/null
docker compose up -d
docker compose ps
```

Then verify from a network location that represents the intended clients:

```bash
export ROUTER_BASE_URL="https://llm-api.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS "$ROUTER_BASE_URL/version"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Run one small completion through an allowed model group:

```bash
curl -fsS "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "replace-with-allowed-model-group",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 16
  }'
```

## Operational Notes

- Terminate TLS at a deployment-managed reverse proxy.
- Keep Postgres private to the deployment network unless a reviewed localhost-only administration override is used.
- Scrape `/metrics` only with a caller authorized for metrics administration.
- Keep `compose/config/`, `compose/state/`, database volumes, logs, and backups under the deployment's secret-handling policy.
- Back up `config.yaml`, `license.json`, license state, and the usage database before upgrades.

## Upgrade And Rollback

Before an upgrade, back up `compose/.env`, `compose/config/`, `compose/state/`, Postgres data, logs needed by the retention policy, and the previous package artifact. Load the new image tar, update `SMART_LLMROUTER_VERSION` to the packaged image tag, review config template changes, run `docker compose config`, then recreate the router service.

After restart, repeat `/readyz`, `/docs/`, `/v1/models`, one caller smoke, and an admin report smoke when reports are enabled.

Rollback is restoring the previous image tag, Compose files, config, license inputs, and compatible state or database snapshot, then rerunning the same smokes before sending production traffic.

For release-to-release sequencing, see [Release Notes And Upgrades](../release-notes/upgrade-guide).
