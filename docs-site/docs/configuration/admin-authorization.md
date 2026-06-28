---
title: Admin Authorization
---

# Admin Authorization

GenAI Smart Router uses Casbin for deployment-owned authorization decisions on operator surfaces. Authentication still happens first:

- router bearer tokens authenticate API and machine callers;
- HTTP Basic can authenticate simple browser-admin users;
- OIDC/session authentication can authenticate browser-admin users through an identity provider.

Casbin receives a stable authenticated subject plus a domain, object, and action. It does not verify passwords, bearer tokens, or OIDC tokens.

## Policy Model

The authorization model is domain-aware RBAC:

```text
sub = authenticated subject, such as caller:ops-key, basic:reports-admin, or user:alice@example.com
dom = deployment, project, or environment domain, such as example/prod
obj = resource class, such as metrics, admin:reports, or content:capture
act = action, such as read, export, drilldown, delete, or purge
```

## Configuration

Policy can be loaded from a deployment-owned CSV file:

```yaml
server:
  admin_auth:
    authorization:
      enabled: true
      source: static
      policy_file: config/authz/policy.csv
      policy: []
```

Inline policy is also supported for small deployments:

```yaml
server:
  admin_auth:
    authorization:
      enabled: true
      source: static
      policy:
        - g, caller:ops-metrics-key, metrics_admin, example/prod
        - g, caller:content-admin-key, content_admin, example/prod
        - g, basic:reports-admin, reports_admin, example/prod
        - g, user:alice@example.com, reports_admin, example/prod
        - p, metrics_admin, example/prod, metrics, read
        - p, content_admin, example/prod, content:capture, delete|purge
        - p, reports_admin, example/prod, admin:reports, read|export|drilldown
        - p, reports_admin, example/prod, admin:security_reports, read|export
```

Use placeholder-like subjects in examples and keep secrets out of policy. Policy must not include raw router tokens, token hashes, provider keys, passwords, password hashes, prompts, images, tool outputs, or full deployment config.

Admin report data is scoped to the subject's policy domain by default. A reports admin in `example/prod` sees caller project `example` and environment `prod`; cross-domain request IDs return `404`. Use an explicit `*` policy domain only for approved deployment-wide report administrators:

```text
p, user:global-reports@example.com, *, admin:reports, read|export|drilldown
p, user:global-reports@example.com, *, admin:security_reports, read|export
```

Managed deployments can load authorization from a DB-backed policy lifecycle store:

```yaml
server:
  usage_db:
    driver: sqlite
    path: usage.sqlite
  admin_auth:
    authorization:
      enabled: true
      source: db
      policy_file: ''
      policy: []
```

DB mode loads the single active policy set from the usage DB. Startup fails closed when no active policy exists, multiple active policies exist, or the active policy does not validate. Static file and inline policy remain supported and are not mixed with DB mode.

## Policy Lifecycle

The DB-backed lifecycle stores versioned policy sets, scalar policy rules, scalar role links, and safe audit events in relational tables. Activation validates the proposed policy before it becomes active. If validation fails, the router keeps the current active policy as the last known valid set.

Rollback reactivates the most recently retired valid policy set and writes an audit event. Audit fields are limited to safe scalar values such as policy set id, actor subject, action, timestamp, request id, and a short summary. Do not put raw tokens, token hashes, provider keys, passwords, prompts, images, tool outputs, or full config fragments in policy or audit summaries.

## Metrics

`/metrics` checks object `metrics` with action `read`.

Existing caller keys with `metrics_admin: true` remain compatible. The router synthesizes an equivalent Casbin grant at startup, so current metrics-admin callers still work while ordinary caller tokens receive `403 metrics-forbidden`.

Deployments can also grant metrics access with explicit policy for a caller subject:

```text
g, caller:ops-metrics-key, metrics_admin, example/prod
p, metrics_admin, example/prod, metrics, read
```

## Content-Capture Maintenance

Content-capture maintenance endpoints check object `content:capture`.

`DELETE /v1/content-captures/<request_id>` checks action `delete`. `POST /v1/content-captures/purge-expired` checks action `purge`. Existing caller keys with `content_admin: true` remain compatible through synthesized Casbin grants:

```text
g, caller:content-admin-key, content_admin, example/prod
p, content_admin, example/prod, content:capture, delete|purge
```

Operators may instead grant the same resource/actions through explicit policy. Metrics and report grants do not grant content-capture maintenance access.

## Admin Reports

`/admin/reports/*` checks object `admin:reports`.

Pages, aggregate JSON APIs, static report assets, request drilldown, and Markdown export all enforce server-side authorization. Request detail uses the separate `drilldown` action. A reports administrator needs policy similar to:

```text
g, basic:reports-admin, reports_admin, example/prod
g, user:alice@example.com, reports_admin, example/prod
p, reports_admin, example/prod, admin:reports, read|export|drilldown
```

OIDC sessions with `subject_claim: email` produce subjects like `user:alice@example.com` in the configured `server.admin_auth.oidc.domain`. Non-email subject claims produce `oidc:<claim value>`. Groups and email domains are identity attributes; grant roles through policy instead of hardcoding permissions in handlers.

Ordinary router caller tokens are not browser-admin credentials and receive `403 reports-forbidden` on report endpoints.

## Rollback

Disable `server.admin_auth.authorization.enabled` and restart the router. Existing metrics-admin caller behavior is preserved, and admin reports cannot be enabled until authorization is configured again.

For DB-backed policy changes, use lifecycle rollback to return to the previous retired valid policy set. If DB policy loading is unavailable, switch back to `source: static` with a known-good deployment-owned policy file and restart.
