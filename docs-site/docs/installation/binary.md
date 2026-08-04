---
title: Binary Install
doc_type: howto
---

# Binary Install

The binary package is for teams that already operate Linux services with their own process supervisor, database, log pipeline, and TLS proxy.

For package selection and architecture guidance, start with [Deployment Artifacts](./deployment-artifacts). For package inspection and security checks, see [Package Validation And Security Checks](./package-validation).

## Package Layout

```text
smart-llmrouter-<version>-linux-<arch>/
  bin/
    router
    router-token-gen
    router-usage-report
    metrum-smartrouterctl
  config/
    config.example.yaml
    env.example.json
    scripts/
      router.ts
  caddy/
    Caddyfile
  docs/
```

Use `linux-amd64` for x86_64 hosts and `linux-arm64` for ARM64 hosts. Release validation checks the package binaries against the selected architecture and rejects unexpected files, deployment-private notes, raw secrets, local state, and macOS archive metadata.

Create a dedicated service account, then create deployment-owned directories:

```bash
sudo groupadd --system router
sudo useradd --system --gid router --home-dir /var/lib/smart-llmrouter --shell /usr/sbin/nologin router
sudo install -d -m 0750 -o router -g router /etc/smart-llmrouter
sudo install -d -m 0750 -o router -g router /var/lib/smart-llmrouter
sudo install -d -m 0750 -o router -g router /var/log/smart-llmrouter
```

If the deployment uses a different service account, substitute that account consistently in the install commands and process supervisor configuration.

Create reviewed runtime files from the shipped templates before installing them:

```bash
cp config/config.example.yaml config/config.yaml
cp config/env.example.json config/env.json

bin/router-token-gen generate \
  --owner-user example-admin \
  --project example-project \
  --env prod \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Save the printed raw token for the caller through an approved secret channel. In `config/config.yaml`, add or verify the referenced `users`, `projects`, and `project_memberships` entries, then replace the placeholder caller token hashes with the generated `callers:` entry. Populate `config/env.json` or the service environment with provider credentials before startup.

Edit runtime paths in `config/config.yaml` for the binary host layout before installing the file:

```yaml
server:
  logging:
    path: /var/log/smart-llmrouter/requests.jsonl
  license:
    enabled: true
    path: /etc/smart-llmrouter/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
  usage_db:
    enabled: true
    driver: sqlite
    path: /var/lib/smart-llmrouter/usage.sqlite

state_path: /var/lib/smart-llmrouter/router-state.json
```

Use `driver: postgres` and a deployment-owned DSN instead of SQLite when the binary service is part of a production database deployment.

Install the binary and runtime files according to the host change-control process:

```bash
sudo install -m 0755 bin/router /usr/local/bin/smart-llmrouter
sudo install -m 0755 bin/router-token-gen /usr/local/bin/router-token-gen
sudo install -m 0755 bin/router-usage-report /usr/local/bin/router-usage-report
sudo install -m 0755 bin/metrum-smartrouterctl /usr/local/bin/metrum-smartrouterctl
sudo install -m 0640 -o router -g router config/config.yaml /etc/smart-llmrouter/config.yaml
sudo install -m 0640 -o router -g router config/env.json /etc/smart-llmrouter/env.json
sudo install -m 0640 -o router -g router license.json /etc/smart-llmrouter/license.json
```

`metrum-smartrouterctl` is not a live deployment driver. Its shipped commands
maintain a non-secret local tenant registry, record independent schema
observations, perform fake-adapter-only quota admission, and return bounded
registry status. `deploy`, `promote`, and `rollback` fail closed while cloud,
Kubernetes, DNS, durability, and network policy gates remain disabled. See the
[multi-environment operator contract](../operations/deployment-patterns#multi-environment-operator-contract)
before installing or invoking this optional administration-host CLI.

## Runtime Configuration

The service process needs access to:

- `config.yaml`;
- the provider credential env file;
- a Metrum-issued `license.json`;
- durable license state;
- the usage database DSN;
- optional routing script files and helper dependencies already packaged on disk.

Example service command:

```bash
smart-llmrouter \
  --config /etc/smart-llmrouter/config.yaml
```

The router loads `env.json` from the same directory as the config file before expanding `${VAR}` references. Deployments that use a secret manager can inject the same environment variables into the service process instead.

Do not configure the router to download code or packages at runtime. TypeScript policy dependencies and helper files must be packaged before deployment.

## Validate

```bash
export ROUTER_BASE_URL="https://<router-host>"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS "$ROUTER_BASE_URL/version"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Expected results:

- `/readyz` returns success only when required runtime checks pass, including license enforcement.
- `/docs/` serves the embedded product documentation from the running binary.
- `/version` returns safe release metadata.
- `/v1/models` returns only model groups allowed for the caller token.

For request-level diagnostics after installation, use [Troubleshooting Requests](../troubleshooting/requests).

## Upgrade And Rollback

Before an upgrade, back up `config.yaml`, `env.json` or equivalent secret-manager state, `license.json`, license state, router state, usage database data, logs needed by the retention policy, and the previous package artifact.

Install the new package beside the old package, run `smart-llmrouter --version` or `bin/router --version`, review config template changes, then restart the supervised service with the new binary. After restart, repeat `/readyz`, `/docs/`, `/v1/models`, and one caller smoke.

Rollback is restoring the previous binary package, config, license inputs, and compatible state or database snapshot, then rerunning the same smokes before sending production traffic.
