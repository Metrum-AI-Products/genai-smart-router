#!/usr/bin/env python3
"""Generate UX issue narratives + bento HTML mocks. Run from repo root."""
from __future__ import annotations

import pathlib

ROOT = pathlib.Path(__file__).resolve().parent
ISSUES = ROOT / "issues"
PREVIEWS = ROOT / "previews"
SHIPPED = ROOT / "shipped"
VIDEO = ROOT / "video"
CSS = "../_ds/metrum-bento.css"
LOGO = "../_ds/metrum-logo-white.png"

ISSUES.mkdir(parents=True, exist_ok=True)
PREVIEWS.mkdir(parents=True, exist_ok=True)
SHIPPED.mkdir(parents=True, exist_ok=True)
VIDEO.mkdir(parents=True, exist_ok=True)


def md_narrative(
    num: int,
    slug: str,
    title: str,
    personas: str,
    depends: str,
    steps: list[tuple[str, str, str, str, str]],
    alts: list[str],
    fields: str,
    validation: str,
    expiry: str,
) -> str:
    rows = "\n".join(
        f"| {s} | {a} | {i} | {o} | {b} |" for s, a, i, o, b in steps
    )
    alt = "\n".join(f"- {x}" for x in alts)
    return f"""# #{num} — {title}

- **Issue:** https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/{num}
- **Commercial model check:** subscription + included allowance + x402 overage (D3)
- **Personas:** {personas}
- **Depends on:** {depends}
- **HTML mock:** [{num}-{slug}.html]({num}-{slug}.html)

## Happy path

| Screen | Actor | Inputs | Outputs | Backend |
| --- | --- | --- | --- | --- |
{rows}

## Alternate paths

{alt}

## Exact fields / payloads (fictional)

{fields}

## Validation & evidence

{validation}

## Expiry / teardown / rollback

{expiry}
"""


