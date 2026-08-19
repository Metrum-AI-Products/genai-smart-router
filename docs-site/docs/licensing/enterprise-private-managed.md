---
title: Enterprise and Private Managed Procurement
doc_type: explanation
---

# Enterprise and Private Managed Procurement

Enterprise self-hosted and private managed paths are intended for customers that need contractual procurement, dedicated deployment planning, private upstream support, security review, or workload-specific validation.

## Enterprise Self-Hosted

The customer operates the router in their own environment and installs a signed `license.json`. This path is appropriate for regulated, customer-cloud, on-prem, or air-gapped deployments.

Commercial and technical planning usually covers:

- annual or contracted license term;
- BYOK provider keys, private upstreams, or approved Metrum-managed provider access;
- deployment scope, instance limits, and optional deployment binding;
- licensed features such as routing, usage reporting, admin reports, dynamic routing, external policy, PII filtering, private upstreams, and exports;
- model-group quality criteria and acceptance tests;
- security assessment, diagnostics redaction, metrics-admin isolation, rollback, and support escalation.

## Private Managed Deployment

In a private managed deployment, Metrum operates a dedicated deployment for one customer or a customer-specific high-availability environment.

The plan should define:

- customer isolation and network controls;
- endpoint handoff and caller-token administration;
- provider-cost handling, including BYOK or pass-through where applicable;
- usage reporting, retention, and support evidence;
- operational acceptance tests and rollback criteria;
- renewal, top-up, cancellation, and incident contacts.

Procurement can use direct order, quote/invoice, or marketplace private offer when available. Share non-secret deployment details through approved private support and operations channels. Exchange credentials such as router tokens, provider keys, or license files only through the governed secret-manager or credential-handoff process defined for the deployment.

For privacy roles, processing categories, subprocessors, and international
transfers, use the canonical [DPA Overview](/docs/dpa), which links the
current subprocessor and transfer schedules. Request engagement-specific
executed copies from [contact@metrum.ai](mailto:contact@metrum.ai).

## Acceptance

Before production use, validate the same client surfaces that will be used after rollout: OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, Codex CLI, Claude Code, tool calls, image/VLM requests, streaming, and structured outputs as applicable.

Use usage reports and admin reports to confirm provider/model selection, request-time cost, latency, fallback behavior, quota enforcement, license status, and access-control results.
