# `dubday.mp4` — Prosumer Launch Sprint Narrated Video Approval Package

Status: **narration approved; awaiting TTS provider and voice selection**  
Version: **v0.1**  
Prepared: **2026-07-19**  
Proposed deliverable: `dubday.mp4` — 1920×1080, 16:9, 30 fps, H.264/AAC,
approximately 3 minutes 20 seconds.  
Audience: internal product, engineering, go-to-market, security, finance, and
support stakeholders. This is a sprint/release-plan explainer, not a statement
that the product has launched.

## Production guardrail

The complete narration below was approved on 2026-07-19. No speech synthesis,
final audio, captions, or MP4 assembly may start until a TTS provider, voice or
model, and secure active-run credential mechanism are selected. If any narration
wording changes after approval, the revised full narration must be approved
again.

## Narrative objective

Explain how the 2026-07-18 to 2026-07-25 Prosumer Launch sprint turns the GenAI
Smart Router into a self-serve, prepaid, managed offering without putting
commercial billing in inference. The audience should leave with four ideas:

1. The customer flow is concrete: verified signup, prepaid funding,
   provisioning, one-time API key, first request, visible usage, top-up, and
   governed cancellation.
2. The router keeps its safety and privacy posture: provider credentials remain
   server-side; prompts and keys are not operational data; Stripe and the ledger
   remain outside the request path.
3. Every launch-blocking feature has observable test and rollback evidence.
4. The July 18–25 window is an implementation-design and execution-plan sprint;
   production launch requires the listed decision gates and go/no-go evidence.

## Brand and visual direction

Use the Metrum AI visual system: near-black field, official white Metrum logo,
white/gray type hierarchy, Geist for headlines, Poppins for explanatory copy,
Geist Mono for UI labels, and restrained `#FF3132 → #FE005F → #EE0089 →
#CC28AF → #9948CB → #465CDA` accents. Use thin technical rules and deliberate
negative space. No recreated text logo, unrelated colors, rounded consumer-card
look, or unsupported product claims.

The following committed, fictional-data previews are the proposed visual source
for the demo scenes:

* [Checkout and provisioning](../previews/checkout-provisioning.png)
* [Prosumer console](../previews/prosumer-console.png)
* [Canary operations](../previews/canary-operations.png)
* [Operations and reconciliation](../previews/operations-dashboard.png)

Record any final screen captures in a non-production environment with synthetic
identity, payment, prompts, tools, API tokens, and provider names. Never record
raw prompts, images, tool payloads, browser session secrets, Stripe card data,
provider keys, router tokens, token hashes, full configuration, or full grants.

## Complete narration for approval

### S01 — Title and outcome

**Narration:**

> This sprint designs Metrum GenAI Smart Router for a self-serve prosumer
> launch: a developer can get a managed running instance, pay in advance, make a
> first request, and understand what they spent—without staff in the loop.

### S02 — The product boundary

**Narration:**

> The design keeps a hard boundary. The commercial control plane owns identity,
> tenants, payments, price books, the ledger, provisioning, and support. The
> router remains the data plane: it protects provider credentials, filters PII,
> routes requests, applies caller limits, and records usage. Stripe and card
> state never enter the inference hot path.

### S03 — Customer starts safely

**Narration:**

> The journey starts with sign-up, verified email, accepted terms, and an
> organization. A risk policy either grants a small bounded trial or sends the
> customer to Stripe-hosted Checkout. A browser return is not proof of payment.
> Only a verified, signed webhook can create credit.

### S04 — Prepaid balance and the ledger

**Narration:**

> That webhook posts immutable, balanced ledger entries in integer microcredits.
> The ledger distinguishes a quoted reserve, the settled customer charge, the
> calculated upstream cost, provider-billed cost when available, and later
> reconciliation. Credits, holds, reversals, refunds, and adjustments stay
> traceable instead of silently changing a balance.

### S05 — Provisioning path

**Narration:**