SPECS: list[dict] = [
    {
        "num": 520,
        "slug": "epic-journey",
        "title": "Epic journey board",
        "personas": "P1–P11 (index)",
        "depends": "All launch-blocking children",
        "steps": [
            ("Epic board", "Orchestrator", "Open child links", "Status per wave", "Milestone checklist"),
            ("Go/No-Go", "Product+Finance+Sec", "Sign evidence", "Launch decision", "Evidence index"),
        ],
        "alts": ["Blocked NEEDS-HUMAN decision → stop wave", "Child red CI → no #534"],
        "fields": "Wave table; PR links; evidence paths under `ux/`.",
        "validation": "Every launch-blocking issue has MD+HTML; E2E #534 green.",
        "expiry": "N/A — epic tracking only.",
        "tiles": """
    <div class="tile span-12"><h2>Launch waves</h2>
      <table class="data"><thead><tr><th>Wave</th><th>Issues</th><th>Status</th></tr></thead>
      <tbody>
        <tr><td>0 Foundations</td><td>#507 #516–#519 #527</td><td><span class="tag wait">ready</span></td></tr>
        <tr><td>1 Control plane</td><td>#521 #540 #523</td><td><span class="tag wait">ready</span></td></tr>
        <tr><td>2 Admission/pay</td><td>#522 #506 #524 #526</td><td><span class="tag wait">ready</span></td></tr>
        <tr><td>3 Surfaces</td><td>#525 #529 #531 #530</td><td><span class="tag wait">ready</span></td></tr>
        <tr><td>4 Gate</td><td>#533 #534</td><td><span class="tag off">blocked</span></td></tr>
      </tbody></table>
    </div>
    <div class="tile span-4"><h2>Commercial model</h2><div class="value" style="font-size:16px">Sub + allowance + x402</div><div class="sub">Stripe off hot path</div></div>
    <div class="tile span-4"><h2>Master E2E</h2><a href="../e2e-subscription.md">e2e-subscription.md</a></div>
    <div class="tile span-4"><h2>INDEX</h2><a href="../INDEX.md">ux/INDEX.md</a></div>
""",
    },
    {
        "num": 521,
        "slug": "control-plane",
        "title": "Control plane: accounts & entitlement",
        "personas": "P2, P6",
        "depends": "#507",
        "steps": [
            ("Signup", "P2", "Email+OIDC", "pending_email", "users row"),
            ("Verify", "P2", "Link click", "pending_funding", "audit"),
            ("Org+tenant", "P2", "Name+region", "tenant draft", "orgs/tenants"),
            ("Key create", "P2", "Name", "Raw once", "hash+public ID"),
            ("Suspend", "P6", "Reason", "suspended", "revoke grants"),
        ],
        "alts": ["Cross-tenant read → 404/403", "Control plane down → router keeps last grant until expiry"],
        "fields": "`POST /v1/api-keys` → `{public_id, token_once}`; states: pending_email→…→canceled",
        "validation": "Transition matrix unit tests; key absent from DB plaintext",
        "expiry": "Cancel revokes keys; retain audit/ledger",
        "tiles": """
    <div class="tile span-3"><h2>Tenant</h2><div class="value" style="font-size:18px">ten_demo_01</div><span class="tag ok">active</span></div>
    <div class="tile span-3"><h2>Entitlement</h2><div class="value" style="font-size:18px">starter_mix</div><div class="sub">period ends 2026-08-18</div></div>
    <div class="tile span-3"><h2>Org</h2><div class="value" style="font-size:18px">Acme Demo</div></div>
    <div class="tile span-3"><h2>Keys</h2><div class="value">2</div><div class="sub">1 active · 1 revoked</div></div>
    <div class="tile span-8 row-2"><h2>State machine</h2>
      <div class="timeline">
        <div class="item"><span class="mark">✓</span><span>pending_email → verified</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>pending_funding → subscription webhook</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>provisioning → active</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark off">○</span><span>past_due / suspended / canceled</span><small>idle</small></div>
      </div>
    </div>
    <div class="tile span-4 row-2 emphasis"><h2>Create API key</h2>
      <label class="field">Name<input value="ci-bot"></label>
      <button class="btn primary">Create (show once)</button>
      <div class="note">Raw key shown once. Hash stored. Never in logs.</div>
    </div>
""",
    },
    {
        "num": 540,
        "slug": "plan-catalog",
        "title": "Plan catalog",
        "personas": "P1, P2, P7",
        "depends": "#521",
        "steps": [
            ("Finance edit", "P7", "SKU+units+bands", "plan_versions row", "activation audit"),
            ("Customer picker", "P2", "Select SKU", "Checkout Price ID", "server-side map"),
        ],
        "alts": ["Invalid overlapping bands rejected", "v2 activation does not rewrite in-flight v1 grants"],
        "fields": "plan_starter_mix_v3: $49/mo, 50M/10M tokens, groups default/fast/high, x402 bands…",
        "validation": "Catalog validation unit tests; Finance sign-off before prod numbers",
        "expiry": "Version stamp on grants/quotes; historical never reprices",
        "tiles": """
    <div class="tile span-4"><h2>Catalog version</h2><div class="value" style="font-size:18px">v12</div><span class="tag live">staging</span></div>
    <div class="tile span-8"><h2>SKU starter_mix</h2>
      <dl class="kv">
        <dt>Stripe Price</dt><dd>price_starter_mix_monthly</dd>
        <dt>Included in</dt><dd>50_000_000 tokens</dd>
        <dt>Included out</dt><dd>10_000_000 tokens</dd>
        <dt>Groups</dt><dd>default, fast, high</dd>
        <dt>Overage</dt><dd>x402 bands per group</dd>
      </dl>
    </div>
    <div class="tile span-12"><h2>x402 bands (fictional)</h2>
      <table class="data"><thead><tr><th>Group</th><th>Shape</th><th>Quote USD</th></tr></thead>
      <tbody>
        <tr><td>default</td><td>text ≤8k</td><td>0.002</td></tr>
        <tr><td>high</td><td>tools+stream</td><td>0.02</td></tr>
        <tr><td>fast</td><td>image present</td><td>0.05</td></tr>
      </tbody></table>
    </div>
""",
    },
    {
        "num": 523,
        "slug": "ledger",
        "title": "Double-entry ledger",
        "personas": "P7, P8",
        "depends": "#507 #521",
        "steps": [
            ("Subscription credit", "Worker", "Webhook event", "Balanced postings", "journal+postings"),
            ("Allowance grant", "Worker", "Period start", "Grant liability", "allowance accounts"),
            ("Settle usage", "Outbox", "request_id", "Debit allowance / x402", "idempotent"),
        ],
        "alts": ["Duplicate settlement → one posting", "Unbalanced insert rejected"],
        "fields": "Accounts: sub_revenue, allowance_liability, x402_settlement, provider_cost_memo",
        "validation": "10k replay twice → one set; SQL debit=credit",
        "expiry": "Commercial retention independent of diagnostics purge",
        "tiles": """
    <div class="tile span-3"><h2>Balance check</h2><div class="value" style="font-size:18px">0 Δ</div><span class="tag ok">balanced</span></div>
    <div class="tile span-3"><h2>Postings / day</h2><div class="value">12.4k</div></div>
    <div class="tile span-3"><h2>Reconcile lag</h2><div class="value" style="font-size:18px">42s</div></div>
    <div class="tile span-3"><h2>Duplicates blocked</h2><div class="value">18</div></div>
    <div class="tile span-12"><h2>Journal (safe)</h2>
      <table class="data"><thead><tr><th>Time</th><th>Source</th><th>Debit</th><th>Credit</th><th>Ref</th></tr></thead>
      <tbody>
        <tr><td>12:01:02Z</td><td>subscription</td><td>cash</td><td>sub_revenue</td><td>sub_…</td></tr>
        <tr><td>12:01:03Z</td><td>allowance_grant</td><td>allowance_exp</td><td>allowance_liab</td><td>grant_…</td></tr>
        <tr><td>12:14:11Z</td><td>usage_settle</td><td>allowance_liab</td><td>usage</td><td>req_…</td></tr>
        <tr><td>12:44:09Z</td><td>x402_settle</td><td>x402_recv</td><td>usage</td><td>pay_…</td></tr>
      </tbody></table>
    </div>
""",
    },
    {
        "num": 522,
        "slug": "admission",
        "title": "Admission: entitlement + allowance + x402 handoff",
        "personas": "P3, P8",
        "depends": "#521 #523 #540",
        "steps": [
            ("Within allowance", "Router", "Request", "200 + settle", "Local grant reserve"),
            ("Exhausted", "Router", "Request", "402 payment-required", "No upstream"),
            ("After x402", "Router", "Retry+proof", "200", "#506 settle then upstream"),
        ],
        "alts": ["Grant expired → deny", "Race N pods ≤ grant", "No Stripe in handler trace"],
        "fields": "Grant: in_remaining, out_remaining, price_book_version, expires_at",
        "validation": "Concurrency + fault injection tests in issue body",
        "expiry": "Fail closed at grant expiry/outbox cap",
        "tiles": """
    <div class="tile span-4"><h2>Entitlement</h2><span class="tag ok">active</span><div class="sub">starter_mix · us-east-1</div></div>
    <div class="tile span-4"><h2>Allowance in</h2><div class="progress"><span style="width:62%"></span></div><div class="sub">31.0M / 50M remaining</div></div>
    <div class="tile span-4"><h2>Allowance out</h2><div class="progress"><span style="width:18%"></span></div><div class="sub">1.8M / 10M remaining</div></div>
    <div class="tile span-12">
      <div class="tabs">
        <button class="active" onclick="showPanel('ok',this)">Within allowance</button>
        <button onclick="showPanel('pay',this)">Needs x402</button>
        <button onclick="showPanel('deny',this)">Entitlement inactive</button>
      </div>
      <div data-panel="ok">
        <div class="timeline">
          <div class="item"><span class="mark">1</span><span>Auth + group allowlist</span><small class="ok">ok</small></div>
          <div class="item"><span class="mark">2</span><span>Reserve worst-case from grant</span><small class="ok">ok</small></div>
          <div class="item"><span class="mark">3</span><span>Upstream</span><small class="ok">ok</small></div>
          <div class="item"><span class="mark">4</span><span>Settle actual · release remainder</span><small class="ok">ok</small></div>
        </div>
      </div>
      <div class="panel-hidden" data-panel="pay">
        <div class="note">Allowance &lt; quote → HTTP 402 payment-required → handoff to #506. No upstream call.</div>
      </div>
      <div class="panel-hidden" data-panel="deny">
        <div class="note">entitlement-inactive · remediation URL to console subscribe · request_id req_demo_…</div>
      </div>
    </div>
""",
    },
    {
        "num": 506,
        "slug": "x402-overage",
        "title": "x402 overage payment",
        "personas": "P3, P7, P9",
        "depends": "#522 #540",
        "steps": [
            ("Challenge", "Router", "No proof", "402 + PAYMENT-REQUIRED", "quote frozen"),
            ("Pay", "Client", "Sign payment", "Retry headers", "Facilitator"),
            ("Settle", "Router", "Valid proof", "200 + settlement hdr", "One upstream + ledger"),
        ],
        "alts": ["Replay → payment-invalid", "Facilitator down → payment-unavailable", "Upstream fail after settle → support ref"],
        "fields": "Quote id `q_…`, amount `0.020000`, asset/network fictional, expiry 120s",
        "validation": "Fake facilitator + test-network smoke; Chat/Responses/Messages",
        "expiry": "Quote TTL; no auto refund",
        "tiles": """
    <div class="tile span-3"><h2>State</h2><span class="tag wait">payment-required</span></div>
    <div class="tile span-3"><h2>Quote</h2><div class="value" style="font-size:18px">$0.02</div><div class="sub">q_demo_9f3a</div></div>
    <div class="tile span-3"><h2>Group</h2><div class="value" style="font-size:18px">high</div></div>
    <div class="tile span-3"><h2>Upstream</h2><span class="tag err">blocked</span></div>
    <div class="tile span-7 row-2"><h2>Challenge (safe)</h2>
      <textarea readonly>HTTP/1.1 402 Payment Required
PAYMENT-REQUIRED: <x402-requirements-opaque>
{"error":"payment-required","request_id":"req_demo_…","quote_id":"q_demo_9f3a"}</textarea>
    </div>
    <div class="tile span-5 row-2"><h2>After settlement</h2>
      <div class="timeline">
        <div class="item"><span class="mark">✓</span><span>Verify proof</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>Settle once</span><small class="ok">pay_demo_…</small></div>
        <div class="item"><span class="mark">✓</span><span>Upstream once</span><small class="ok">200</small></div>
        <div class="item"><span class="mark">✓</span><span>Ledger posting</span><small class="ok">ok</small></div>
      </div>
    </div>
""",
    },
    {
        "num": 524,
        "slug": "stripe-subscription",
        "title": "Stripe subscription checkout",
        "personas": "P2, P7",
        "depends": "#521 #523 #540",
        "steps": [
            ("Checkout", "P2", "Confirm plan", "Stripe hosted", "Session + idempotency"),
            ("Webhook", "Stripe", "invoice.paid", "Entitlement active", "Fulfillment worker"),
            ("Portal", "P2", "Manage PM", "Portal session", "No card in Metrum DOM"),
        ],
        "alts": ["Redirect without webhook → no entitlement", "Duplicate event → one fulfillment"],
        "fields": "mode=subscription; success_url confirms; Price from catalog only",
        "validation": "Stripe CLI E2E; SAQ-A boundary scan",
        "expiry": "Cancel/refund via Portal → async entitlement revoke",
        "tiles": """
    <div class="tile span-4"><h2>Checkout</h2><span class="tag ok">session complete</span><div class="sub">cs_test_demo_…</div></div>
    <div class="tile span-4"><h2>Webhook</h2><span class="tag ok">verified</span><div class="sub">evt_demo_…</div></div>
    <div class="tile span-4"><h2>Fulfillment</h2><span class="tag wait">grant issuing</span></div>
    <div class="tile span-8"><h2>Confirming payment</h2>
      <div class="note">Browser success URL is not payment proof. Waiting for signed webhook fulfillment.</div>
      <div class="progress"><span style="width:70%"></span></div>
    </div>
    <div class="tile span-4"><h2>Portal</h2><button class="btn primary">Open Customer Portal</button><div class="sub">Invoices · payment method · cancel</div></div>
""",
    },
    {
        "num": 526,
        "slug": "provisioning",
        "title": "EKS provisioning & teardown",
        "personas": "P2, P8",
        "depends": "#521 #507 #516-519",
        "steps": [
            ("Saga start", "Worker", "Funded tenant", "op running", "Idempotent steps"),
            ("Activate", "Worker", "Config+grant", "Smokes", "Shared fleet"),
            ("Ready", "Console", "—", "Key reveal", "Mark active"),
            ("Cancel", "P2/P6", "Confirm", "Revoke then teardown", "Retention hold"),
        ],
        "alts": ["Fail mid-saga → compensate exact step", "Dedicated path adds DNS/TLS"],
        "fields": "op_id, desired_version, compensating_action",
        "validation": "Kind/EKS double-create; isolation suite",
        "expiry": "Cancel: revoke grants → drain → evidence → delete ephemeral",
        "tiles": """
    <div class="tile span-12"><h2>Provisioning saga</h2>
      <div class="timeline">
        <div class="item"><span class="mark">✓</span><span>Validate funded + region</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>Logical tenant + caller + allowlist</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>Secret refs + entitlement + allowance grant</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>x402 verifier public config</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark wait">•••</span><span>Smokes: readyz · models · Chat/Responses/Messages</span><small class="wait">running</small></div>
        <div class="item"><span class="mark off">○</span><span>DNS/TLS (dedicated only)</span><small>n/a shared</small></div>
      </div>
    </div>
    <div class="tile span-6"><h2>Topology</h2><div class="sub">Shared regional EKS fleet · hostname api.example.invalid · logical isolation</div></div>
    <div class="tile span-6"><h2>Cancel teardown</h2><div class="sub">Revoke grants/keys → drain → retain ledger → delete ephemeral after approval</div></div>
""",
    },
]


