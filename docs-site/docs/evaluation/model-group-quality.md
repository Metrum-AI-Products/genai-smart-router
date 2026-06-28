---
title: Model Group Quality Criteria
---

# Model Group Quality Criteria

Model groups are the caller-facing quality and cost contracts in GenAI Smart Router. A group name should mean something operational: what work it is intended to handle, what clients can use it, what modalities and tools it supports, and what outcome it must preserve while the router optimizes cost, latency, and provider mix behind the scenes.

This is the key product principle: not every task needs the most expensive model. Expensive targets should be reserved for workloads that require them. Simpler text, extraction, summarization, and routine coding work can often be served by lower-cost routes when validation shows the group still meets its objective.

For a repeatable router-versus-fixed-model proof plan, see [Prove Router Quality](./prove-router-quality).

## Group Contract Fields

Define these fields for every model group in a deployment:

| Field | Description |
|---|---|
| Group name | Deployment-defined caller-facing name |
| Intended users | Teams, apps, agents, or environments allowed to use the group |
| Intended workloads | The tasks the group is expected to complete |
| API shapes | OpenAI Chat, OpenAI Responses, Anthropic Messages |
| Modalities | Text, image/VLM, or other configured modalities |
| Tool dialects | OpenAI Chat tools, Responses function tools, Anthropic client tools |
| Quality target | Objective success threshold for this group |
| Cost target | Cost/request, daily budget, or savings target against a baseline |
| Latency target | p50/p95 latency or token-throughput target |
| Reliability target | Timeout, fallback, and provider error thresholds |
| Validation harness | Harbor, workload-specific tests, or another objective harness |
| Promotion criteria | When to increase target weight or caller access |
| Rollback criteria | When to reduce weight, disable a target, or isolate the group |

## Example Group Contracts

| Group Type | Intended Workloads | Example Success Criteria |
|---|---|---|
| Low-cost general group | short chat, summarization, extraction, simple code edits | extraction accuracy threshold, simple unit tests pass, latency and cost target met |
| Balanced developer group | day-to-day coding, refactors, tool use, occasional image context | coding task tests pass, tool-call file assertion passes, image-bearing requests select VLM-capable targets |
| Coding-agent group | Codex CLI, Claude Code, multi-step tool tasks | Harbor reward score meets threshold, created files pass verifier, fallback and timeout rates stay within target |
| VLM-capable group | OCR, screenshot reasoning, browser-control context | image smoke passes, OCR target accuracy meets threshold, image cost fields populate |
| Reasoning-capable group | explicit OpenAI reasoning or Anthropic thinking requests | direct and router reasoning smokes pass, requested reasoning controls are forwarded or safely translated, no-compatible-target requests fail before upstream |
| Private-model group | internal vLLM/SGLang or private GPU workloads | direct upstream and router smokes pass, private endpoint remains hidden from callers, chargeback values populate |

These are examples only. The deployment chooses group names and contracts that match its teams, applications, and governance model.

For the reasoning metadata, caller examples, and negative test behind a reasoning-capable group, see [Reasoning Routing](../configuration/reasoning-routing).

## Agentic Quality Validation

Harbor-style validation is useful because it tests the whole agent loop, not only a single completion. For a coding-agent group, run tasks that require the agent to inspect files, call tools, edit artifacts, and pass an external verifier.

Track:

- task reward score or pass/fail result;
- agent runtime errors;
- selected provider/model;
- fallback count;
- input, output, cache, and image token counts;
- elapsed time and token throughput;
- request-time router cost and upstream-reported billed cost;
- generated artifact correctness.

Promotion is justified when the group maintains the required outcome while meeting cost, latency, and reliability targets. A cheaper provider mix should be adopted when it passes the same objective criteria. A stronger target should remain available for workloads that need it, but it does not need to handle every request.

Use the [Prove Router Quality](./prove-router-quality) decision matrix when deciding whether to promote a group, keep a fixed model, split a workload, or collect more evidence.

## Release Gates

Before a group receives broad caller access:

- direct upstream smokes pass for each active target;
- router-level smokes pass for each required API shape;
- tool requests are validated with real tool calls when tools are advertised;
- image requests are validated with realistic VLM budgets when image modality is advertised;
- reasoning or thinking requests are validated for each advertised API shape and target control mode;
- capped request behavior is tested for the caller API's output cap field, including OpenAI Chat `max_completion_tokens`;
- usage rows include selected provider/model, token counts, status, latency, cache behavior, and cost fields;
- Harbor or workload-specific validation meets the group success criteria;
- rollback criteria are documented.

## Ongoing Governance

Review model groups after provider price changes, model entitlement changes, provider instability, new client requirements, new modalities, or observed quality regressions. The router makes model substitution operationally easier, but the group contract determines whether a substitution is acceptable.
