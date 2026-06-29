# Production Runbook

Internal note: this runbook is private operational material and is intentionally not included in binary or Docker release packages. Keep package-safe deployment bootstrap guidance in `docs/PACKAGE_README.md`, `docs/BINARY_INSTALL.md`, `docs/DOCKER_COMPOSE_INSTALL.md`, and the router-served Docusaurus installation docs.

This runbook is for the current Metrum-managed engineering deployment. Smart LLM Router can also run on-prem or in an enterprise cloud account with different hostnames, model groups, providers, and caller policies.

Commercial portal, Stripe, quote, invoice, refund, dispute, and license-fulfillment operations are not performed from this production router host. Use the approved commercial/support systems and `docs/LICENSE_OPERATIONS.md`; never copy Stripe secrets, signing-service private credentials, customer license payloads, or commercial back-office exports into the production compose tree.

## Current Managed Deployment

```text
Host: ubuntu@100.30.225.66
Compose directory: /opt/smart-llmrouter/compose
Runtime config: /opt/smart-llmrouter/compose/config/config.yaml
Runtime env: /opt/smart-llmrouter/compose/config/env.json
Public URL: https://llm-api-engg.metrum.ai
```

Do not print router tokens, provider keys, token hashes, or full production config contents.

## Config-Only Change

1. Update `config.example.yaml` when the change affects reference config.
2. Update ignored local `config.production.yaml`.
3. Validate YAML with structured parsing.
4. Run relevant tests, normally `rtk go test ./cmd/... ./internal/...`.
5. Back up the live config as `config/config.yaml.bak.<purpose>-<UTC timestamp>`.
6. Apply the config change with structured YAML tooling.
7. Run `sudo docker compose config >/dev/null`, restart router, and verify `sudo docker compose ps router`.
8. Verify `/readyz`, a relevant authenticated request, and local/remote config SHA-256 match.
9. Update `deployment.md`.

## Package Deployment

1. Run:

```bash
rtk go test ./cmd/... ./internal/...
rtk make docs-build
rtk make package-docker
```

2. Commit source/docs changes before packaging so the binary version is not dirty.
3. Copy the package matching the production host CPU architecture to the host. The current Metrum-managed production host uses the `linux-amd64` package.
4. Back up `/opt/smart-llmrouter` as `/opt/smart-llmrouter.backup.<purpose>-<UTC timestamp>`.
5. Unpack the new package into a fresh directory.
6. Copy forward live `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt`.
7. If applying a new production config, back up the copied config before replacing it.
8. Set `SMART_LLMROUTER_VERSION=<version>-linux-amd64` in `compose/.env` for the current x86_64 production host, or the matching package architecture for other deployments.
9. Ensure runtime ownership:

```bash
sudo chown -R 65532:65532 compose/config compose/state compose/logs
sudo find compose/config compose/state compose/logs -type d -exec chmod 0750 {} +
sudo find compose/config -type f -exec chmod 0640 {} +
```

10. Run `sudo docker compose config >/dev/null`, load the image, and start the stack.

## Required Verification

Run at minimum:

```bash
curl -fsS https://llm-api-engg.metrum.ai/readyz
curl -fsS https://llm-api-engg.metrum.ai/version
curl -fsS https://llm-api-engg.metrum.ai/docs/overview
```

With a router token, verify:

- `/v1/models` returns the model groups allowed for that exact caller token;
- a text request succeeds;
- an omitted-model request behaves according to `server.default_model_group`;
- changed model/provider/tool/VLM behavior passes a targeted smoke.

Run real CLI smokes for production-affecting routing or API changes:

- Claude Code with `claude -p` through `ANTHROPIC_BASE_URL`;
- Codex CLI through `/v1/responses`;
- opencode and aider when OpenAI-compatible coding routes or customer client compatibility changed;
- image smoke when modality metadata changes;
- OpenAI Chat tools smoke for Warp-style clients when tool routing changes.

For coding-agent route changes, run the deterministic matrix before live smokes:

```bash
rtk python3 scripts/coding_agent_matrix.py --mode mock --output-dir tmp/coding-agent-matrix
```

Then follow `docs/CODING_AGENT_E2E_MATRIX.md` for live Codex, Claude Code, opencode, and aider coverage. Record request IDs and verify usage rows instead of relying only on client stdout.

For VLM route changes, run direct upstream and router-level image smokes with a realistic budget, normally at least `512` output tokens, plus a tiny-cap enforcement smoke and a negative private-image-URL smoke. If an image target fails OCR or workload quality, explicit cap behavior, or URL-safety validation, restore the previous config backup or remove the target from the affected group, restart the router, and rerun `/readyz`, `/v1/models`, text, and image smokes before reopening traffic.

## Cleanup

After deployment:

- remove uploaded packages and temporary config files from `/home/ubuntu`;
- remove replaced `/opt/smart-llmrouter.replaced.*` trees after validation;
- remove stale `/tmp/smart-llmrouter-*tar*` files;
- run `sudo docker system prune -f` when safe;
- do not prune Docker volumes unless intentionally resetting state.

Record cleanup results in `deployment.md`.

## Rollback

Prefer config rollback for bad model/provider weights. Restore the previous `config.yaml` backup, restart router, then verify `/readyz` and a representative request.

For package rollback, stop compose, move the current `/opt/smart-llmrouter` aside, restore the timestamped backup tree, start compose, and verify health, version, docs, and affected API behavior.