def more_specs() -> list[dict]:
    return [
        {
            "num": 525,
            "slug": "prosumer-console",
            "title": "Prosumer console",
            "personas": "P2, P3",
            "depends": "#521 #522 #523 #524 #506 #540",
            "steps": [
                ("Home", "P2/P3", "—", "Allowance+x402 tiles", "Scoped APIs"),
                ("Keys", "P2", "Create/rotate", "Token once", "Control plane"),
                ("Billing", "P2", "Portal link", "Stripe Portal", "#524"),
                ("Support", "P3", "Correlation ID", "Case created", "Safe IDs only"),
            ],
            "alts": ["Exhausted → payment-required panel", "Suspended → remediation"],
            "fields": "Nav: Home, Keys, Usage, Allowance, x402, Billing, Support",
            "validation": "Playwright states; no secret in DOM",
            "expiry": "Cancelled org read-only history",
            "tiles": """
    <div class="tile span-2"><h2>Plan</h2><div class="value" style="font-size:16px">Starter</div></div>
    <div class="tile span-2"><h2>Status</h2><span class="tag ok">active</span></div>
    <div class="tile span-4"><h2>Allowance</h2><div class="progress"><span style="width:55%"></span></div><div class="sub">in 55% · out 22%</div></div>
    <div class="tile span-4"><h2>x402 spend MTD</h2><div class="value" style="font-size:18px">$12.40</div></div>
    <div class="tile span-3 row-2"><h2>Nav</h2><div class="nav-side">
      <button class="active">Home</button><button>Keys</button><button>Usage</button>
      <button>Allowance</button><button>x402</button><button>Billing</button><button>Support</button>
    </div></div>
    <div class="tile span-9 row-2"><h2>First request</h2>
      <textarea readonly>curl -s https://api.example.invalid/v1/models \\
  -H "Authorization: Bearer $METRUM_KEY"</textarea>
      <div class="row"><button class="btn primary">Copy</button><span class="tag">Chat · Responses · Messages</span></div>
    </div>
""",
        },
        {
            "num": 529,
            "slug": "abuse-fraud",
            "title": "Abuse & fraud controls",
            "personas": "P6, P9, P2",
            "depends": "#521 #522 #524 #506",
            "steps": [
                ("Signup risk", "System", "Signals", "Allow/deny trial", "Risk policy"),
                ("Kill switch", "P6", "Tenant ID", "Suspended", "Revoke grants"),
                ("Customer msg", "P2", "—", "Limit guidance", "No rule leakage"),
            ],
            "alts": ["Risk service outage → fail-safe paid-only", "False positive unsuspend"],
            "fields": "Safe reason codes only; no prompts",
            "validation": "Abuse fixtures; load test neighbor isolation",
            "expiry": "Appeal window documented",
            "tiles": """
    <div class="tile span-4"><h2>Review queue</h2><div class="value">7</div><span class="tag wait">open</span></div>
    <div class="tile span-4"><h2>Kill switches 24h</h2><div class="value">1</div></div>
    <div class="tile span-4"><h2>Trial blocks</h2><div class="value">14</div></div>
    <div class="tile span-12"><h2>Case</h2>
      <table class="data"><thead><tr><th>Tenant</th><th>Signal</th><th>Action</th></tr></thead>
      <tbody>
        <tr><td>ten_demo_99</td><td>signup_velocity</td><td>hold_trial</td></tr>
        <tr><td>ten_demo_17</td><td>x402_burst</td><td>rate_limit</td></tr>
      </tbody></table>
    </div>
""",
        },
        {
            "num": 527,
            "slug": "release-engineering",
            "title": "Release engineering gates",
            "personas": "P8",
            "depends": "#507 #516-519",
            "steps": [
                ("Build", "CI", "Signed image", "SBOM", "Attestation"),
                ("Stage", "CI", "Smokes", "Promote/block", "Evidence"),
                ("Rollback", "P8", "Prior digest", "Health restore", "GitOps"),
            ],
            "alts": ["Failed smoke blocks prod", "Unsigned image rejected"],
            "fields": "digest, chart version, migration window",
            "validation": "Stage-to-prod rehearsal",
            "expiry": "N/A",
            "tiles": """
    <div class="tile span-3"><h2>Image</h2><span class="tag ok">signed</span></div>
    <div class="tile span-3"><h2>SBOM</h2><span class="tag ok">pass</span></div>
    <div class="tile span-3"><h2>Migrate</h2><span class="tag ok">compat</span></div>
    <div class="tile span-3"><h2>Smoke</h2><span class="tag wait">running</span></div>
    <div class="tile span-12"><h2>Evidence gates</h2>
      <div class="timeline">
        <div class="item"><span class="mark">✓</span><span>Unit/integration</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>Postgres migration drill</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark wait">•••</span><span>Staging E2E subscription path</span><small class="wait">…</small></div>
        <div class="item"><span class="mark off">○</span><span>Prod-like promote</span><small>blocked</small></div>
      </div>
    </div>
""",
        },
        {
            "num": 530,
            "slug": "security-compliance",
            "title": "Security & tenant isolation",
            "personas": "P9",
            "depends": "#521 #524 #526 #527 #506",
            "steps": [
                ("Threat model", "P9", "Surfaces list", "Controls map", "ADR"),
                ("Pentest", "External", "Staging", "Findings", "Remediation"),
                ("Sign-off", "P9", "Exception register", "Launch OK", "Evidence"),
            ],
            "alts": ["Open critical finding → no launch", "Metrics admin isolation regression"],
            "fields": "Checklist: identity, webhook, x402 facilitator, secrets, EKS network",
            "validation": "Authz matrix + scans + tabletop",
            "expiry": "Exception expiry dates required",
            "tiles": """
    <div class="tile span-12"><h2>Assessment checklist</h2>
      <table class="data"><thead><tr><th>Control</th><th>Status</th></tr></thead>
      <tbody>
        <tr><td>Tenant isolation (usage/config/secrets)</td><td><span class="tag ok">pass</span></td></tr>
        <tr><td>Stripe SAQ-A (no card in DOM)</td><td><span class="tag ok">pass</span></td></tr>
        <tr><td>x402 facilitator SSRF/allowlist</td><td><span class="tag wait">pending</span></td></tr>
        <tr><td>/metrics metrics_admin only</td><td><span class="tag ok">pass</span></td></tr>
      </tbody></table>
    </div>
""",
        },
        {
            "num": 531,
            "slug": "observability-oncall",
            "title": "Observability & on-call",
            "personas": "P8",
            "depends": "Wave 2 + #527",
            "steps": [
                ("Dashboard", "P8", "—", "SLO panels", "Metrics"),
                ("Alert", "System", "Lag threshold", "Page", "Runbook"),
                ("Game day", "P8", "Inject fault", "Recover", "Timeline"),
            ],
            "alts": ["Control-plane outage → grant-bounded traffic", "x402 facilitator health red"],
            "fields": "SLOs: signup, provision, webhook, grant, x402, reconcile",
            "validation": "Synthetic probes + game-day evidence",
            "expiry": "N/A",
            "tiles": """
    <div class="tile span-2"><h2>Signup</h2><div class="value" style="font-size:16px">99.2%</div></div>
    <div class="tile span-2"><h2>Provision p95</h2><div class="value" style="font-size:16px">2.1m</div></div>
    <div class="tile span-2"><h2>Webhook lag</h2><div class="value" style="font-size:16px">9s</div></div>
    <div class="tile span-2"><h2>x402 settle</h2><div class="value" style="font-size:16px">180ms</div></div>
    <div class="tile span-2"><h2>Reconcile Δ</h2><div class="value" style="font-size:16px">$0.00</div></div>
    <div class="tile span-2"><h2>Pages</h2><div class="value" style="font-size:16px">0</div></div>
    <div class="tile span-12"><h2>On-call</h2><div class="sub">Primary: sre-demo · Secondary: finance-oncall · Status page components: API, Checkout, x402</div></div>
""",
        },
        {
            "num": 533,
            "slug": "docs-legal-support",
            "title": "Docs, legal, support",
            "personas": "P1, P2, P6",
            "depends": "Customer-visible blockers",
            "steps": [
                ("Pricing page", "P1", "—", "Plan disclosure", "Approved copy"),
                ("Support intake", "P2", "Topic+ID", "Ticket", "Template"),
                ("Error docs", "P3", "Error type", "Remediation", "D11 map"),
            ],
            "alts": ["Unapproved price claim blocked", "Stale prepaid copy fails QA"],
            "fields": "ToS, AUP, privacy, refund, SLA, status, pricing/allowance/x402",
            "validation": "docs-qa + usability to first request",
            "expiry": "Policy version captured at signup",
            "tiles": """
    <div class="tile span-6"><h2>Pricing disclosure</h2>
      <div class="sub">Base monthly · included tokens by mix · x402 overage · taxes · refunds</div>
      <button class="btn primary">Subscribe</button>
    </div>
    <div class="tile span-6"><h2>Support intake</h2>
      <label class="field">Topic
        <select><option>Billing</option><option>API error</option><option>Abuse appeal</option></select>
      </label>
      <label class="field">Request ID<input placeholder="req_…"></label>
      <button class="btn">Submit</button>
    </div>
""",
        },
        {
            "num": 534,
            "slug": "integrated-beta",
            "title": "Integrated beta acceptance",
            "personas": "P2, P3, P7, P8, P9",
            "depends": "#521-527 #529-531 #540 #506",
            "steps": [
                ("E2E run", "QA", "Test customer", "Pass/fail", "Evidence pack"),
                ("Go/No-Go", "Leads", "Checklist", "Launch", "Sign-off"),
            ],
            "alts": ["Any critical fail → no launch"],
            "fields": "Checklist mirrors issue #534 acceptance criteria",
            "validation": "Full staging journey recorded",
            "expiry": "N/A",
            "tiles": """
    <div class="tile span-12"><h2>#534 acceptance</h2>
      <table class="data"><thead><tr><th>Criterion</th><th>Result</th></tr></thead>
      <tbody>
        <tr><td>Checkout → webhook → entitlement + allowance</td><td><span class="tag wait">pending</span></td></tr>
        <tr><td>/v1/models + three API shapes within allowance</td><td><span class="tag wait">pending</span></td></tr>
        <tr><td>Exhaustion → x402 → one upstream</td><td><span class="tag wait">pending</span></td></tr>
        <tr><td>Cancel denies; no Stripe in router traces</td><td><span class="tag wait">pending</span></td></tr>
        <tr><td>No secret leakage in console/logs</td><td><span class="tag wait">pending</span></td></tr>
      </tbody></table>
    </div>
""",
        },
        {
            "num": 528,
            "slug": "canary",
            "title": "Model canary ops",
            "personas": "P10",
            "depends": "#505 #7 #507 #531",
            "steps": [
                ("Propose", "P10", "Target+evidence", "Draft canary", "Config set"),
                ("Shadow", "System", "Traffic sample", "Metrics", "No response change"),
                ("Promote/rollback", "P10", "Approve", "Weights", "Auto rollback gates"),
            ],
            "alts": ["Quality breach → auto rollback", "Prosumer not enrolled → no experimental"],
            "fields": "exposure %, shape gates, SLO thresholds",
            "validation": "Fault inject each rollback trigger",
            "expiry": "Canary expiry timestamp required",
            "tiles": """
    <div class="tile span-3"><h2>Canary</h2><div class="value" style="font-size:16px">2%</div></div>
    <div class="tile span-3"><h2>Quality</h2><span class="tag ok">pass</span></div>
    <div class="tile span-3"><h2>p95</h2><div class="value" style="font-size:16px">410ms</div></div>
    <div class="tile span-3"><h2>Rollback</h2><span class="tag ok">armed</span></div>
    <div class="tile span-12"><h2>Proposal</h2><div class="sub">provider/model/skin + request_shape_support + evidence timestamps · approve/promote/abort</div>
      <div class="row"><button class="btn primary">Approve stage</button><button class="btn">Abort → prior fingerprint</button></div>
    </div>
""",
        },
        {
            "num": 532,
            "slug": "marketplace",
            "title": "AWS Marketplace / AMI",
            "personas": "P1 procurement, P5",
            "depends": "#521 #523 #527 #533",
            "steps": [
                ("Subscribe", "Buyer", "Offer", "Register", "Control plane link"),
                ("AMI launch", "P5", "Instance role", "Bootstrap", "BYOK+license"),
            ],
            "alts": ["Fast-follow; does not replace Stripe+x402 hosted"],
            "fields": "product code, dimensions, private offer id",
            "validation": "Sandbox buyer flow; no hot-path metering",
            "expiry": "Unsubscribe stops entitlement per contract",
            "tiles": """
    <div class="tile span-6"><h2>SaaS offer</h2><div class="sub">Register → link org → provision hosted → async metering</div><button class="btn primary">Continue registration</button></div>
    <div class="tile span-6"><h2>AMI / container BYOC</h2><div class="sub">Hardened artifact · entitlement check · customer provider secrets · smokes</div><button class="btn">Launch guide</button></div>
""",
        },
        {
            "num": 541,
            "slug": "auto-tune",
            "title": "Auto-tune cheapest mix",
            "personas": "P10, P4",
            "depends": "#392-394 #501 #528 #537 #521",
            "steps": [
                ("Job create", "P10", "Dataset+min pass", "Job queued", "Central tuner"),
                ("Search", "Worker", "Candidates", "Best config", "Evidence bundle"),
                ("Activate", "P4/P10", "Approve", "Routing studio / canary", "Rollback fingerprint"),
            ],
            "alts": ["Fail quality → no activation", "Hot path never calls tuner"],
            "fields": "min_pass_rate, max_p95_ms, max_cost, candidate allowlist",
            "validation": "Synthetic dataset cheaper config proof",
            "expiry": "Job retention policy",
            "tiles": """
    <div class="tile span-4"><h2>Min pass</h2><div class="value" style="font-size:18px">92%</div></div>
    <div class="tile span-4"><h2>Best cost</h2><div class="value" style="font-size:18px">$0.11</div><div class="sub">per task</div></div>
    <div class="tile span-4"><h2>Pass achieved</h2><div class="value" style="font-size:18px">93.4%</div></div>
    <div class="tile span-12"><h2>Output artifact</h2>
      <textarea readonly>models.demo-tuned:
  strategy: weighted
  targets:
    - {provider: baseten, model_ref: gpt-oss-120b, weight: 70}
    - {provider: crusoe, model_ref: gemma-4-31b, weight: 30}
# evidence: eval_job_demo_… fingerprint cfg_…</textarea>
      <button class="btn primary">Send to routing studio approval</button>
    </div>
""",
        },
        {
            "num": 535,
            "slug": "byok-vault",
            "title": "BYOK vault",
            "personas": "P4",
            "depends": "#521",
            "steps": [
                ("Add key", "P4", "Secret once", "public_id", "Secrets Manager write"),
                ("Rotate", "P4", "New secret", "Overlap then revoke", "Audit"),
            ],
            "alts": ["Control plane cannot read secret back", "Validation_failed state"],
            "fields": "provider kind, name, secret (write-only)",
            "validation": "Secret absent from Postgres; RBAC+reauth",
            "expiry": "Revoke stops upstream in bounded window",
            "tiles": """
    <div class="tile span-8"><h2>Credentials</h2>
      <table class="data"><thead><tr><th>Public ID</th><th>Provider</th><th>State</th></tr></thead>
      <tbody>
        <tr><td>cred_demo_01</td><td>openai</td><td><span class="tag ok">active</span></td></tr>
        <tr><td>cred_demo_02</td><td>baseten</td><td><span class="tag wait">validating</span></td></tr>
      </tbody></table>
    </div>
    <div class="tile span-4 emphasis"><h2>Add</h2>
      <label class="field">Provider<select><option>openai</option><option>baseten</option></select></label>
      <label class="field">API key<input type="password" value="••••••••"></label>
      <button class="btn primary">Store (write-only)</button>
    </div>
""",
        },
        {
            "num": 536,
            "slug": "upstream-onboarding",
            "title": "Upstream connection onboarding",
            "personas": "P4",
            "depends": "#535",
            "steps": [
                ("Draft connection", "P4", "Adapter+URL+cred", "Draft", "Egress check"),
                ("Probes", "Worker", "Synthetic", "Evidence rows", "Per skin"),
                ("Select models", "P4", "Passing only", "Draft routing", "#537"),
            ],
            "alts": ["Probe fail → bounded reason", "Never browser hot-path probes"],
            "fields": "adapter, base_url, credential public_id, skins claimed",
            "validation": "Scalar evidence only; no raw provider bodies",
            "expiry": "Revalidate on DNS/endpoint change",
            "tiles": """
    <div class="tile span-12"><h2>Probe results</h2>
      <table class="data"><thead><tr><th>Model</th><th>Chat</th><th>Tools</th><th>Image</th><th>Messages</th></tr></thead>
      <tbody>
        <tr><td>gpt-oss-120b</td><td><span class="tag ok">pass</span></td><td><span class="tag ok">pass</span></td><td><span class="tag err">n/a</span></td><td><span class="tag ok">pass</span></td></tr>
      </tbody></table>
    </div>
""",
        },
        {
            "num": 537,
            "slug": "routing-studio",
            "title": "Routing studio",
            "personas": "P4",
            "depends": "#7 #521 #536",
            "steps": [
                ("Draft group", "P4", "Targets+weights", "Validation", "Server checks"),
                ("Approve", "Owner", "Reauth", "Activate", "Version+rollback"),
            ],
            "alts": ["Invalid weights rejected", "v1 static/weighted only"],
            "fields": "group name, targets, weights, shape envelope",
            "validation": "Simulation uses synthetic templates only",
            "expiry": "Rollback to prior version fingerprint",
            "tiles": """
    <div class="tile span-5"><h2>Draft group coding</h2>
      <label class="field">Strategy<select><option>weighted</option><option>static</option></select></label>
      <table class="data"><thead><tr><th>Target</th><th>Weight</th></tr></thead>
      <tbody><tr><td>baseten/gpt-oss-120b</td><td>70</td></tr><tr><td>crusoe/gemma-4</td><td>30</td></tr></tbody></table>
    </div>
    <div class="tile span-7"><h2>Validation</h2>
      <div class="timeline">
        <div class="item"><span class="mark">✓</span><span>Targets validated + credential active</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark">✓</span><span>Weights sum 100</span><small class="ok">ok</small></div>
        <div class="item"><span class="mark wait">•••</span><span>Approval required</span><small class="wait">owner</small></div>
      </div>
      <button class="btn primary">Submit for approval</button>
    </div>
""",
        },
        {
            "num": 538,
            "slug": "private-upstream",
            "title": "Private upstream connectivity",
            "personas": "P4, P9",
            "depends": "#536",
            "steps": [
                ("Request private", "P4", "Host+profile", "network_review", "Ops review"),
                ("Activate", "P8", "Approve egress", "active", "NetworkPolicy"),
            ],
            "alts": ["SSRF ranges rejected", "Public shared tenancy: approved public only at launch"],
            "fields": "canonical URL, egress profile, region class",
            "validation": "Connectivity tests; no arbitrary proxy",
            "expiry": "DNS change → revalidation",
            "tiles": """
    <div class="tile span-6"><h2>Endpoint</h2><div class="mono">https://llm.customer.example.invalid/v1</div><span class="tag wait">network_review</span></div>
    <div class="tile span-6"><h2>Egress profile</h2><div class="sub">PrivateLink · deny metadata · HTTPS only · approved ports</div></div>
""",
        },
        {
            "num": 539,
            "slug": "byok-attribution",
            "title": "BYOK cost attribution",
            "personas": "P4, P7",
            "depends": "#521 #522 #523 #535 #536",
            "steps": [
                ("View costs", "P4", "Filters", "Attribution table", "Scalar usage"),
                ("Reconcile", "P7", "Run", "Report", "No provider invoices stored"),
            ],
            "alts": ["BYOK must not draw pooled grant", "Unknown upstream cost labeled unknown"],
            "fields": "funding_source pooled|byok, estimate vs upstream billed",
            "validation": "Finance attribution review",
            "expiry": "N/A",
            "tiles": """
    <div class="tile span-12"><h2>Attribution</h2>
      <table class="data"><thead><tr><th>Funding</th><th>Connection</th><th>Requests</th><th>Router est.</th><th>Upstream billed</th></tr></thead>
      <tbody>
        <tr><td>pooled</td><td>—</td><td>1200</td><td>$40.00</td><td>$28.10</td></tr>
        <tr><td>byok</td><td>cred_demo_01</td><td>300</td><td>$9.00</td><td>$8.40</td></tr>
      </tbody></table>
    </div>
""",
        },
    ]


