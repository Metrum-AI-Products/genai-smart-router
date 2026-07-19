# Shipped: Admin browser reports

**Personas:** P11 Deployment admin (HTTP Basic + Casbin). Ordinary callers get 403 on `/metrics` and admin APIs.

**Entry:** `https://router.example.invalid/admin/` (fictional)

**HTML:** [admin-reports-shell.html](admin-reports-shell.html)

## How accessed

1. Admin opens `/admin/` with Basic credentials that map to Casbin roles with report permissions.
2. Global filters: time window (`since`), project, environment, limit.
3. Nav groups: overview, usage, savings, performance, traffic-shaping, routing-decisions, provider-catalog, security, request-drilldown, system-status.
4. Each tab loads SQL-backed aggregates — never dump raw prompts/tools/images.

## Tab catalog

| Tab ID | Label | Persona | Decision it supports | Filters |
| --- | --- | --- | --- | --- |
| `overview` | Overview | P11 | Health + cost + latency trends | since, project |
| `groups` | Groups | P11 | Usage by model group | status, cache |
| `providers` | Providers | P11 | Usage by provider | status, cache |
| `tokens` | Keys | P11 | Usage by API key | status, cache |
| `savings` | Savings | P11 | Savings vs baseline | baseline |
| `savings-by-user` | Savings by user | P11 | Chargeback by user | baseline |
| `savings-by-key` | Savings by key | P11 | Chargeback by key | baseline |
| `savings-by-group` | Savings by group | P11 | Group savings | baseline |
| `savings-by-project` | Savings by project | P11 | Project/env savings | baseline |
| `savings-by-provider-model` | Savings by provider | P11 | Provider/model savings | baseline |
| `model-groups-by-user` | User groups | P11 | Groups used per user | status |
| `usage-by-key` | Key usage | P11 | Key traffic | status |
| `usage-by-caller` | Caller usage | P11 | Caller traffic | status |
| `requested-models` | Requested models | P11 | Requested group distribution | status |
| `provider-model-mix` | Provider/model | P11 | Selected upstream mix | status |
| `latency-throughput` | Latency | P11 | Latency + tok/s | sort |
| `errors-fallbacks` | Errors | P11 | Errors and fallbacks | sort |
| `upstream-failures` | Upstream failures | P11 | Upstream failure classes | sort |
| `request-shape-failures` | Shape failures | P11 | Shape filter denials | sort |
| `fallback-health` | Fallback health | P11 | Fallback outcomes | sort |
| `user-client-impact` | User impact | P11 | Client-visible impact | sort |
| `cache-report` | Cache | P11 | Hit/miss/bypass | cache |
| `quotas-budgets` | Quotas | P11 | Quota/budget consumption | status |
| `traffic-shaping-overview` | Shaping | P11 | Traffic shape overview | shape filters |
| `traffic-shaping-by-user` | Shaping users | P11 | Shape by user | shape filters |
| `traffic-shaping-by-key` | Shaping keys | P11 | Shape by key | shape filters |
| `traffic-shaping-by-client` | Shaping clients | P11 | Shape by client | shape filters |
| `traffic-shaping-by-group` | Shaping groups | P11 | Shape by group | shape filters |
| `provider-capacity-shaping` | Provider shaping | P11 | Provider capacity shaping | shape filters |
| `adaptive-upstream-backoff` | Backoff | P11 | Adaptive upstream backoff | shape filters |
| `traffic-tuning-advisor` | Tuning advisor | P11 | Tuning recommendations | shape+status |
| `troubleshooting-buckets` | Troubleshooting | P11 | Bucketed triage | sort |
| `routing-decisions` | Routing | P11 | Routing decision aggregates | sort |
| `dynamic-signals` | Dynamic signals | P11 | Signal telemetry | sort |
| `dynamic-score-buckets` | Dynamic scores | P11 | Score buckets | sort |
| `dynamic-thresholds` | Dynamic thresholds | P11 | Threshold hits | sort |
| `max-token-buckets` | Max tokens | P11 | Output cap buckets | sort |
| `input-token-buckets` | Input tokens | P11 | Input size buckets | sort |
| `admission-reasons` | Admission | P11 | Admission allow/deny reasons | sort |
| `provider-catalog-status` | Catalog status | P11 | Catalog readiness | sort |
| `retention-status` | Retention | P11 | Retention job status | sort |
| `contract-buckets` | Contracts | P11 | Contract evaluations | sort |
| `contract-workloads` | Workloads | P11 | Workload contracts | sort |
| `target-validation` | Validation | P11 | Target validation evidence | sort |
| `expensive-requests` | Expensive | P11 | Top expensive requests | sort |
| `client-breakdown` | Clients | P11 | Client breakdown | status |
| `project-chargeback` | Projects | P11 | Project chargeback | status |
| `capability-usage` | Capabilities | P11 | Tools/VLM/structured/stream | status |
| `anomalies` | Anomalies | P11 | Anomaly/abuse signals | status |
| `security-events` | Security | P11 | Authz/security events | sort |
| `requests` | Requests | P11 | Request drilldown search | status+sort |

## States

- **Empty:** no rows in window — show empty table + widen filter CTA
- **Error:** safe query failure message + request correlation; no SQL with values
- **Unauthorized:** 403; metrics-admin isolation preserved

## Validation

Playwright e2e in `internal/router/admindist/web/e2e/admin-reports.spec.ts` asserts every tab renders.
