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