def write_issue(spec: dict) -> None:
    num = spec["num"]
    slug = spec["slug"]
    md = md_narrative(
        num,
        slug,
        spec["title"],
        spec["personas"],
        spec["depends"],
        spec["steps"],
        spec["alts"],
        spec["fields"],
        spec["validation"],
        spec["expiry"],
    )
    (ISSUES / f"{num}-{slug}.md").write_text(md)
    body = spec["tiles"]
    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Metrum UX · #{num} {spec['title']}</title>
<link rel="stylesheet" href="{CSS}">
<script>
function showPanel(id, btn){{
  const root=btn?btn.closest('.tile')||document:document;
  root.querySelectorAll('[data-panel]').forEach(el=>el.classList.add('panel-hidden'));
  root.querySelectorAll('.tabs button').forEach(b=>b.classList.remove('active'));
  const p=root.querySelector('[data-panel=\"'+id+'\"]');
  if(p) p.classList.remove('panel-hidden');
  if(btn) btn.classList.add('active');
}}
</script>
</head>
<body>
<div class="app">
  <header class="topbar">
    <img class="logo" src="{LOGO}" alt="Metrum AI">
    <div class="title">#{num} · {spec['title']}</div>
    <div class="meta">Static UX mock · fictional data</div>
  </header>
  <div class="accent-line"></div>
  <main class="bento">
{body}
  </main>
  <footer class="footer">
    <span>#{num} · subscription + included allowance + x402 overage</span>
    <span>No real credentials, prices, or customer data</span>
  </footer>
</div>
</body>
</html>
"""
    (ISSUES / f"{num}-{slug}.html").write_text(html)


def main() -> None:
    for spec in SPECS + more_specs():
        write_issue(spec)
    print(f"wrote {len(SPECS)+len(more_specs())} issue pairs to {ISSUES}")


if __name__ == "__main__":
    main()
