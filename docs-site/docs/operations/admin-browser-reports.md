---
title: Admin Browser Reports
---

# Admin Browser Reports

Admin browser reports are an authenticated operational surface for usage, performance, cost, cache, fallback, and diagnostic drilldown. They are disabled by default and served separately from public `/docs/`.

## Access Model

The first browser identity option is HTTP Basic under `server.admin_auth.basic`. Authorization is Casbin-backed under `server.admin_auth.authorization`; every `/admin/reports/*` page, API, export, and drilldown route requires an allow decision for object `admin:reports`.

```yaml
server:
  admin_auth:
    basic:
      enabled: true
      users:
        - username: admin
          password_hash_env: SMART_ROUTER_ADMIN_PASSWORD_HASH
          subject: basic:admin
          domain: example/prod
    authorization:
      enabled: true
      policy:
        - g, basic:admin, reports_admin, example/prod
        - p, reports_admin, example/prod, admin:reports, read|export
  admin_reports:
    enabled: true
    path_prefix: /admin/reports
    default_since: 24h
    max_range: 31d
    max_rows: 500
    export_markdown: true
```

Ordinary router caller tokens receive `403 reports-forbidden`. Missing or invalid Basic credentials receive `401`.

## What It Shows

The browser UI displays requests, errors, tokens, cost, latency, TTFB, upstream/downstream throughput, cache hit/miss/bypass, attempts, fallbacks, provider/model groups, public token IDs, caller metadata, status codes, and recent safe request rows. Request drilldown joins the relational usage, attempt, trace-event, and terminal-error rows by request ID.

Responses do not include raw router tokens, token hashes, provider keys, raw prompts, raw images, raw tool outputs, full config values, or unsanitized upstream bodies.

## Embedded Assets

The admin HTML, CSS, JavaScript, and local chart bundle are embedded in the router binary. The UI does not depend on external CDNs. Charts are assistive; the same data is available in tables and Markdown export.

Admin pages and APIs send no-store cache headers. Static admin assets may use private cache headers and contain no report data.

## Smoke Test

```bash
curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Expected for an authorized subject: `200` JSON with `summary`, `series`, grouped tables, and recent request rows.

Expected for an ordinary router token:

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Response: `403 reports-forbidden`.
