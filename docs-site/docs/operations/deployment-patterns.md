---
title: Deployment Patterns
doc_type: explanation
---

# Deployment Patterns

GenAI Smart Router is deployment-owned infrastructure. Choose boundaries that
match provider-key custody, state ownership, network policy, reporting,
retention, and release cadence.

## Central Gateway

One router per environment or network trust boundary gives applications a
stable endpoint while a platform team owns provider credentials, model groups,
caller access, budgets, and reports. Use PostgreSQL for a validated
multi-replica design; keep SQLite to one writer.

## Per-Environment Or Per-Team

Separate routers reduce blast radius when development, staging, production,
regions, or teams need independent credentials, provider eligibility, state,
retention, or release timing. Promote configuration only after the exact Chat,
Responses, Messages, tool, image, and streaming shapes used by that environment
pass.

## Federated Routers

A router can call another compatible router as an upstream, but every hop must
have independent authentication, group access, timeout budgets, request-ID
correlation, and failure testing. This is an API composition pattern, not a
turnkey topology controller.

## Private Model Serving

Private vLLM/SGLang-style services can be configured as OpenAI-compatible
upstreams. Keep endpoints on protected networks and validate served model IDs,
parser/chat templates, tools, streaming, and rollback directly and through the
router before promotion.

## Selection Checklist

- Who owns provider credentials and caller access?
- Is one writer sufficient, or is validated PostgreSQL multi-replica operation
  required?
- Which clients, dialects, tools, modalities, and request sizes must pass?
- What quality, latency, and cost contract does each model group have?
- Which logs, reports, metrics, backups, and retention controls are required?
- Can the operator restore the previous artifact, config, license inputs, and
  database within the planned recovery window?

See [Installation](../installation/), [Architecture And
Limitations](../reference/architecture-limitations), [Deployment
Readiness](../evaluation/deployment-readiness), and the [Upgrade
Guide](../release-notes/upgrade-guide).