> Funding triggers an idempotent provisioning saga. The recommended launch path
> is a shared regional EKS router fleet: a logical tenant gets caller access,
> model groups, quota and traffic-shaping policy, an entitlement, and a signed
> balance grant. A dedicated tier can add namespace, Helm release, secrets,
> DNS, TLS, and license—only when that higher-cost isolation is approved.

### S06 — Healthy before visible

**Narration:**

> A tenant does not become active until health gates pass. We verify readiness,
> allowed models, OpenAI Chat, OpenAI Responses, Anthropic Messages, a credited
> usage settlement, and metrics-admin isolation. Then the console shows an API
> key once, along with tested curl, OpenAI, and Anthropic examples.

### S07 — Safe pay-per-use admission

**Narration:**

> Before any upstream call, the router validates a local, signed, short-lived
> balance grant and reserves the conservative maximum customer price. It does
> not call Stripe, the ledger, or the control plane during inference. Actual
> usage settles asynchronously and releases unused reserve. If authority,
> pricing, or the reservation backend is unsafe, the request fails closed before
> Metrum can incur provider cost.

### S08 — Customer console and support

**Narration:**

> The prosumer console makes the operating state legible: balance and held
> amount, live usage and cost, API-key lifecycle, budgets, receipts, top-up,
> and safe low-balance guidance. Support sees a redacted tenant timeline, not
> customer credentials or request content. Cancellation first revokes grants and
> access, then preserves required financial evidence before governed teardown.

### S09 — Abuse resistance

**Narration:**

> Day-one abuse is part of the launch design. Verified identity, trial velocity
> limits, payment risk signals, per-tier hard caps, bounded authorization holds,
> anomaly alerts, and a tenant kill switch prevent a free-credit or runaway-spend
> path from becoming Metrum’s provider bill.

### S10 — Release engineering

**Narration:**

> Delivery is Git-native and migration-safe. The release train runs source and
> contract checks, signed images with SBOMs, an ephemeral full-stack environment,
> expand-and-migrate compatibility, staging, synthetic customer journeys, soak
> and SLO gates, human approval, then progressive production rollout. A failed
> gate stops promotion and rolls workloads or configuration back without deleting
> financial history.

### S11 — Proof, not promises

**Narration:**

> Each blocking issue closes with proof: unit and PostgreSQL integration tests,
> browser end-to-end tests, concurrency and fault injection, load and security
> tests, redacted terminal replays, safe audit and ledger queries, dashboard
> screenshots, and rollback evidence. Critical invariants include no negative
> prepaid authority, one economic effect per source event, no cross-tenant access,
> and no commercial network call from the router handler.

### S12 — Sprint order

**Narration:**

> The July 18 to 25 sprint starts with the shared account registry, EKS release
> foundation, and threat model. It then builds lifecycle, ledger, signed grants,
> provisioning, fraud protection, payments, console, observability, and
> customer-facing documentation. Experimental model canaries and Marketplace
> artifacts follow as fast-follow work after the launch blockers are proven.

### S13 — Decisions and launch gate

**Narration:**

> Before launch, product, finance, security, SRE, legal, and support must record
> the tenancy, ledger, prepaid pricing, region, provider, identity, trial, and
> customer-error decisions. Go or no-go requires a redacted evidence bundle,
> reconciled finance results, security sign-off, operational readiness, and a
> rollback owner for every exception.

### S14 — Close

**Narration:**

> The goal is simple: give a prosumer a fast path to a running instance while
> keeping Metrum’s cost exposure bounded, customer data protected, and every
> operational claim testable. This sprint turns that goal into an executable,
> reviewable release plan.

## Scene, visual, and timing plan

Estimated spoken durations below are planning estimates at approximately 145
words per minute. After approval, rendered speech durations replace every
estimate. Insert a 0.75-second pause after every non-final scene: hold the
outgoing visual for 0.30 seconds, then crossfade for 0.45 seconds. The final
scene has no trailing pause.

