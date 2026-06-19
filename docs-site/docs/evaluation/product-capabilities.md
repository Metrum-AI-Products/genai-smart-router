---
title: Product Capabilities
---

# Product Capabilities

This page describes what Smart LLM Router is designed to do today, and where its current boundaries are.

## Implemented Strengths

| Area | Capability |
|---|---|
| API compatibility | OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, filtered `/v1/models`, caller `/v1/usage`, health/version endpoints |
| Routing | Static, weighted, failover, and TypeScript-scripted model group routing |
| Caller governance | Router-issued caller tokens, per-key allow lists, rate limits, token budgets, concurrency limits |
| Provider control | Server-side provider keys, deployment-defined provider catalogs, active targets separate from catalog metadata |
| Tools | Dialect-specific tool metadata and request filtering for OpenAI Chat, OpenAI Responses, and Anthropic Messages |
| Images/VLM | Image input detection across supported request shapes and modality-aware target filtering |
| Cost accounting | Request-time input/output/image prices, calculated cost fields, upstream-reported billed cost fields |
| Usage reporting | Reports by caller, project, environment, token ID, provider, model, model group, client, status, cache, latency, and token counts |
| Observability | JSONL logs, relational usage DB, diagnostics child tables, metrics-admin Prometheus telemetry |
| Caching | In-process LRU/TTL cache for eligible non-tool responses with cache snapshots in usage rows |
| Deployment | Linux binary and Docker Compose packages with embedded hosted Docusaurus docs |
| Agent clients | Codex CLI and Claude Code CLI workflows validated through router-compatible API shapes |
| Private upstreams | OpenAI-compatible vLLM, SGLang, Baseten-style, and other internal services can be configured as providers |

## Product Boundaries

Smart LLM Router is not positioned as:

- a public model marketplace;
- a general-purpose observability SaaS;
- a replacement for every enterprise API gateway product;
- a full enterprise administration dashboard with SSO and compliance workflow automation in this repository;
- an automatic model-quality oracle for every possible prompt.

It is a governed routing gateway. Its value comes from centralizing model access, policy, validation, cost records, and client compatibility in a deployment-owned control point.

## Validation Philosophy

Provider catalogs are metadata, not proof. A model should only become active after the exact deployment validates:

- provider key entitlement;
- direct upstream text behavior;
- realistic token budget behavior;
- small max-token cap behavior when caps matter;
- tool behavior for the intended API shape;
- image behavior when image modality is advertised;
- router-level behavior through the exposed model group.

Models can remain catalog-only until validation passes.

## Deployment Options

Smart LLM Router can be deployed:

- inside an enterprise network;
- in a customer cloud account;
- as a Metrum-managed instance.

Deployment-specific hostnames, model group names, caller policies, provider keys, and upstream mixes are configuration choices, not product constants.

