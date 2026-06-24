---
title: Admin Authentication
---

# Admin Authentication

GenAI Smart Router separates caller API authentication from browser-admin authentication.

- Router caller tokens authenticate applications and CLI users for `/v1/*` model APIs.
- Metrics-admin caller tokens read `/metrics`.
- HTTP Basic can establish a simple browser-admin identity for `/admin/*` routes.
- Casbin authorization is the intended policy decision layer for broader admin permissions.

HTTP Basic is disabled by default and is intended for simple self-hosted deployments, bootstrap access, and early admin-reporting surfaces.

## Basic Auth Configuration

Use a bcrypt password hash stored in deployment secrets or environment variables:

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

`password_hash_env` names an environment variable that contains the bcrypt hash. Do not put plaintext passwords in router config.

`trusted_proxy_cidrs` controls whether the router honors `X-Forwarded-Proto: https` from a reverse proxy. Leave it empty unless the router is behind a proxy you control, and set it only to that proxy's network or host address.

For local loopback testing only, set `allow_insecure_http: true`:

```yaml
server:
  admin_auth:
    basic:
      enabled: true
      realm: Local Router Admin
      allow_insecure_http: true
      trusted_proxy_cidrs: []
      users:
        - username: admin
          password_hash_env: SMART_ROUTER_ADMIN_PASSWORD_HASH
          subject: basic:admin
          domain: local/dev
          permissions:
            - admin:auth:read
```

Generate a hash with an operating-system package or a trusted admin tool. One Python example:

```bash
python3 - <<'PY'
import bcrypt
password = b"replace-with-a-strong-password"
print(bcrypt.hashpw(password, bcrypt.gensalt(rounds=12)).decode())
PY
```

Install the hash as a deployment secret or environment variable before the router starts:

```bash
export SMART_ROUTER_ADMIN_PASSWORD_HASH='$2b$12$replaceWithTheGeneratedHash'
```

## Runtime Behavior

`GET /admin/auth/check` is a protected stub endpoint for validating the first admin identity path.

| Condition | Result |
| --- | --- |
| Basic Auth disabled | `404` |
| Missing or invalid credentials | `401` with `WWW-Authenticate` |
| Valid credentials without route permission | `403 admin-forbidden` |
| Valid credentials with `admin:auth:read` | `200` with safe subject metadata |

Responses for protected admin routes use no-store cache headers. Passwords and password hashes are not returned.

## Smoke Tests

Missing credentials should challenge:

```bash
curl -i "$ROUTER_BASE_URL/admin/auth/check"
```

Example response:

```http
HTTP/1.1 401 Unauthorized
Cache-Control: no-store
WWW-Authenticate: Basic realm="GenAI Smart Router Admin", charset="UTF-8"
```

Invalid credentials return the same status without revealing which field was wrong:

```bash
curl -i -u admin:wrong-password "$ROUTER_BASE_URL/admin/auth/check"
```

Valid credentials with the current stub permission return safe subject metadata:

```bash
curl -i -u admin:replace-with-the-admin-password "$ROUTER_BASE_URL/admin/auth/check"
```

Example response:

```json
{
  "ok": true,
  "source": "basic",
  "subject": "basic:admin",
  "domain": "example/prod"
}
```

If the password is valid but `admin:auth:read` is not granted, the endpoint returns:

```json
{
  "error": {
    "type": "admin-forbidden",
    "message": "admin-forbidden"
  }
}
```

Normal model APIs continue to use router caller tokens:

```bash
curl "$ROUTER_BASE_URL/v1/models" \
  -H "Authorization: Bearer $ROUTER_TOKEN"
```

Metrics continue to use metrics-admin caller tokens:

```bash
curl "$ROUTER_BASE_URL/metrics" \
  -H "Authorization: Bearer $METRICS_ADMIN_ROUTER_TOKEN"
```

## TLS Requirement

Production deployments should set `allow_insecure_http: false`. The router accepts Basic credentials only when the incoming request is HTTPS or when a configured trusted reverse proxy forwards `X-Forwarded-Proto: https`.

Only trusted proxy CIDRs are allowed to assert forwarded HTTPS:

```yaml
server:
  admin_auth:
    basic:
      allow_insecure_http: false
      trusted_proxy_cidrs:
        - 127.0.0.1/32
        - 172.18.0.0/16
```

Use the actual reverse-proxy subnet for the deployment. Do not add broad networks unless the router is isolated from direct client traffic on those networks.

Use `allow_insecure_http: true` only for local development and loopback smoke tests.

## Authorization Boundary

Basic Auth establishes a subject such as `basic:admin`; it does not decide what that subject may do. Current stub-route permissions are intentionally small. Broader admin/report authorization should be expressed through the deployment's Casbin policy as those surfaces are enabled.

Example Casbin-style policy shape for later admin reports:

```text
g, basic:admin, reports_admin, example/prod
p, reports_admin, example/prod, admin:reports, read
p, reports_admin, example/prod, admin:reports, export
```

The username is not the authorization rule. The stable subject, domain, object, and action should be the policy inputs.

OIDC and server-side browser sessions are a separate follow-up for deployments that need enterprise identity-provider integration.

## Rollback

Disable Basic Auth and restart the router:

```yaml
server:
  admin_auth:
    basic:
      enabled: false
      users: []
```

After rollback, `/admin/auth/check` should return `404`, while `/v1/*` model APIs and `/metrics` keep their existing token behavior.
