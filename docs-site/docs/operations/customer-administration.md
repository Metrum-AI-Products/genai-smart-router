---
title: Customer Administrator Guide
doc_type: howto
---

# Customer Administrator Guide

This guide is for **customer platform administrators**: the people who own router configuration, provider credentials, model groups, caller access, quotas, and routine operational changes for one deployment.

**Metrum-hosted production** is **CLI and docs only**. You do not edit
Kubernetes objects. Do not look for a ConfigMap or a Secret, and do not use
`kubectl`. Metrum applies configuration through the customer CLI; use the base
URL assigned to your deployment.

| Deployment shape | Where config lives | How you change it |
| --- | --- | --- |
| **Private managed** (Metrum-hosted) | Platform runtime bundle | `metrum-genai-smartrouter-fleetctl customer` from a **release binary package** |
| **Self-hosted Compose / binary** | Files on the host (`config.yaml`, `env.json`, `license.json`, mode `0600`) | Edit files, then restart |
| **Self-hosted Kubernetes** (you operate the cluster) | **One Kubernetes Secret** with `config.yaml`, `env.json`, and `license.json` mounted under `/app/config/` | Update that Secret, then roll the Deployment (`Recreate` for SQLite PVC). Never put production runtime files in a ConfigMap. See [Deploy To Kubernetes](../installation/kubernetes). |

Model group names, hostnames, and allow lists are **deployment-defined**. Call `/v1/models` with each caller token to confirm what that token may request.

For token format and identity rules, see [User Key Generation](./key-generation). For caller config fields, see [Caller Tokens](../configuration/caller-tokens). For self-hosted install mechanics, see [Installation](../installation/).

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

## How To Update Config (Private Managed)

Do not edit ConfigMaps or Secrets. Metrum applies configuration through the customer CLI. Every mutating command publishes a new runtime bundle and an unsigned workspace manifest. Activation is a **signed** `customer create`. Rollback is the previous signed revision, not `kubectl rollout undo`.

Your operator supplies `--profile-ref`, `--license-ref`, and `--runtime-bundle-ref`. Replace `<customer-id>` and paths with your deployment values.

### Download live config

```bash
metrum-genai-smartrouter-fleetctl customer get-config \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>" \
  --runtime-bundle-ref "<runtime-bundle-ref>" \
  --license-ref "<license-ref>" \
  --config-out /protected/config.yaml \
  --env-out /protected/env.json
```

Stdout is safe JSON (`config_sha256`, `env_sha256`, `model_group_count`, paths). The files are mode `0600`. Pass `--force` only when you intend to overwrite local copies.

Edit locally, then validate. To activate a full replacement, use `customer publish-runtime-bundle` with the edited files, then signed `customer create`. For a YAML mapping merge, use `customer update-config --patch-file`.

### List caller keys

```bash
metrum-genai-smartrouter-fleetctl customer list-callers \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>" \
  --runtime-bundle-ref "<runtime-bundle-ref>" \
  --license-ref "<license-ref>"
```

Stdout is a JSON array of caller records with user, project, membership, allow list, and configured rate/quota fields. The CLI does not group or chart this data; keep the JSON if you want later analysis. Hashes and raw tokens are omitted.

### Add a caller key

`customer grant-caller` generates a token, writes `--token-out` (mode `0600`, refuse overwrite), publishes a bundle that adds the hashed caller row, and writes an unsigned manifest. Distribute the token file once through your approved channel. Sign and `customer create` to activate.

### Revoke or disable a key

```bash
metrum-genai-smartrouter-fleetctl customer revoke-caller \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>" \
  --runtime-bundle-ref "<runtime-bundle-ref>" \
  --license-ref "<license-ref>" \
  --caller-id <caller-id> \
  --status disabled
```

`--status` may be `disabled` (default), `rotated`, `expired`, or `suspended`. The caller row stays in config so reports keep `token_id`. Sign and `customer create` to activate.

### Check live quota remaining

Quotas are per **caller key**, not per `users[]` record. To inspect one user, pass `--owner-user` (repeatable) so every caller owned by that user is listed.

```bash
metrum-genai-smartrouter-fleetctl customer quota-status \
  --customer-id <customer-id> \
  --admin-basic-file /protected/basic-admin \
  --owner-user <user-id>
```

