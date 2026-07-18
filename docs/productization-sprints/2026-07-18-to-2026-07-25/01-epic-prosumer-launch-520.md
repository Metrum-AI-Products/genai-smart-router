# 01 — Prosumer Launch Delivery Contract (#520)

Plan version: **v1.0.0**  
Issue: [#520](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/520)  
Classification: **epic / launch gate**

## Outcome

A verified person can fund a prepaid balance, receive an operational endpoint and
write-only API token, make a request through a Metrum-managed EKS router, see the
settled charge, top up, and cancel without staff intervention. Metrum's maximum
provider-cost exposure remains bounded when dependencies fail or calls race.

## Architecture contract

The delivery comprises three trust zones:

1. **Commercial control plane:** identity, organization, tenant state, Stripe,
   price books, ledger, provisioning orchestration, notifications, and support.
2. **Provisioned data plane:** the existing Go router, provider credentials,
   request admission, routing, PII filtering, usage persistence, diagnostics, and
   metrics-admin telemetry.
3. **Asynchronous financial bridge:** signed balance grants flow control plane to
   router; idempotent settlement events flow router to ledger. Neither direction
   carries prompts, raw tools/images, provider keys, router token secrets, card
   data, or complete configuration.

The control plane may be unavailable without immediately stopping already-funded
traffic, but the signed grant's finite value and expiry cap exposure. Once it
expires or is consumed, admission fails closed before an upstream request.

## Customer journey

1. User signs up, verifies email, accepts legal terms, and creates an organization.
2. Risk policy either grants a bounded trial or requires Stripe-hosted Checkout.
3. A signed payment webhook credits the ledger; the browser redirect alone never
   proves payment.
4. Provisioning creates tenant configuration. Under the recommended shared model,
   it activates a logical caller and lease/grant on a regional fleet. A dedicated
   plan additionally creates a namespace/release, secrets, DNS/TLS, and license.
5. Health gates test `/readyz`, authenticated `/v1/models`, and one non-billable or
   explicitly credited smoke before exposing the endpoint/token.
6. Token is shown once; the console supplies exact curl/OpenAI/Anthropic examples.
7. Each request reserves worst-case customer price from a local grant, routes,
   records usage, and settles actual price asynchronously.
8. Low balance prompts top-up; exhaustion denies before provider use. Cancellation
   revokes future grants/keys, preserves required financial evidence, and queues
   governed teardown.

See [the customer-flow prototype](previews/checkout-provisioning.html).

## Human gates and configuration

Resolve D1-D11 in the sprint index/root register. Record decisions as reviewed ADRs
with effective date, approver, rollback condition, and affected issue numbers.
Configuration must be environment-scoped and secret-referenced: identity issuer,
Stripe product/price IDs and webhook secret reference, region/cell, public console
and API origins, signing key IDs, ledger DSN reference, email sender, trial policy,
price-book version, hold buffer/expiry, and alert destinations.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Enterprise-owned
dev/stage accounts are required for AWS, Stripe, OIDC/email, DNS/TLS, provider
upstreams, GitHub environments, and support/status. Production equivalents require
primary/backup owners and a rotation test. The router service account must be
unable to read Stripe, OIDC-client, commercial-DB, or signing private credentials;
the control-plane service must be unable to read provider API keys. Record safe
account aliases, resource IDs, non-secret Stripe Price IDs, public origins,
region/cell, and price/policy versions in reviewed config. Store credential values
only in KMS/Secrets Manager and inject references through scoped workload identity.

## Integrated acceptance test

| Phase | Action | Expected proof |
| --- | --- | --- |
| Signup | New browser session creates and verifies an account | user/org audit events; no active caller yet |
| Funding | Complete Stripe test-mode Checkout | verified webhook receipt; balanced credit postings; one fulfillment |
| Provision | Run saga to healthy | tenant `active`; validated config/license/grant; DNS/TLS if dedicated |
| First request | Call `/v1/models`, then Chat, Responses, and Messages smokes | allowed groups only; 2xx; usage/request IDs; no commercial hot-path calls |
| Settlement | Consume usage | hold/reservation released; one balanced debit; console balance converges |
| Concurrency | Race requests near remaining balance | admitted maximum never exceeds signed grant/provider exposure cap |
| Recovery | Crash webhook, ledger consumer, and provisioner at each boundary | retries converge once; no duplicate credit/resource/token |
| Exhaustion | Spend to zero | safe balance error before upstream attempt; top-up restores service |
| Cancellation | Cancel then attempt traffic | future grants/keys revoked; retention preserved; teardown audit complete |
| Security | Cross-tenant, invalid signature, secret/log scans | denied; no secret/content leakage; `/metrics` still metrics-admin only |

## Evidence and go/no-go

Create a release evidence index containing environment/version, test run IDs,
redacted terminal replays, UI screenshots, safe DB invariant queries, SLO panels,
security report, backup/restore drill, and rollback. Finance, Product, Security,
SRE, Legal, and Support must sign the go/no-go checklist. Any unresolved launch
exception must name an owner, expiry, containment, and rollback trigger.

## Integrated monitoring and rollback

The epic launch view must join customer-journey synthetics, control-plane state,
Stripe fulfillment, ledger integrity/reconciliation, grant exposure, provisioning,
router/provider SLOs, abuse controls and support/status readiness without joining
on secrets or content. Pages fire for cross-tenant exposure, unbalanced journal,
admission above signed authority, negative balance, lost payment fulfillment,
reconciliation threshold, stuck provision, expired grant and customer API burn.
Rollback order is disable new signup/Checkout, preserve webhook/ledger intake,
stop new grants/provisions, return data-plane/config to the last compatible release,
run reconciliation/synthetics, and communicate status. Existing funded customer
rights and financial history are never deleted as a rollback shortcut.

## Out of scope

x402 (#506), postpaid credit exposure, public multi-region failover, arbitrary
customer routing editors, hardcoded product model-group names, and changes to the
enterprise offline-license path.
