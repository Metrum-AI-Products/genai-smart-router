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
    router-migrate
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

`metrum-smartrouterctl` is not a live cloud deployment driver. It includes a
credential-free local fake lifecycle for validating a strict reference-only
manifest before live adapters are approved. With a protected mode-`0600`
`file://` non-production profile, `plan` returns deterministic JSON without
creating state; `deploy` creates or resumes one normalized local job; exact-job
`status` is read-only; and `delete` requires an expiring mode-`0600` approval
that explicitly selects database and state-PVC retention. These commands do
not contact AWS, Kubernetes, RDS, DNS, a provider, or a production host.
The plan returns a stable instance ID separately from its intent-bound job ID.
New config revisions reconcile that instance in place, and superseded jobs
cannot delete resources managed by a newer lifecycle.


The CLI also maintains its established non-secret tenant inventory, independent
schema observations, fake quota admission, and bounded inventory status.
`promote`, `rollback`, protected-profile resolution, and all live adapters fail
closed until deployment policy and validation gates pass. See the
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

## Run The Migration Gate Before Service Start

For `server.usage_db.migration_policy: deployment-job`, use the packaged non-serving runner and its canonical `docs/DATA_MIGRATIONS.md` runbook before a fresh service start: `plan`, approved backup, `apply`, every required data job until its safe state is `validated`, `verify`, `status`, then serve. `auto-safe` is not a PostgreSQL production procedure. For PostgreSQL, provide the connection through the protected service environment and name it with `--dsn-env`; never put a DSN in a command line, ticket, or log. A successful checkpoint ordinal `0` does not establish service readiness. For SQLite, use exclusive downtime, an SQLite-safe backup, integrity verification, and free-space checks before the gate. The metrics-admin migration summary and authenticated read-only **Operations / Data migrations** report verify safe state after startup; they do not apply or reverse migrations.

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

Package rollback never runs a reverse migration. If the release migration contract is `restore-required`, restore the approved pre-migration snapshot before deploying the earlier binary. Otherwise preserve the usage database and restore only approved package/config inputs, then repeat migration verify/status and the same smokes before sending traffic.
