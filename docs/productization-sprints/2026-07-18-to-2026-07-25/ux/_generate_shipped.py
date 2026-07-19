#!/usr/bin/env python3
"""Generate shipped-surface narratives + HTML shells."""
from pathlib import Path

ROOT = Path(__file__).resolve().parent
SHIPPED = ROOT / "shipped"
SHIPPED.mkdir(exist_ok=True)
CSS = "../_ds/metrum-bento.css"
LOGO = "../_ds/metrum-logo-white.png"

TABS = [
    ("overview", "Overview", "P11", "Health + cost + latency trends", "since, project"),
    ("groups", "Groups", "P11", "Usage by model group", "status, cache"),
    ("providers", "Providers", "P11", "Usage by provider", "status, cache"),
    ("tokens", "Keys", "P11", "Usage by API key", "status, cache"),
    ("savings", "Savings", "P11", "Savings vs baseline", "baseline"),
    ("savings-by-user", "Savings by user", "P11", "Chargeback by user", "baseline"),
    ("savings-by-key", "Savings by key", "P11", "Chargeback by key", "baseline"),
    ("savings-by-group", "Savings by group", "P11", "Group savings", "baseline"),
    ("savings-by-project", "Savings by project", "P11", "Project/env savings", "baseline"),
    ("savings-by-provider-model", "Savings by provider", "P11", "Provider/model savings", "baseline"),
    ("model-groups-by-user", "User groups", "P11", "Groups used per user", "status"),
    ("usage-by-key", "Key usage", "P11", "Key traffic", "status"),
    ("usage-by-caller", "Caller usage", "P11", "Caller traffic", "status"),
    ("requested-models", "Requested models", "P11", "Requested group distribution", "status"),
    ("provider-model-mix", "Provider/model", "P11", "Selected upstream mix", "status"),
    ("latency-throughput", "Latency", "P11", "Latency + tok/s", "sort"),
    ("errors-fallbacks", "Errors", "P11", "Errors and fallbacks", "sort"),
    ("upstream-failures", "Upstream failures", "P11", "Upstream failure classes", "sort"),
    ("request-shape-failures", "Shape failures", "P11", "Shape filter denials", "sort"),
    ("fallback-health", "Fallback health", "P11", "Fallback outcomes", "sort"),
    ("user-client-impact", "User impact", "P11", "Client-visible impact", "sort"),
    ("cache-report", "Cache", "P11", "Hit/miss/bypass", "cache"),
    ("quotas-budgets", "Quotas", "P11", "Quota/budget consumption", "status"),
    ("traffic-shaping-overview", "Shaping", "P11", "Traffic shape overview", "shape filters"),
    ("traffic-shaping-by-user", "Shaping users", "P11", "Shape by user", "shape filters"),
    ("traffic-shaping-by-key", "Shaping keys", "P11", "Shape by key", "shape filters"),
    ("traffic-shaping-by-client", "Shaping clients", "P11", "Shape by client", "shape filters"),
    ("traffic-shaping-by-group", "Shaping groups", "P11", "Shape by group", "shape filters"),
    ("provider-capacity-shaping", "Provider shaping", "P11", "Provider capacity shaping", "shape filters"),
    ("adaptive-upstream-backoff", "Backoff", "P11", "Adaptive upstream backoff", "shape filters"),
    ("traffic-tuning-advisor", "Tuning advisor", "P11", "Tuning recommendations", "shape+status"),
    ("troubleshooting-buckets", "Troubleshooting", "P11", "Bucketed triage", "sort"),
    ("routing-decisions", "Routing", "P11", "Routing decision aggregates", "sort"),
    ("dynamic-signals", "Dynamic signals", "P11", "Signal telemetry", "sort"),
    ("dynamic-score-buckets", "Dynamic scores", "P11", "Score buckets", "sort"),
    ("dynamic-thresholds", "Dynamic thresholds", "P11", "Threshold hits", "sort"),
    ("max-token-buckets", "Max tokens", "P11", "Output cap buckets", "sort"),
    ("input-token-buckets", "Input tokens", "P11", "Input size buckets", "sort"),
    ("admission-reasons", "Admission", "P11", "Admission allow/deny reasons", "sort"),
    ("provider-catalog-status", "Catalog status", "P11", "Catalog readiness", "sort"),
    ("retention-status", "Retention", "P11", "Retention job status", "sort"),
    ("contract-buckets", "Contracts", "P11", "Contract evaluations", "sort"),
    ("contract-workloads", "Workloads", "P11", "Workload contracts", "sort"),
    ("target-validation", "Validation", "P11", "Target validation evidence", "sort"),
    ("expensive-requests", "Expensive", "P11", "Top expensive requests", "sort"),
    ("client-breakdown", "Clients", "P11", "Client breakdown", "status"),
    ("project-chargeback", "Projects", "P11", "Project chargeback", "status"),
    ("capability-usage", "Capabilities", "P11", "Tools/VLM/structured/stream", "status"),
    ("anomalies", "Anomalies", "P11", "Anomaly/abuse signals", "status"),
    ("security-events", "Security", "P11", "Authz/security events", "sort"),
    ("requests", "Requests", "P11", "Request drilldown search", "status+sort"),
]

