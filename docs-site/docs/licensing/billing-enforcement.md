---
title: Billing, Cancellation, and Runtime Enforcement
doc_type: explanation
---

# Billing, Cancellation, and Runtime Enforcement

GenAI Smart Router separates commercial billing systems from router request handling. The router enforces the active signed license or online lease mode configured for the deployment; it does not store card details or run a public in-router payment wallet.

## Offline License Enforcement

Enterprise self-hosted deployments can use an offline signed `license.json`. The router verifies the license locally and does not need to contact Metrum during startup or periodic license checks. This is the default fit for contracted self-hosted, private-cloud, on-prem, and air-gapped customers.

If a license expires, exceeds a licensed volume/window/concurrency limit, or lacks a required feature, caller requests fail with structured `license-*` errors. See [Error Reference](../reference/errors).

## Commercial Control Plane

Commercial systems such as orders, invoices, private offers, managed-service records, or customer portals can authorize license issuance or replacement. The router runtime receives only the signed license or deployment entitlement it must enforce; it does not process card details, invoice state, refunds, or disputes.

When a commercial plan uses online lease renewal, the plan should state the renewal interval, grace behavior, payment-required state, cancellation timing, support escalation, and whether traffic is blocked or degraded when renewal fails. Offline enterprise contracts continue to use signed license files without a startup network dependency.

## Cancellation, Refunds, And Revocation

Cancellation or refund behavior depends on the commercial plan:

- offline enterprise licenses usually continue according to the signed license until Metrum issues a replacement, revocation bundle, or corrected license under the contract;
- online lease-required plans can reflect payment-required, canceled, or revoked status at the next lease renewal or grace boundary;
- evaluation, pilot, or top-up packages use the replacement workflow defined by the commercial/support plan.

For support, share request IDs and safe license status fields only. Do not send provider keys, router tokens, full config, private deployment details, signing material, raw prompts, raw images, raw tool outputs, or full customer-specific license payloads through ordinary support channels.