| Scene | Est. speech | Proposed visual / mock screen | On-screen text | Evidence/source |
| --- | ---: | --- | --- | --- |
| S01 | 15s | Black field, official white logo, gradient rule reveal | `PROSUMER LAUNCH SPRINT` / `July 18–25` | Sprint README |
| S02 | 19s | Three-zone animated architecture; data/control/financial bridge | `COMMERCIAL CONTROL PLANE` / `ROUTER DATA PLANE` / `ASYNC SETTLEMENT` | #520 architecture contract |
| S03 | 18s | Checkout/provisioning preview, cropped to signup and payment stages | `VERIFY → FUND → PROVISION` | #520 customer journey; #524 |
| S04 | 20s | Operations dashboard ledger/reconciliation section, annotated | `CREDIT · HOLD · SETTLE · RECONCILE` | #523 truth model |
| S05 | 22s | Checkout/provisioning preview with shared vs dedicated split diagram | `SHARED FLEET DEFAULT` / `DEDICATED BY APPROVAL` | #526 |
| S06 | 18s | Terminal replay-style synthetic montage: ready, models, three API shapes | `HEALTH GATES BEFORE ACTIVE` | #520/#526 test plans |
| S07 | 22s | Animated request flow and reserve/settle lane | `LOCAL SIGNED GRANT` / `NO STRIPE IN INFERENCE` | #522 |
| S08 | 22s | Prosumer console preview, then operations dashboard support timeline | `BALANCE · USAGE · KEYS · TOP-UP` | #525/#521 |
| S09 | 17s | Minimal policy stack: verification, velocity, caps, holds, kill switch | `PROVIDER-COST PROTECTION` | #529 |
| S10 | 22s | Progressive delivery timeline with staging and percentage gates | `BUILD ONCE · PROMOTE DIGESTS` | #527 |
| S11 | 22s | Evidence wall: test grid, safe terminal replay, SLO panel, rollback arrow | `PROOF, NOT PROMISES` | BACKLOG evidence standard |
| S12 | 19s | Ordered sprint dependency graph from README | `BLOCKERS FIRST · FAST FOLLOW AFTER` | Sprint README |
| S13 | 19s | Decision-gate grid and go/no-go checklist | `HUMAN DECISIONS REQUIRED` | D1–D11 register |
| S14 | 14s | Logo close over dark grid and gradient endpoint | `RUNNING INSTANCE. BOUNDED RISK.` | Sprint outcome |

Planning total: approximately **4 minutes 54 seconds of speech** plus **9.75
seconds of inter-scene pauses**, for approximately **5 minutes 04 seconds**.
The 3-minute-20-second header is superseded by this scene calculation; use the
measured final audio duration after approval. If a shorter cut is required, make
an independently approved 90-second version rather than silently trimming this
narration.

## Proposed mock-screen capture list

Use the committed previews as the initial visual assets. If final UI becomes
available, replace only the matching scene with a synthetic-data capture after
the following setup and proof checklist passes.

| Capture | Scene | Required state | Safe visual proof | Do not show |
| --- | --- | --- | --- | --- |
| Signup/Checkout | S03 | verified fictitious user, test-mode Checkout | terms acceptance, hosted redirect, processing state | card number, email, browser/session token |
| Provisioning | S05–S06 | one synthetic tenant undergoing saga | logical tenant stages, health-gate status | cluster IDs, secret refs, tenant slug from real data |
| API key and first request | S06/S08 | synthetic project and one-time key screen | masked key state, `/v1/models`, Chat/Responses/Messages success | raw API key, raw prompts/tools, provider key |
| Balance/usage | S04/S08 | fictional prepaid ledger and usage | available vs held vs settled, redacted receipt | real customer usage, account IDs, financial data |
| Release gate | S10/S11 | staging-only fake deployment | immutable digest prefix, synthetic checks, progressive percent | production endpoints, credentials, full config |
| Canary operations | S11/S12 | synthetic experimental target | shadow/canary/promote/rollback state, SLO signals | raw diagnostics or customer traffic |

