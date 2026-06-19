---
title: Enterprise Evaluation Guide
---

# Enterprise Evaluation Guide

Use this guide to evaluate GenAI Smart Router for an enterprise deployment. The goal is to prove routing control, caller governance, multimodal and agent compatibility, private upstream access, cost accounting, and operational safety with real requests.

## Platform Evaluation

Confirm the deployment shape:

- on-prem, customer cloud, or Metrum-managed instance;
- TLS/ingress ownership;
- where provider keys are stored;
- whether private upstreams such as vLLM, SGLang, or Baseten-style endpoints are required;
- which model groups callers should see;
- which clients need OpenAI Chat, OpenAI Responses, or Anthropic Messages compatibility.

Acceptance checks:

- `/readyz` and `/version` return build metadata.
- Browser docs show the same package version and build timestamp.
- `/v1/models` is filtered by the presented caller token.
- Disallowed model groups return `403 model-not-allowed` before any provider call.

## Security Evaluation

Confirm:

- provider keys stay server-side;
- caller tokens are stored only as hashes in config;
- raw tokens, token hashes, provider keys, raw prompts, and raw image payloads are not written to diagnostics;
- `/metrics` is restricted to metrics-admin tokens;
- ordinary callers use `/v1/usage` or generated reports instead of global metrics.

Acceptance checks:

- ordinary caller token receives `403 metrics-forbidden` on `/metrics`;
- metrics-admin token can scrape `/metrics`;
- usage reports contain public token IDs and metadata, not raw secrets.

## Developer Experience Evaluation

Validate real clients, not only curl:

- Codex CLI through `/v1/responses`;
- Claude Code CLI through `/v1/messages`;
- OpenAI-compatible chat clients through `/v1/chat/completions`;
- Warp-style OpenAI Chat tool calls when agent tools are required.

For CLI tool tests, require the agent to create a file and assert the file contents. Text-only responses are not sufficient evidence for tool compatibility.

## Routing Evaluation

Test each required route type:

- weighted routing;
- failover behavior;
- TypeScript scripted routing if custom policy is needed;
- tool-only target selection;
- image/VLM target selection;
- capped `max_tokens` or `max_output_tokens` behavior.

For new upstream models, validate direct provider behavior first, then router behavior. Do not activate a model broadly until text, tools, image, and cap smokes pass for the request shapes the deployment will serve.

## Finance Evaluation

Confirm usage reports answer these questions:

- Who used the router?
- Which project and environment generated usage?
- Which router group was requested?
- Which provider/model actually served the request?
- How many input, output, and image tokens were recorded?
- What request-time prices and calculated costs were used?
- Were any upstream-reported billed costs returned?
- How much traffic was cached or bypassed?

GenAI Smart Router stores cost inputs on each usage row so historical reports do not need to be recalculated from current provider prices.

## Operations Evaluation

Require:

- timestamped deployment/config backups;
- local production snapshot synchronized with live config when production changes;
- clear rollback path;
- request ID based troubleshooting;
- cleanup of old packages and temporary files after deployment.

Deployment-specific operator runbooks should cover the exact hostnames, backup locations, credentials process, and rollback commands for the selected environment.
