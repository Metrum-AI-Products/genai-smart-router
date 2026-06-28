---
title: Billing, Cancellation, and Runtime Enforcement
---

# Billing, Cancellation, and Runtime Enforcement

GenAI Smart Router separates commercial billing systems from router request handling. The router enforces the active signed license or online lease mode configured for the deployment; it does not store card details or run a public in-router payment wallet.

## Offline License Enforcement

Enterprise self-hosted deployments can use an offline signed `license.json`. The router verifies the license locally and does not need to contact Metrum during startup or periodic license checks. This is the default fit for contracted self-hosted, private-cloud, on-prem, and air-gapped customers.

If a license expires, exceeds a licensed volume/window/concurrency limit, or lacks a required feature, caller requests fail with structured `license-*` errors. See [Error Reference](../reference/errors).

## Online Lease Mode

Some planned commercial plans, especially monthly, card-paid, trial, usage-sensitive, or managed-service plans, may require periodic signed lease renewal. In that mode, the deployment receives a short-lived signed lease from Metrum infrastructure and the router enforces the lease status at runtime.

Online lease mode is planned capability and should not be assumed for offline enterprise contracts. Where used, customer-facing behavior should be documented in the commercial plan: lease renewal interval, grace behavior, payment-required state, cancellation timing, support escalation, and whether traffic is blocked or degraded when renewal fails.

## Stripe Billing Surfaces

Where Stripe is used, Stripe-hosted checkout, invoice, quote, or customer-portal pages may handle payment method updates, invoice downloads, and payments. Stripe public documentation describes webhook-based fulfillment for Checkout, quote/invoice flows for sales-led B2B billing, and Stripe Tax support for Checkout. GenAI Smart Router uses those systems only as commercial control-plane inputs; the router runtime does not process card data.

Portal download, re-download, renewal, or top-up flows are planned until the licensing portal and Stripe fulfillment work is implemented and enabled.

## Cancellation, Refunds, And Revocation

Cancellation or refund behavior depends on the commercial plan:

- offline enterprise licenses usually continue according to the signed license until Metrum issues a replacement, revocation bundle, or corrected license under the contract;
- online lease-required plans can reflect payment-required, canceled, or revoked status at the next lease renewal or grace boundary;
- planned portal-issued evaluation, pilot, or top-up licenses can use portal re-download or replacement flows when available.

For support, share request IDs and safe license status fields only. Do not send provider keys, router tokens, full config, private deployment details, signing material, raw prompts, raw images, raw tool outputs, or full customer-specific license payloads through ordinary support channels.