## Test-case montage and proof plan

The video should show only outcomes, not sensitive test data. Before publishing,
capture these redacted proof moments in a non-production environment:

| Feature | Test action | Video-safe proof |
| --- | --- | --- |
| Account lifecycle | create → verify → fund fixture → provision → active → suspend/resume → cancel | state timeline advances once; audit identifiers are synthetic |
| Stripe fulfillment | replay signed test webhook and browser-return-only negative case | one ledger credit after verified webhook; no fulfillment from redirect alone |
| Ledger | replay hold, settle, release, refund, and duplicate event fixtures | balanced-entry check is zero; duplicate has no extra economic effect |
| Router admission | race requests against nearly exhausted test grant | accepted maximum never exceeds grant; exhausted call denied before upstream |
| API compatibility | smoke Chat, Responses, Anthropic Messages, tools, streaming, structured output where the eligible test target supports them | 2xx, selected synthetic model label, request ID, no raw content |
| Provisioning | concurrent same-tenant saga plus step-by-step fault injection | one resource outcome; retry or exact compensation; health gate blocks activation |
| Tenant isolation | two-tenant read/write/support attempts | every cross-tenant action denied with no existence disclosure |
| Release safety | invalid signature, failed readiness, migration lock, and SLO breach | promotion stops; prior compatible release returns healthy |
| Observability | create reconciliation lag and canary SLO breach | alert, runbook link, rollback event, recovery panel |

## Release plan and schedule to show

This schedule is an implementation sequence for the sprint planning window, not
a promise of public GA. Work may start in parallel only after its prerequisite
decision and contract are available.

| Date | Primary focus | Completion signal | Release posture |
| --- | --- | --- | --- |
| Jul 18 | Shared account/config registry; D1–D11 owner assignment; epic baseline | owners, environments, secret references, decision review calendar | no customer traffic |
| Jul 19 | Migration-safe delivery foundation and security baseline | build/sign/SBOM path; threat model; test environment | dev and ephemeral only |
| Jul 20 | Control plane lifecycle and ledger contract | relational schema, state-machine and journal invariants | dev integration |
| Jul 21 | Signed grants/router admission and provisioning saga | bounded admission design; EKS desired-state and rollback tests | staging integration |
| Jul 22 | Fraud controls, Stripe webhooks, reconciliation | verified payment flow; abuse and idempotency tests | staging synthetic journey |
| Jul 23 | Prosumer console, support surfaces, observability | onboarding/usage/top-up UI; alerts and dashboards | staging usability and game day |
| Jul 24 | Docs/legal/pricing/support; full release rehearsal | terms/pricing review; full E2E, load, security, restore/rollback evidence | human go/no-go prep |
| Jul 25 | Decision review and release readiness | signed evidence index; exceptions, owners, expiry, rollback triggers | approve staged progressive launch only if all blockers pass |

Post-sprint fast-follow: #528 experimental-model canaries and #532 Marketplace
artifacts begin only after their stated dependencies and launch-blocking
evidence are satisfied. Production rollout remains a separate approval sequence:
0%/shadow → 1% → 10% → 50% → 100%, with health, SLO, reconciliation, abuse, and
synthetic journey gates at each stage.

## Open production choices

These choices materially affect narration claims and final screen recordings:

1. Confirm the intended publishing destination: internal Google Chat, an
all-hands presentation, customer-facing launch content, or a mixed audience.
2. Confirm the final cut: retain this approximately five-minute stakeholder
explainer, or request a separately approved 90-second announcement cut.
3. After narration approval, choose a TTS provider, voice/model, and provide
credentials through the active secure mechanism only. Do not put credentials in
this repository or this manifest.

## Approval request

Please reply with either:

* **“Approve dubday narration v0.1”** to lock this exact narration and move to
  TTS-provider selection; or
* consolidated edits to the narration. I will then publish the complete revised
  narration for a new approval.
