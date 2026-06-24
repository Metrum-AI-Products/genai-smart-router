# Admin Authentication

This runbook covers the first browser-admin identity option: HTTP Basic authentication for `/admin/*` routes. Basic Auth establishes identity only. It does not grant permissions by itself, and it does not replace router caller tokens for `/v1/*` model APIs.

## Current Scope

- Disabled by default.
- Applies to browser/admin routes such as `GET /admin/auth/check`.
- Uses bcrypt password hashes from environment variables or deployment secrets.
- Requires HTTPS unless `allow_insecure_http: true` is set for local development.
- Honors `X-Forwarded-Proto: https` only when the request comes from a configured trusted proxy CIDR.
- Produces a stable subject such as `basic:admin`.
- Uses route permissions only for the current stub check route. Broader admin authorization should move to Casbin under issue #71.

`/metrics` remains protected by caller tokens with `metrics_admin: true`. Basic Auth does not grant metrics access.

## Configure

Generate a bcrypt hash on an admin workstation or deployment host:

```bash
python3 - <<'PY'
import bcrypt
password = b"replace-with-a-strong-password"
print(bcrypt.hashpw(password, bcrypt.gensalt(rounds=12)).decode())
PY
```

Store the hash in a deployment secret or process environment, for example:

```bash
export SMART_ROUTER_ADMIN_PASSWORD_HASH='$2b$12$exampleplaceholderexampleplaceholderexampleplaceholder'
```

Configure Basic Auth:

```yaml
server:
  admin_auth:
    basic:
      enabled: true
      realm: GenAI Smart Router Admin
      allow_insecure_http: false
      trusted_proxy_cidrs:
        - 127.0.0.1/32
      users:
        - username: admin
          password_hash_env: SMART_ROUTER_ADMIN_PASSWORD_HASH
          subject: basic:admin
          domain: example/prod
          permissions:
            - admin:auth:read
```

Use `allow_insecure_http: true` only for local loopback testing. Production deployments should terminate TLS at the reverse proxy and pass `X-Forwarded-Proto: https` to the router. Set `trusted_proxy_cidrs` to the reverse proxy network only; do not trust forwarded headers from arbitrary clients.

## Smoke Test

Missing credentials should challenge:

```bash
curl -i "$ROUTER_BASE_URL/admin/auth/check"
```

Expected: `401` with `WWW-Authenticate: Basic ...`.

Valid credentials with the test permission should succeed:

```bash
curl -i -u admin:replace-with-the-admin-password "$ROUTER_BASE_URL/admin/auth/check"
```

Expected: `200` with safe subject metadata such as `basic:admin` and no credential material.

Valid credentials without `admin:auth:read` should return `403 admin-forbidden`.

Existing API callers should be unchanged:

```bash
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" "$ROUTER_BASE_URL/v1/models"
```

## Rollback

Set `server.admin_auth.basic.enabled: false`, restart the router, and verify:

- `/admin/auth/check` returns `404`;
- `/v1/models` still works with a normal router caller token;
- `/metrics` still requires a metrics-admin caller token.

## Security Notes

- Do not store plaintext admin passwords in config, docs, logs, shell history, or tickets.
- Do not commit password hashes for real deployments. Keep them in environment variables or a deployment secret manager.
- Failed login responses intentionally do not reveal whether the username or password was wrong.
- Protected admin responses use `Cache-Control: no-store`.
- Basic Auth is appropriate as a simple first option. OIDC and browser sessions are tracked separately in issue #73.
