# 10 — Prosumer Onboarding and Console (#525)

Plan version: **v1.0.0**  
Issue: [#525](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/525)  
Classification: **launch blocking**

## Information architecture and mockup

Build an accessible responsive web app served separately from the router. The
navigation is Overview, API Keys, Usage, Limits, Billing, Members, and Support.
The static design is committed at [prosumer-console.html](previews/prosumer-console.html);
the payment/provisioning state journey is at
[checkout-provisioning.html](previews/checkout-provisioning.html).

Overview prioritizes time-to-first-request: provisioning status, endpoint, copyable
curl/SDK sample, allowed groups from authenticated `/v1/models`, balance/held/
estimated runway, current spend, limits, and service status. API Keys shows public
ID/name/created/last-used/status only; raw key is shown in one modal immediately
after creation with a required copy/download acknowledgement and never retrievable.
Billing shows credit packs, top-up, auto-top-up policy, receipts and a short-lived
Stripe Portal redirect. Usage shows customer-price aggregates, not Metrum upstream
cost or global/admin telemetry.

## Page and state inventory

* Signup, verify email, terms/privacy consent, organization name, launch region.
* Funding choice: eligible trial or prepaid Checkout; `confirming`, `paid`,
  `failed`, `refunded/disputed`, and `needs-action` states.
* Provisioning timeline: account, funding, deployment/config, license/lease,
  DNS/TLS if dedicated, service smoke, API-key readiness; retry/support on failure.
* Overview empty/active/low balance/exhausted/past-due/suspended/canceled/outage.
* API key create/show-once/list/rename/rotate/revoke and compromised-key emergency.
* Allowed model groups and API shape examples driven by `/v1/models`; never assume
  fixed product group names.
* Usage filters by project/key/model group/time with stored customer price/tokens/
  requests/errors; CSV only if scoped and retention-approved.
* Budgets/limits display plus allowed customer-set lower caps; requests to raise
  hard tier caps are support/product flows, not silent UI changes.
* Members/roles, billing/receipts, support case with safe request ID, status/docs/
  legal links, cancellation/export/deletion state.

## Customer interaction flow

1. OIDC signup/verification and server-side session; resume interrupted onboarding.
2. Create org/tenant, choose supported immutable region, accept versioned terms.
3. Fund/trial; redirect to hosted Stripe and return to server-confirmed status.
4. Show live provisioning stages. No token appears until endpoint, entitlement,
   grant, and service smokes pass.
5. Show raw token once and guide `/v1/models`, then one Chat/Responses/Messages
   example appropriate to selected client. The first request page waits for usage/
   ledger correlation and marks onboarding complete.
6. Send low-balance alerts and in-app top-up. Exhausted calls show D11 error, request
   ID, top-up action and status link without revealing internal pricing/provider.
7. Cancel: explain access cutoff and retention/refund consequences, reauthenticate,
   confirm, revoke future grants/keys, and display teardown state.

## Human interaction and configuration

Product/Design approve flows/copy, plan/region selection, state transitions,
customer price terminology, and mobile/a11y target. Security approves OIDC/session,
key display/download, CSP/CORS/CSRF, analytics and reauth. Legal approves terms,
privacy, refund/cancel/trial copy. Support approves case fields, SLAs, escalation and
account recovery. Finance approves Billing/auto-top-up/receipt presentation.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Browser receives
only public console/API origins, OIDC public client/issuer data, safe plan/region/
model metadata, feature flags and support/status/docs links. Server/BFF holds any
OIDC client secret, session key, control-plane identity, Stripe restricted key for
short-lived Checkout/Portal sessions, and support integration in Secrets Manager.
No provider, license/grant private, raw token hash, DB, AWS or webhook secret enters
the browser bundle. Configure callback/logout/CORS/CSP origins, cookie/session/CSRF,
API timeouts, terms versions, plan/region catalog, feature flags, polling/backoff,
price display/locale, analytics allowlist/redaction, email template IDs, support
fields and status component. Customer API keys are CSPRNG-generated server-side,
shown once, never placed in URL/localStorage/analytics, and rotate by public ID.

## Test plan

* Component/unit tests cover every page/state, formatting/rounding, safe errors,
  retry/idempotency, copy behavior, key show-once lifecycle, and permission gates.
* Playwright E2E: new signup -> verify -> fund -> provision -> key -> `/v1/models`
  -> first request -> usage/debit -> low/exhausted -> top-up -> rotate -> cancel.
* Separate E2Es for interrupted sessions, delayed/failed payment, provisioning
  failure/retry, suspended/disputed account, dependency outage, and support case.
* Two-tenant/role authorization matrix across every API and route; no existence
  leak, cached cross-account page, or browser back-button secret recovery.
* WCAG automated/manual checks: keyboard, focus, screen reader labels, contrast,
  zoom/reflow, reduced motion, errors/status announcements and responsive layouts.
* Security tests for XSS/CSRF/CSP/CORS/session fixation, open redirect, key in DOM/
  URL/storage/telemetry, cache headers, clickjacking and dependency vulnerabilities.
* Exact documented Python client flow runs with `uv` in ignored `tmp/`; curl,
  OpenAI and Anthropic examples use placeholder token and staging endpoint.
* Capture sanitized Playwright video/screenshots/network trace and terminal replay;
  assert no secrets/card data/prompts appear in artifacts.

## Monitoring, rollback, and definition of done

Measure Web Vitals, JS/error rate, login/verification, onboarding step conversion/
age, Checkout return-to-fulfillment, provisioning, key and first-request success,
API latency, low-balance/top-up, support clicks and accessibility synthetic. Use
privacy-approved tenant-safe IDs only. Roll back static console independently;
server maintains compatible API and old/new asset versions. Done when a person
with no internal context completes the staging journey from docs alone and
Product/Design/Security/Support approve evidence.
