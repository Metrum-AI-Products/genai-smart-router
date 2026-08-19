---
title: Customer Administrator Guide
doc_type: howto
---

# Customer Administrator Guide

This guide is for **customer platform administrators**: the people who own router configuration, provider credentials, model groups, caller access, and routine operational changes for one deployment.

It covers two common shapes:

| Deployment shape | Who applies live changes | Primary tools |
| --- | --- | --- |
| **Self-hosted** (Compose, binary, or customer-managed Kubernetes) | Your team | `config.yaml`, `env.json`, package restart or rollout |
| **Private managed Kubernetes (Fleet SQLite)** | Your team prepares config; activation uses the Fleet customer CLI from a release binary package | `metrum-genai-smartrouter-fleetctl customer …`, `router-token-gen` |

Model group names, hostnames, and allow lists are **deployment-defined**. Call `/v1/models` with each caller token to confirm what that token may request.

For token format and identity rules, see [User Key Generation](./key-generation). For caller config fields, see [Caller Tokens](../configuration/caller-tokens). For installation mechanics, see [Installation](../installation/). For Fleet topology and operator contracts, see [Enterprise Deployment Patterns](./deployment-patterns).

## Prepare Initial Configuration

Every deployment needs three protected artifacts:

1. **`config.yaml`** — model groups, provider catalog metadata, routing targets, callers, users, projects, admin auth, and usage settings. Start from the packaged `config.example.yaml` in your release and adapt it to your deployment.
2. **`env.json`** — provider API keys and other upstream credentials as environment variable names and values. Keep this file mode `0600` and out of version control.
3. **`license.json`** — Metrum-issued license file mounted at the path configured in `server.license.path`.

Before first production traffic:

- Define `users`, `projects`, and `project_memberships` for every caller you plan to issue.
- Define `models.*` groups and `providers.*.models` catalog entries for the upstream routes you will activate.
- Choose SQLite or PostgreSQL usage storage according to your installation path ([Docker Compose](../installation/docker-compose), [Binary](../installation/binary), [Kubernetes](../installation/kubernetes)).

Validate YAML/JSON syntax locally. On self-hosted installs you may also run:

```bash
metrum-genai-smartrouterctl config validate --config /path/to/config.yaml
```

(`metrum-genai-smartrouterctl` validates and diffs local config; it does not activate config on a managed hostname by itself.)

## Self-Hosted: First Start And Restart

### Docker Compose or binary

1. Place reviewed `config.yaml`, `env.json`, and `license.json` in the protected paths defined by your install layout.
2. Start or restart the router service (for example `docker compose restart router` or your systemd unit).
3. Confirm health:

```bash
curl -fsS https://<your-router-host>/readyz
```

### Customer-managed Kubernetes

Follow [Deploy To Kubernetes](../installation/kubernetes): apply manifests, mount config/license/secrets, wait for `/readyz`, then run acceptance smokes.

Config changes require updating the mounted config Secret or ConfigMap and rolling the Deployment. Use **`Recreate`** strategy when the router uses a single `ReadWriteOnce` SQLite PVC so only one pod mounts state during rollout.

## Self-Hosted: Update Configuration And Restart

1. Edit `config.yaml` (and `env.json` when provider keys change) on the deployment host or through your config-management pipeline.
2. Validate syntax and deployment-specific checks.
3. Apply the change:
   - **Compose / binary:** restart the router process or container.
   - **Kubernetes:** update the mounted config artifact and roll the Deployment.
4. Re-run smokes: `/readyz`, `/v1/models` with a representative caller token, one chat completion, and any client surfaces you rely on (Codex CLI, Claude Code, tools, images).

Keep timestamped backups of the prior config before each change. Roll back by restoring the backup and restarting.

## Issue Router API Keys (Caller Tokens)

Router **caller tokens** are bearer tokens applications use as `Authorization: Bearer <token>`. The router stores only hashes in config.

