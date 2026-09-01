# Production Runbook

Internal note: Metrum engineering production runs on Fleet EKS. See
**`docs/EKS_PRODUCTION_OPERATIONS.md`** for the current Metrum operator path.

This runbook retains **customer Docker Compose** procedures only.

## Customer Docker Compose deployment

Smart LLM Router can run on-prem or in an enterprise cloud account with different
hostnames, model groups, providers, and caller policies. Package-safe bootstrap
guidance lives in `docs/PACKAGE_README.md`, `docs/BINARY_INSTALL.md`,
`docs/DOCKER_COMPOSE_INSTALL.md`, and the router-served Docusaurus installation docs.

### Layout

```text
/opt/smart-llmrouter/compose/
  docker-compose.yml
  docker-compose.postgres-localhost.yml   # optional Postgres usage DB
  config/config.yaml
  config/env.json
  state/
  ROUTER_TOKEN*.txt
```

### Config-only change

1. Update `config.example.yaml` when the change affects reference config.
2. Validate YAML with structured parsing.
3. Run `rtk go test ./cmd/... ./internal/...`.
4. Back up live config as `config/config.yaml.bak.<purpose>-<UTC timestamp>`.
5. Apply with structured YAML tooling; restart router; verify `/readyz`.

### Package deployment

1. `rtk go test ./cmd/... ./internal/...`
2. `rtk make package-docker`
3. Use `scripts/compose_package_upgrade.py plan|apply|rollback` against the
   customer install root (see `docs/DOCKER_DEPLOYMENT.md`).

### Verification

```bash
curl -fsS https://<customer-hostname>/readyz
curl -fsS https://<customer-hostname>/version
```

Run Codex CLI and Claude Code CLI smokes when routing or API compatibility changed.

## Metrum Fleet production

See `docs/EKS_PRODUCTION_OPERATIONS.md`.
