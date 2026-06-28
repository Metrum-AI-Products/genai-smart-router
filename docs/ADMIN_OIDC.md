# Admin OIDC Sessions

This runbook covers browser-admin OIDC authentication for `/admin/*` routes. OIDC establishes a human admin subject and stores it in a server-side session. Casbin still authorizes every protected admin action.

OIDC is disabled by default and is independent from HTTP Basic. Basic can remain enabled for bootstrap or break-glass access, but it should be explicitly configured and audited.

## Configure

Register an OIDC client with the provider and set the callback to:

```text
https://router.example.com/admin/auth/callback
```

Store the client credentials in deployment secrets or environment variables:

```bash
export GOOGLE_OIDC_CLIENT_ID='replace-with-client-id'
export GOOGLE_OIDC_CLIENT_SECRET='replace-with-client-secret'
```

Configure OIDC and server-side sessions:

```yaml
server:
  admin_auth:
    oidc:
      enabled: true
      issuer_url: https://accounts.google.com
      client_id_env: GOOGLE_OIDC_CLIENT_ID
      client_secret_env: GOOGLE_OIDC_CLIENT_SECRET
      redirect_url: https://router.example.com/admin/auth/callback
      scopes:
        - email
        - profile
      allowed_domains:
        - example.com
      groups_claim: groups
      email_claim: email
      subject_claim: email
      domain: example/prod
    sessions:
      cookie_name: smart_router_admin_session
      ttl: 8h
      secure_cookies: true
      same_site: strict
    authorization:
      enabled: true
      policy:
        - g, user:alice@example.com, reports_admin, example/prod
        - p, reports_admin, example/prod, admin:reports, read|export|drilldown
```

`client_id_env` and `client_secret_env` name environment variables; do not put OIDC client secrets directly in YAML. The router validates issuer, audience, expiry, nonce, state, ID-token signature/JWKS, allowed email domain, and configured claim names before creating a session.

## Routes

- `GET /admin/auth/login` redirects to the IdP using Authorization Code flow with PKCE and nonce/state.
- `GET /admin/auth/callback` verifies the OIDC response and creates a server-side session cookie.
- `GET /admin/auth/me` returns safe subject metadata for the active Basic or OIDC admin identity.
- `POST /admin/auth/logout` deletes the server-side session and expires the browser cookie.

Pending OIDC login state is stored server-side with a short TTL, a global memory cap, and a per-client pending-login cap. Excess login attempts from one client receive `429 oidc-login-rate-limited` responses without blocking unrelated clients.

Successful OIDC login maps a verified email subject to `user:<email>` when `subject_claim: email`. Non-email subject claims map to `oidc:<claim value>`. Casbin policy should grant roles to those stable subjects; handlers must not hardcode email domains or groups as permissions.

## Google Workspace Notes

For Google Workspace, use issuer `https://accounts.google.com`, add the exact callback URL as an authorized redirect URI, and restrict `allowed_domains` to the Workspace domains that should administer the deployment. Google does not always include a `groups` claim in standard ID tokens; use explicit Casbin policy for initial access unless a governed provider-specific groups claim is configured and validated.

## Smoke Test

1. Start with `server.admin_auth.oidc.enabled: false` and confirm `/admin/auth/login` returns `404`.
2. Enable OIDC, sessions, and Casbin policy, then restart the router.
3. Open `/admin/auth/login` in a browser and complete IdP login.
4. Verify `/admin/auth/me` returns only safe subject metadata such as `source`, `subject`, `domain`, `email`, `issuer`, and optional group names.
5. Open `/admin/reports/` with a subject that has `admin:reports` policy and confirm it succeeds.
6. Test a valid login without report policy and expect `403 reports-forbidden`.
7. `POST /admin/auth/logout`, then confirm `/admin/auth/me` returns `401`.

`/docs/` remains public documentation, and `/v1/*` model APIs continue to use router caller tokens.

## Rollback

Set `server.admin_auth.oidc.enabled: false`, restart the router, and verify `/admin/auth/login`, `/admin/auth/callback`, `/admin/auth/logout`, and `/admin/auth/me` are no longer available for OIDC-only deployments. If Basic remains enabled, `/admin/auth/check` and Basic-protected admin routes continue to work according to their own config and Casbin policy.

## Security Notes

- Use HTTPS for production redirect URLs and `secure_cookies: true`.
- Use `same_site: strict` unless a deployment has a documented reason for `lax`.
- Sessions are stored server-side in memory and contain only safe identity metadata, not raw OIDC tokens.
- Monitor repeated `oidc-login-rate-limited` security events as potential login-state exhaustion attempts.
- Do not store raw OIDC tokens in browser localStorage, logs, docs, tickets, or policy files.
- Log only safe audit values such as result class, stable subject, issuer, request ID, and a session identifier hash or prefix.
