# Binary Package Quick Install

This is the offline bootstrap path for a Linux binary package. After startup, use the embedded `/docs/` site as the primary administrator reference.

## Prerequisites

- A Linux host matching the package architecture: `linux-amd64` or `linux-arm64`.
- A service supervisor such as systemd, runit, or an equivalent platform supervisor.
- A TLS reverse proxy such as Caddy, nginx, or a managed load balancer.
- Deployment-owned config, secret, state, log, and usage database locations.
- A Metrum-issued `license.json`.

## Bootstrap

```bash
tar -xzf smart-llmrouter-<version>-linux-<arch>.tar.gz
cd smart-llmrouter-<version>-linux-<arch>

bin/router --version
cp config/config.example.yaml config/config.yaml
cp config/env.example.json config/env.json
```

Edit `config/config.yaml` for the deployment:

- set `server.listen`;
- set `server.license.path` and `server.license.state_path`;
- set `state_path`;
- configure `server.usage_db`;
- configure providers, model groups, and caller access;
- point TypeScript routing to packaged script paths only when used.

Populate `config/env.json` or the process environment with provider credentials. Keep credentials server-side and restrict file permissions.

Generate a caller token and add the generated caller entry to `config/config.yaml`:

```bash
bin/router-token-gen generate \
  --owner-user example-admin \
  --project example-project \
  --env prod \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Before starting a `deployment-job` router, version-check and run the non-serving migration gate: `plan`, approved backup, `apply`, `verify`, then `status`. PostgreSQL receives its connection only through `--dsn-env`; `auto-safe` is not a PostgreSQL production procedure.

```bash
bin/router-migrate --version
bin/router-migrate --driver=postgres --dsn-env=ROUTER_USAGE_DB_DSN --action=plan --json
# Take and approve the deployment backup before continuing.
bin/router-migrate --driver=postgres --dsn-env=ROUTER_USAGE_DB_DSN --action=apply --json
bin/router-migrate --driver=postgres --dsn-env=ROUTER_USAGE_DB_DSN --action=resume --job=historical-usage-validation-v1 --checkpoint-ordinal=0 --json
bin/router-migrate --driver=postgres --dsn-env=ROUTER_USAGE_DB_DSN --action=verify --json
bin/router-migrate --driver=postgres --dsn-env=ROUTER_USAGE_DB_DSN --action=status --json
```

Complete every release-defined data job before verification; the current package begins `historical-usage-validation-v1` at checkpoint ordinal `0`. Start the router in the foreground only after compatible final status:

```bash
bin/router --config config/config.yaml
```

## Validate

From a client-like network location:

```bash
export ROUTER_BASE_URL="https://router.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Then run one small request against a model group returned by `/v1/models`:

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

## Upgrade And Rollback

Before upgrading, back up `config.yaml`, `env.json`, `license.json`, license state, router state, usage database data, and logs according to the deployment policy. Install the new package beside the old one, run `bin/router --version`, review config changes, then restart the supervised service.

Package rollback never runs a reverse migration. For a `restore-required` release contract, restore the approved pre-migration database snapshot before deploying the earlier package; otherwise preserve the usage database and restore only approved package/config inputs. Rerun migration verify/status, `/readyz`, `/docs/`, `/v1/models`, and one caller smoke.