### Generate a token and caller row

Use `router-token-gen` from the release binary package (or `metrum-genai-smartrouterctl callers generate` on the deployment host):

```bash
router-token-gen generate \
  --owner-user <user-id> \
  --project <project-id> \
  --env <environment> \
  --allow <group>[,<group>...] \
  --format json
```

The output includes:

- the **raw token** (distribute once to the application owner);
- a public **`token_id`** for reports; and
- a **`caller`** JSON object to merge into `config.yaml` under `callers:`.

Ensure `users`, `projects`, and `project_memberships` exist and are active for that owner/project pair before enabling the caller.

### Self-hosted activation

1. Append the generated caller row to `callers:` in `config.yaml` (or merge with your config controller).
2. Restart or roll the router.
3. Ask the caller to run `/v1/models` and confirm the allowed groups appear.

### Rotate or suspend a token

1. Generate a new token and caller row (or set `status: rotated` / `disabled` on the old caller).
2. Apply config and restart.
3. Confirm the old token fails and the new token succeeds on `/v1/models`.

Never commit raw tokens, token hashes, or provider keys to git, tickets, or chat.

## Private Managed Kubernetes (Fleet SQLite)

In a **private managed** deployment, Metrum or your operator runs the router on dedicated Kubernetes infrastructure. Your team still owns routing policy inputs: provider keys, model groups, and caller allow lists.

Live activation is a **signed config revision**, not an in-place edit on the running pod. Customer administrators use **`metrum-genai-smartrouter-fleetctl customer`** commands from a **release binary package** on a trusted admin workstation. That CLI is not included in the standard customer Docker image.

Your operator supplies deployment-specific protected references (`profile-ref`, `license-ref`, `runtime-bundle-ref`) and the lifecycle signing workflow. Replace `<customer-id>`, paths, and hostnames with your deployment values.

**Greenfield:** `customer bootstrap` chains publish → manifest → sign → deploy → grant caller → sign → deploy → smoke. Use `--rewrite-paths fleet-eks` for EKS PVC paths. If the bundle exceeds Secrets Manager's 64 KiB limit, run `customer prepare-runtime-bundle --trim-catalog-only` first. See [Binary Installation](../installation/binary) for smoke flags.

**Updates:** `customer publish-runtime-bundle` or `customer update-config --patch-file`, then `customer create --sign-with-key` (or an externally signed `--intent`) to activate.

**Callers:** `customer grant-caller` publishes an updated bundle and unsigned manifest; sign and `customer create` to activate. Distribute the `--token-out` file once through your approved channel.

**Verify:**

```bash
metrum-genai-smartrouter-fleetctl customer status \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>"

metrum-genai-smartrouter-fleetctl customer smoke \
  --customer-id <customer-id> \
  --token-file /protected/CALLER_TOKEN.txt \
  --model <group-from-v1-models>
```

Smoke checks `/readyz`, caller-filtered `/v1/models`, and one OpenAI Chat completion that returns exact `OK` for the chosen group.

**Retire an instance:** contact your Metrum operator. Instance deletion requires a governed lifecycle delete approval; customer administrators do not receive `customer list` or `customer delete` in routine self-service flows.

## Operational Checklist After Every Change

- [ ] `/readyz` returns 200
- [ ] `/v1/models` lists expected groups for each affected caller token
- [ ] One representative completion succeeds for each client surface you support
- [ ] Ordinary caller tokens receive `403` on `/metrics` unless explicitly granted metrics-admin access
- [ ] Usage or admin reports show expected caller/project labels (when reporting is enabled)

## Where To Go Next

- [Enterprise Deployment Patterns](./deployment-patterns) — topology and operator contracts
- [License-Protected Deployments](./license-protected-deployments) — license replacement and feature gates
- [Request Troubleshooting](../troubleshooting/requests) — 401/403/502 diagnostics with request IDs