`--admin-basic-file` is a mode-`0600` `username:password` file for browser-admin / reports-admin. Ordinary caller tokens cannot read another key's remaining budget. The same HTTP route is `GET /admin/reports/api/quota-status`.

On self-hosted installs, a single caller can still use `GET /v1/usage` with that caller's token.

### Update quota limits

This changes configured `rate` / `quota` / lifetime budgets. It does **not** reset used day/month/lifetime counters.

```bash
metrum-genai-smartrouter-fleetctl customer update-quota \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>" \
  --runtime-bundle-ref "<runtime-bundle-ref>" \
  --license-ref "<license-ref>" \
  --owner-user <user-id> \
  --day-tokens 40000000 \
  --tpm 400000
```

You may pass `--caller-id` and/or `--owner-user` (repeatable). Sign and `customer create` to activate.

### Activate, verify, rollback

```bash
metrum-genai-smartrouter-fleetctl customer create \
  --customer-id <customer-id> \
  --sign-with-key /protected/lifecycle_approval_private_key.b64

metrum-genai-smartrouter-fleetctl customer status \
  --customer-id <customer-id> \
  --profile-ref "<profile-ref>"

metrum-genai-smartrouter-fleetctl customer smoke \
  --customer-id <customer-id> \
  --token-file /protected/CALLER_TOKEN.txt \
  --model <group-from-v1-models>
```

Smoke checks `/readyz`, caller-filtered `/v1/models`, and one OpenAI Chat completion that returns exact `OK` for the chosen group.

**Greenfield:** `customer bootstrap` chains publish → manifest → sign → deploy → grant caller → sign → deploy → smoke. Your operator supplies the rewrite and bundle-size steps when they apply.

**Retire an instance:** contact your Metrum operator. Instance deletion is a governed lifecycle action, not a routine self-service flow.

## Self-Hosted: First Start And Restart

### Docker Compose or binary

1. Place reviewed `config.yaml`, `env.json`, and `license.json` in the protected paths defined by your install layout.
2. Start or restart the router service (for example `docker compose restart router` or your systemd unit).
3. Confirm health:

```bash
curl -fsS https://<your-router-host>/readyz
```

### Customer-managed Kubernetes

Follow [Deploy To Kubernetes](../installation/kubernetes). Production runtime files belong in **one Secret** (`config.yaml`, `env.json`, `license.json`). Use **`Recreate`** when the router uses a single `ReadWriteOnce` SQLite PVC.

## Self-Hosted: Update Configuration And Restart

1. Edit `config.yaml` (and `env.json` when provider keys change) on the deployment host or through your config-management pipeline.
2. Validate syntax and deployment-specific checks.
3. Apply the change:
   - **Compose / binary:** restart the router process or container.
   - **Kubernetes:** update the runtime Secret and roll the Deployment.
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

On **private managed** hosts, prefer `customer grant-caller` so the hashed row is published into the live bundle. Local generate is a draft only until signed `create` activates it.

### Self-hosted activation

1. Append the generated caller row to `callers:` in `config.yaml` (or merge with your config controller).
2. Restart or roll the router.
3. Ask the caller to run `/v1/models` and confirm the allowed groups appear.

### Rotate or suspend a token

1. Generate a new token and caller row (or set `status: rotated` / `disabled` on the old caller).
2. Apply config and restart (or `revoke-caller` + signed `create` on managed).
3. Confirm the old token fails and the new token succeeds on `/v1/models`.

Never commit raw tokens, token hashes, or provider keys to git, tickets, or chat.

## Operational Checklist After Every Change

- [ ] `/readyz` returns 200
- [ ] `/v1/models` lists expected groups for each affected caller token
- [ ] One representative completion succeeds for each client surface you support
- [ ] Ordinary caller tokens receive `403` on `/metrics` unless explicitly granted metrics-admin access
- [ ] Usage or admin reports show expected caller/project labels (when reporting is enabled)
- [ ] For managed: `customer quota-status` matches the intended remaining vs configured limits after a quota change

## Where To Go Next

- [Enterprise Deployment Patterns](./deployment-patterns) — topology and operator contracts
- [License-Protected Deployments](./license-protected-deployments) — license replacement and feature gates
- [Request Troubleshooting](../troubleshooting/requests) — 401/403/502 diagnostics with request IDs
