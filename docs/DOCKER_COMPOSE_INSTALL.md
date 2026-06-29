# Docker Compose Package Quick Install

This is the offline bootstrap path for a Docker Compose package. After startup, use the embedded `/docs/` site as the primary administrator reference.

## Prerequisites

- A Linux host matching the package architecture: `docker-linux-amd64` or `docker-linux-arm64`.
- Docker Engine and the Docker Compose plugin.
- A TLS reverse proxy or ingress in front of the router.
- A Metrum-issued `license.json`.
- A deployment-owned Postgres password and usage database policy.

## Bootstrap

```bash
tar -xzf smart-llmrouter-<version>-docker-linux-<arch>.tar.gz
cd smart-llmrouter-<version>-docker-linux-<arch>

docker load -i images/smart-llmrouter-<version>-linux-<arch>.tar

cd compose
mkdir -p config state logs
cp ../config/config.example.yaml config/config.yaml
cp ../config/env.example.json config/env.json
cp -R ../config/scripts config/scripts
```

Review `compose/.env`. Set a strong `POSTGRES_PASSWORD`, set `ROUTER_USAGE_DB_DSN` to the same password, and keep `SMART_LLMROUTER_VERSION` pinned to the image tag from this package. Do not use `latest`.

Place the issued license at `config/license.json`, matching `server.license.path` in `config/config.yaml`. Put provider credentials in `config/env.json` or the deployment secret manager.

Generate a caller token from the packaged image and add the generated caller entry to `config/config.yaml`:

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

Set permissions for the container runtime user used by the packaged image:

```bash
sudo chown -R 65532:65532 config state logs
chmod 0750 config config/scripts state logs
chmod 0400 config/env.json config/license.json
```

Start the service:

```bash
docker compose config >/dev/null
docker compose up -d
docker compose ps
```

The package includes a bundled Postgres service for Compose deployments. If administrators need host-local database access for maintenance, use the packaged localhost-only override so the database binds to `127.0.0.1`, not a public interface.

## Validate

```bash
export ROUTER_BASE_URL="https://router.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

If browser admin reports are enabled, verify the authorized report surface with the deployment's browser-admin identity:

```text
https://router.example.com/admin/reports/
```

## Upgrade And Rollback

Before upgrading, back up `compose/.env`, `compose/config/`, `compose/state/`, Postgres data, and logs according to the deployment policy. Load the new image tar, update `SMART_LLMROUTER_VERSION`, review config changes, run `docker compose config`, then recreate the router.

Rollback is restoring the previous package image tag, config, license inputs, and durable state or database snapshot, then rerunning `/readyz`, `/docs/`, `/v1/models`, and one caller smoke.