rows = "\n".join(
    f"| `{tid}` | {label} | {who} | {purpose} | {filters} |"
    for tid, label, who, purpose, filters in TABS
)

(SHIPPED / "admin-reports.md").write_text(
    f"""# Shipped: Admin browser reports

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
{rows}

## States

- **Empty:** no rows in window — show empty table + widen filter CTA
- **Error:** safe query failure message + request correlation; no SQL with values
- **Unauthorized:** 403; metrics-admin isolation preserved

## Validation

Playwright e2e in `internal/router/admindist/web/e2e/admin-reports.spec.ts` asserts every tab renders.
"""
)

tab_buttons = "\n".join(
    f'      <button data-tab="{tid}" onclick="selectTab(\'{tid}\', this)">{label}</button>'
    for tid, label, *_ in TABS
)
tab_panels = "\n".join(
    f'''    <div class="tile span-9 {"panel-hidden" if i else ""}" data-tab-panel="{tid}">
      <h2>{label}</h2>
      <div class="sub">{purpose} · filters: {filters}</div>
      <div class="progress"><span style="width:{(37 + i * 3) % 90 + 10}%"></span></div>
      <table class="data"><thead><tr><th>Key</th><th>Requests</th><th>Cost</th><th>p95</th></tr></thead>
      <tbody>
        <tr><td>demo-a</td><td>{1200 + i * 17}</td><td>${(12.5 + i * 0.3):.2f}</td><td>{200 + i * 3}ms</td></tr>
        <tr><td>demo-b</td><td>{800 + i * 11}</td><td>${(8.1 + i * 0.2):.2f}</td><td>{180 + i * 2}ms</td></tr>
      </tbody></table>
      <div class="note">Fictional aggregates. Raw prompts/tools/images never appear.</div>
    </div>'''
    for i, (tid, label, who, purpose, filters) in enumerate(TABS)
)

