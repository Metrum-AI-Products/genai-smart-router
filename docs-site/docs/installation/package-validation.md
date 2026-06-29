---
title: Package Validation And Security Checks
---

# Package Validation And Security Checks

Release packages are designed to be small, inspectable, and safe for external administrators. The router-served `/docs/` site is the primary external reference; tarball Markdown is only an offline bootstrap set for starting the service.

## What To Verify

For binary packages:

```text
smart-llmrouter-<version>-linux-<arch>/
  bin/router
  bin/router-token-gen
  bin/router-usage-report
  config/config.example.yaml
  config/env.example.json
  config/scripts/router.ts
  caddy/Caddyfile
  docs/
```

For Docker Compose packages:

```text
smart-llmrouter-<version>-docker-linux-<arch>/
  compose/docker-compose.yml
  compose/docker-compose.postgres-localhost.yml
  compose/Caddyfile.compose
  compose/.env.example
  compose/.env
  config/config.example.yaml
  config/env.example.json
  config/scripts/router.ts
  images/smart-llmrouter-<version>-linux-<arch>.tar
  docs/
```

Confirm the architecture suffix matches the host and, for Docker packages, that `compose/.env` pins `SMART_LLMROUTER_VERSION` to the loaded image tag.

## What Must Not Be Present

Packages should not contain:

- private production runbooks or internal source-maintenance notes;
- private hostnames, IP addresses, SSH users, SSH key paths, or live production paths;
- raw provider keys, raw router tokens, token hashes, GitHub tokens, signing keys, or signing-service credentials;
- real `license.json`, license state files, local usage databases, logs, JSONL state, or router state files;
- source checkout directories such as `.git`, `cmd/`, `internal/`, `docs-site/`, or local build output;
- full production configs or ignored local config snapshots.

If any of those are present, stop the deployment and request a corrected package.

## Runtime Checks

After startup:

```bash
export ROUTER_BASE_URL="https://router.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/version"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Then run one small completion against a model group returned by `/v1/models`.

When admin reports are enabled, verify `/admin/reports/` only with a browser-admin subject authorized by the deployment. Ordinary caller tokens should not receive report access. Metrics scraping should use a caller subject authorized for metrics administration, not normal application keys.

## Evidence To Keep

Record the package name, router version, build timestamp, architecture, config checksum, image tag when applicable, `/readyz` result, `/docs/` result, `/v1/models` result for each caller class, and one request ID from a successful smoke.

Keep evidence free of raw tokens, provider keys, token hashes, prompt content, raw images, full config files, and private support-only paths.
