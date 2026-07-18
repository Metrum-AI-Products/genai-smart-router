# 12 — Product Docs, Legal, Pricing, and Support Operations (#533)

Plan version: **v1.0.0**  
Issue: [#533](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/533)  
Classification: **launch blocking / final customer-facing gate**

## Design approach

Update public Docusaurus and internal operator docs together only when the related
behavior has shipped. Existing pages that correctly say self-service/Stripe is
planned must remain until the launch gate. Public docs describe hosted behavior,
deployment region, APIs, price/credits, limits, support and errors without private
AWS topology, hostnames, access paths, source-control concepts, secrets, production
config or internal provider mix.

Canonical public information architecture:

* Pricing: approved credit packs/rates, price-book concept, rounding/minimum,
  tax/receipt, trial, auto-top-up, upstream/provider distinction and examples.
* Hosted quickstart: signup -> fund -> provisioning -> key -> `/v1/models` -> exact
  Chat/Responses/Messages first request -> usage/balance -> rotate/revoke.
* Account/billing: payment confirmation, delayed/failure, top-up, receipts/Portal,
  refund/dispute, suspension, cancellation, export/deletion and recovery.
* Limits/errors: RPM/TPM/concurrency/quota/traffic shape, low/exhausted balance,
  license/lease, provisioning and upstream errors with safe remediation.
* Security/privacy: PII and content boundaries, credential handling, shared/dedicated
  isolation, region/residency, retention, incident/status and responsible use.
* Support: intake fields using request/public IDs, severities/response targets,
  status page, account recovery and security escalation.

Each Docusaurus page keeps required `doc_type`, router version banner, one canonical
sidebar home, customer-safe language, and links to `/v1/models` rather than fixed
group assumptions. Internal docs cover configuration, validation, rollout,
rollback, reconciliation, support and incident detail.

## Human interaction and approvals

Legal approves ToS, AUP, Privacy Notice/DPA, cookies/analytics, retention/deletion,
trial, refund/dispute/cancellation, auto-top-up consent, region/residency, sanctions/
acceptable use, subprocessors and claim language. Finance approves all public
numbers/examples/tax/receipts. Product approves packaging and errors. Security/
Compliance approve trust claims and certification scope. Support approves SLA/
severity/intake/escalation/account recovery. Marketing approves public positioning.

Approvals record document/version, approver, date, effective date, jurisdictions,
superseded version, rollback/unpublish plan and customer notice requirement.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Docs need no live
API/provider/Stripe/license/grant/AWS/DB key. Public examples use placeholders and
a documented ignored `tmp/` test project; automated docs E2E obtains a dedicated
stage caller token through secret reference without printing it. Non-secret config
includes public console/API/docs/status/support origins, supported region, plan/
price/policy/legal versions, Stripe Portal behavior (not secret IDs unless useful),
allowed API surfaces, error catalog, limits, retention and support SLAs. Docs build
holds analytics/search IDs only if approved public configuration. Legal source
documents and approval evidence live in the governed legal system; source control
contains approved customer copy, not privileged drafts or signatures.

## Support workflow

1. In-product error gives safe type, request ID, status/docs link and appropriate
   self-service action; it never asks customer to send token/prompt/provider key.
2. Support intake collects account/tenant/public token ID, request IDs, UTC time,
   client/API shape, safe symptom and consented attachment. It warns against
   secrets/content.
3. Triage selects auth/payment/provisioning/balance/router/provider/security,
   queries authorized safe evidence, and escalates by severity/owner.
4. Billing adjustments/refunds use #523/#524 reviewed workflows; support cannot
   edit a balance directly. Security incidents use #530 playbook.
5. Resolution records safe cause/action, customer communication and follow-up.

## Test plan

* `rtk make docs-qa`, public docs checks/build/link checker, frontmatter/sidebar/
  version-banner/release-note validation and package-secret/internal-word scans.
* Run every curl and SDK example exactly against staging; Python uses `uv` in an
  ignored `tmp/` project with placeholder docs and runtime secret injection.
* New-person usability: from pricing/legal through signup/fund/provision/key/request/
  usage/top-up/exhaust/cancel without internal help; record redacted replay/video.
* State/error matrix maps every console/API/email/support state to current docs and
  policy; no dead end or claim that unshipped behavior exists.
* Accessibility, mobile, print/PDF where needed, SEO/canonical, privacy/cookie and
  localization fallback checks.
* Support drills: lost key, stolen account, delayed payment, double charge claim,
  refund/dispute, stuck provisioning, exhausted balance, outage, security report
  and data request. Assert correct role, evidence and response.
* Search stale enterprise-only/planned-self-service wording and duplicated pages;
  update, delete or mark dated historical with reason.

## Release, rollback, and definition of done

Publish behind a launch flag/date coordinated with production readiness. Archive
the superseded approved legal/pricing versions and keep customer notice evidence.
If product rollout is reversed, restore prior contact-led copy immediately and
disable checkout/signup entry points without deleting customer rights/history.
Done when Product, Legal, Finance, Security, Support and Docs owners sign the
state-to-policy matrix and all exact examples/usability/support drills pass.

Monitor docs/console broken links, example synthetic success, search failures,
404s, support contact conversion, policy acceptance-version errors, outdated
version banners, accessibility regressions and customer error-to-help success.
Alerts route to Docs/Product for content and to the owning service for behavioral
mismatch. Never add customer prompt/token/card data to documentation analytics.