(SHIPPED / "admin-reports-shell.html").write_text(
    f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · Admin reports shell</title>
<link rel="stylesheet" href="{CSS}">
<script>
function selectTab(id, btn){{
  document.querySelectorAll('[data-tab-panel]').forEach(el=>el.classList.add('panel-hidden'));
  document.querySelectorAll('.nav-side button').forEach(b=>b.classList.remove('active'));
  const p=document.querySelector('[data-tab-panel=\"'+id+'\"]');
  if(p) p.classList.remove('panel-hidden');
  if(btn) btn.classList.add('active');
}}
</script>
</head>
<body>
<div class="app">
  <header class="topbar">
    <img class="logo" src="{LOGO}" alt="Metrum AI">
    <div class="title">Admin Reports</div>
    <div class="meta">Shipped surface · fictional · Basic+Casbin</div>
  </header>
  <div class="accent-line"></div>
  <main class="bento">
    <div class="tile span-12">
      <div class="row">
        <label class="field grow">Since<select><option>24h</option><option>7d</option></select></label>
        <label class="field grow">Project<select><option>all</option><option>demo</option></select></label>
        <label class="field grow">Limit<select><option>50</option><option>100</option></select></label>
        <button class="btn primary">Apply</button>
      </div>
    </div>
    <div class="tile span-3 row-3"><h2>Sections</h2>
      <div class="nav-side">
{tab_buttons}
      </div>
    </div>
{tab_panels}
  </main>
  <footer class="footer"><span>Shipped admin reports · {len(TABS)} tabs</span><span>No secrets or prompts</span></footer>
</div>
</body>
</html>
"""
)

# Enterprise self-host
(SHIPPED / "enterprise-self-host.md").write_text(
    """# Shipped: Enterprise self-host

**Personas:** P5 Enterprise operator

**HTML:** [enterprise-self-host.html](enterprise-self-host.html)

## Happy path

| Step | User sees | User does | System |
| --- | --- | --- | --- |
| Download | Package matrix amd64/arm64 | Fetch tarball | Release artifact |
| Install | Binary/Compose/K8s docs | Unpack, place config | — |
| License | Path for license.json | Install signed license | Local verify public keys |
| Keys | env example | Set provider env refs | api_key_env |
| Smoke | Commands | /readyz, /v1/models, API shapes | Pass/fail |
| Renew | Expiry banner | Replace license.json | Recheck |

## Expiry

License grace per config; after grace fail-closed. Renewal via commercial channel — replacement file.
"""
)

(SHIPPED / "enterprise-self-host.html").write_text(
    f"""<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · Enterprise self-host</title><link rel="stylesheet" href="{CSS}"></head>
<body><div class="app">
<header class="topbar"><img class="logo" src="{LOGO}" alt="Metrum AI"><div class="title">Enterprise self-host</div><div class="meta">Shipped · fictional</div></header>
<div class="accent-line"></div>
<main class="bento">
<div class="tile span-3"><h2>1 Package</h2><div class="sub">linux-amd64 / arm64 tarball</div><button class="btn primary">Download mock</button></div>
<div class="tile span-3"><h2>2 License</h2><div class="mono">/app/config/license.json</div><span class="tag ok">valid</span></div>
<div class="tile span-3"><h2>3 Config</h2><div class="sub">config.yaml + env refs</div></div>
<div class="tile span-3"><h2>4 Smoke</h2><span class="tag wait">run</span></div>
<div class="tile span-12"><h2>Smoke checklist</h2>
<table class="data"><thead><tr><th>Check</th><th>Status</th></tr></thead>
<tbody>
<tr><td>/readyz</td><td><span class="tag ok">pass</span></td></tr>
<tr><td>/v1/models</td><td><span class="tag ok">pass</span></td></tr>
<tr><td>Chat / Responses / Messages</td><td><span class="tag wait">queued</span></td></tr>
</tbody></table></div>
</main>
<footer class="footer"><span>Enterprise self-host</span><span>Offline license path unchanged by hosted D3</span></footer>
</div></body></html>
"""
)

(SHIPPED / "api-and-cli.md").write_text(
    """# Shipped: API & CLI developer journey

**Personas:** P3

**HTML:** [api-and-cli.html](api-and-cli.html)

## Flow

1. Obtain key (hosted console once, or self-host token-gen).
2. `GET /v1/models` → allowed groups only.
3. Chat Completions, Responses, Anthropic Messages.
4. Codex CLI (`wire_api=responses`) / Claude Code (`ANTHROPIC_AUTH_TOKEN`, unset API key).

## Error remediation

| Error | Meaning | Next |
| --- | --- | --- |
| entitlement-inactive | No subscription | Subscribe / Portal |
| payment-required | Allowance exhausted | x402 pay + retry |
| payment-invalid | Bad/expired proof | Fresh challenge |
| quota / RPM | Traffic shape | Wait / raise limits |
| upstream-* | Provider path | request_id triage |
"""
)

(SHIPPED / "api-and-cli.html").write_text(
    f"""<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · API &amp; CLI</title><link rel="stylesheet" href="{CSS}"></head>
<body><div class="app">
<header class="topbar"><img class="logo" src="{LOGO}" alt="Metrum AI"><div class="title">API &amp; CLI</div><div class="meta">Shipped · fictional</div></header>
<div class="accent-line"></div>
<main class="bento">
<div class="tile span-4"><h2>/v1/models</h2><textarea readonly>{{"data":[{{"id":"high"}},{{"id":"fast"}}]}}</textarea></div>
<div class="tile span-4"><h2>Chat</h2><textarea readonly>POST /v1/chat/completions</textarea></div>
<div class="tile span-4"><h2>402 overage</h2><span class="tag wait">payment-required</span><div class="sub">x402 challenge then retry</div></div>
<div class="tile span-6"><h2>Codex CLI</h2><div class="mono">wire_api=responses · base https://api.example.invalid/v1</div></div>
<div class="tile span-6"><h2>Claude Code</h2><div class="mono">ANTHROPIC_BASE_URL + AUTH_TOKEN · env -u ANTHROPIC_API_KEY</div></div>
</main>
<footer class="footer"><span>API & CLI</span><span>D11 commercial errors distinct from upstream</span></footer>
</div></body></html>
"""
)

(SHIPPED / "routing-and-policy.md").write_text(
    """# Shipped: Routing & policy

**Personas:** P5 / P11 today (YAML); P4 future (#537)

**HTML:** [routing-and-policy.html](routing-and-policy.html)

## Today

Operators edit `config.yaml` model groups: static, failover, weighted, dynamic_score, script/TS, external policy, PII filter. Validate with smokes; activate via deploy.

## Tomorrow

Routing studio (#537) for tenant-admin weighted/static with approvals; canary (#528) for experimental targets.
"""
)

(SHIPPED / "routing-and-policy.html").write_text(
    f"""<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · Routing &amp; policy</title><link rel="stylesheet" href="{CSS}"></head>
<body><div class="app">
<header class="topbar"><img class="logo" src="{LOGO}" alt="Metrum AI"><div class="title">Routing &amp; policy</div><div class="meta">Shipped YAML · future studio</div></header>
<div class="accent-line"></div>
<main class="bento">
<div class="tile span-6"><h2>Today (YAML)</h2><textarea readonly>models.high:
  strategy: weighted
  targets: [{{provider: …, weight: 70}}, …]</textarea></div>
<div class="tile span-6"><h2>Future (#537)</h2><div class="sub">Routing studio · approvals · rollback fingerprint</div><a class="btn" href="../issues/537-routing-studio.html">Open studio mock</a></div>
</main>
<footer class="footer"><span>Routing</span><span>External policy is trusted infra</span></footer>
</div></body></html>
"""
)

(SHIPPED / "licensing-ops.md").write_text(
    """# Shipped / ops: Licensing operations

**Personas:** P6, P7 (issuer)

**HTML:** [licensing-ops.html](licensing-ops.html)

Issue, renew, replace, top-up enterprise `license.json` via secure channel. Support uses safe scalars only (`license_id`, expiry, grace, request IDs). Never paste private signing keys or real licenses into tickets.
"""
)

(SHIPPED / "licensing-ops.html").write_text(
    f"""<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · Licensing ops</title><link rel="stylesheet" href="{CSS}"></head>
<body><div class="app">
<header class="topbar"><img class="logo" src="{LOGO}" alt="Metrum AI"><div class="title">Licensing ops</div><div class="meta">Internal · fictional</div></header>
<div class="accent-line"></div>
<main class="bento">
<div class="tile span-4"><h2>License ID</h2><div class="mono">lic_demo_…</div></div>
<div class="tile span-4"><h2>SKU</h2><div class="value" style="font-size:16px">enterprise_std</div></div>
<div class="tile span-4"><h2>Expiry</h2><span class="tag wait">30d</span></div>
<div class="tile span-12"><h2>Actions</h2><div class="row"><button class="btn primary">Issue replacement</button><button class="btn">Top-up volume</button><button class="btn">Revocation bundle</button></div>
<div class="note">Deliver via secure channel only. No private keys in tickets.</div></div>
</main>
<footer class="footer"><span>License ops</span><span>Safe scalars only</span></footer>
</div></body></html>
"""
)

print("shipped surfaces written", len(TABS), "tabs")
