# Package Validation Bootstrap

Release packages are intentionally small and package-safe. The full administrator documentation is embedded in the router binary and served at `/docs/` after startup.

## Validate Package Layout

Expected binary package layout:

```text
smart-llmrouter-<version>-linux-<arch>/
  bin/
  config/
  caddy/
  docs/
```

Expected Docker Compose package layout:

```text
smart-llmrouter-<version>-docker-linux-<arch>/
  compose/
  config/
  images/
  docs/
```

Package docs are copied only from `scripts/package_docs_allowlist.txt` during release creation. In a delivered package, the offline docs should be this bootstrap set plus the package-safe solution brief.

For a Docker package, load the saved image and version-check all operational binaries, including the non-serving migration runner:

```bash
docker run --rm --entrypoint /app/bin/router smart-llmrouter:<version>-linux-<arch> --version
docker run --rm --entrypoint /app/bin/router-token-gen smart-llmrouter:<version>-linux-<arch> --version
docker run --rm --entrypoint /app/bin/router-usage-report smart-llmrouter:<version>-linux-<arch> --version
docker run --rm --entrypoint /app/bin/router-migrate smart-llmrouter:<version>-linux-<arch> --version
```

Before a `deployment-job` serving startup, use `router-migrate` to run `plan`, take the approved backup, then `apply`, complete every release-defined data job, `verify`, and `status`. The current package completes `historical-usage-validation-v1` at checkpoint ordinal `0` before verification; larger future jobs continue one ordinal at a time until `validated`. PostgreSQL uses `--dsn-env=ROUTER_USAGE_DB_DSN`, never a literal DSN. The detailed procedure is `DATA_MIGRATIONS.md`.

Packages must not contain:

- private production runbooks;
- private hostnames, IP addresses, SSH usernames, or key paths;
- raw router tokens, token hashes, provider keys, GitHub tokens, or signing material;
- real `license.json`, license state, local usage databases, logs, or JSONL state;
- source checkout directories such as `docs-site/`, `internal/`, `cmd/`, or `.git`;
- full production config files.

## Runtime Health Checks

After starting the router:

```bash
export ROUTER_BASE_URL="https://router.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/version"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

`/readyz` confirms required runtime checks such as license enforcement and configured dependencies. `/docs/` confirms the embedded Docusaurus admin docs are reachable. `/v1/models` confirms the caller token is valid and shows the deployment-defined model groups available to that caller.

For package questions or support escalation, contact `contact@metrum.ai`.
