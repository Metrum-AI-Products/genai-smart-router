---
title: Evaluate GenAI Smart Router
doc_type: explanation
---

# Evaluate GenAI Smart Router

Use this checklist to evaluate GenAI Smart Router with a real deployment or a Metrum-managed evaluation instance. The goal is to verify client compatibility, governance, routing behavior, cost evidence, and operational trust signals before rollout. If the first question is whether the product fits a common enterprise concern, start with [Enterprise FAQ](./enterprise-faq). If the first question is where the router should run, start with [Enterprise Deployment Patterns](../operations/deployment-patterns).

<div class="contactBanner">
  <p>Need an evaluation environment or help choosing proof points? Contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

When the question is whether a routed group is as good as a fixed model or previous policy, use [Prove Router Quality](./prove-router-quality) to run an evidence-first comparison instead of relying on anecdotes.

## What To Prove First

| Question | Proof point |
|---|---|
| Can existing clients integrate with minimal change? | Run OpenAI Chat, OpenAI Responses, Anthropic Messages, or SDK/CLI traffic by changing base URL, token, and model group. |
| Are provider keys and private endpoints hidden? | Confirm clients receive only the router endpoint, router token, and model group names. |
| Can the platform control who uses what? | Call `/v1/models` with different tokens and verify allow-list filtering. |
| Are tool and image requests routed safely? | Run one tool call and one image request through a group with validated compatible targets. |
| Does the router explain spend and performance? | Generate or review a report with provider/model, tokens, cost, latency, throughput, cache, attempts, and fallback fields. |
| Can model groups be tuned by outcome? | Run Harbor or another verifier and compare reward/pass status against cost and latency. |

## Commercial Evaluation Flow

Commercial evaluations can run on a Metrum-managed hosted service, an enterprise/on-prem licensed package, or a private customer-cloud deployment. The commercial path is:

1. request an evaluation through [contact@metrum.ai](mailto:contact@metrum.ai);
2. receive a router endpoint and token, or a deployment package plus signed JSON license for customer-controlled infrastructure;
3. validate production-like workloads and clients;
4. inspect savings, performance, quota, provider mix, security access, and retention/rollup evidence;
5. choose the commercial path that fits the deployment: enterprise self-hosted, private managed, marketplace/private-offer, renewal, top-up, or another contracted plan.

Production access uses either a signed enterprise license for customer-operated deployments or a private managed plan for Metrum-operated customer deployments. Use the contact address above for evaluation setup, license delivery, and deployment-specific access details.

## Thirty-Minute Evaluation Path

1. Receive a router base URL, a caller token, and one or more allowed model groups.
2. Run [`/v1/models`](../getting-started/available-models) and verify only allowed groups appear.
3. Run an OpenAI-compatible chat smoke from the [Hosted Quickstart](../getting-started/hosted-quickstart).
4. Run the client that matters most: [Codex CLI](../getting-started/codex-cli), [Claude Code CLI](../getting-started/claude-code-cli), or an OpenAI-compatible SDK.
5. Run one request-shape test: image input, tool call, quota/rate-limit behavior, or a deployment-specific policy route.
6. Ask the administrator for the usage/report excerpt for the evaluation window.
7. Confirm selected upstream provider/model, token counts, cost fields, latency, fallback behavior, and request status are visible without storing raw prompts or raw images.

This path should prove both caller-visible behavior and operator controls. The caller sees stable model-group names, compatibility with its chosen API shape, allow-list enforcement, and clear errors such as `model-not-allowed` or `no-eligible-target`. The operator sees request-time evidence for target selection, cost, latency, attempts, fallback, and quota impact.

## Expected First Outputs

| Step | Successful result |
|---|---|
| `/v1/models` | JSON list of model group IDs allowed for the token. |
| Chat smoke | Assistant returns the requested short text. |
| Codex or Claude Code smoke | CLI returns the requested text or creates the requested disposable smoke file. |
| Image smoke | Model reads the image through the same router group or returns `no-eligible-target` when the group has no validated image target. |
| Usage report | Report groups requests by caller, project, model group, provider/model, status, latency, tokens, cache, and cost. |

## Proof Package To Request

- One text request through `/v1/chat/completions`.
- One Responses request or Codex CLI task.
- One Anthropic Messages request or Claude Code task.
- One tool-call request using the API shape your client sends.
- One image request through the same group users would normally call.
- A `/v1/models` allow-list check for the evaluation token.
- A usage/report excerpt for the test window.
- A safe license-status summary when evaluating an enterprise or on-prem licensed deployment.
- A security summary covering provider-key handling, diagnostics redaction, metrics-admin isolation, and private-upstream network controls.
- A retention and rollup summary explaining raw operational rows, finalized daily rollups, legal holds, archived exports, and dry-run-only purge status.
- A rollback plan for disabling a target or reducing its weight.

## Outcome-Based Model Validation

Model routing should be evaluated by task success, not only by model reputation or token price. Harbor is one useful coding-agent harness, but any verifier that reflects real work can be used:

- unit tests for coding tasks;
- extraction accuracy checks;
- OCR target answers;
- tool-call correctness assertions;
- browser-control tasks;
- internal golden datasets;
- product acceptance tests.

The target state is a model group that preserves the required task outcome while improving cost, latency, reliability, or provider optionality.

For router-versus-fixed-model comparisons, pre-register the task set, control, metrics, seed policy, config version or safe routing/config summary, and rollout decision rule using the [Prove Router Quality](./prove-router-quality) template.

## Common Evaluation Failures

| Symptom | Likely meaning | Next step |
|---|---|---|
| `401` or `403` before any model call | Token or endpoint mismatch | Verify base URL and router token. |
| Expected group missing from `/v1/models` | Caller token is not allowed to use it | Ask the administrator to update access if intended. |
| `no-eligible-target` for tools or images | Group exists, but no target satisfies that request shape | Use a compatible group or add a validated target. |
| Timeout on a long agent task | Upstream or per-attempt timeout is too small for the workload | Inspect request attempts and adjust target mix or timeout. |
| A cheaper group fails the verifier | The group does not satisfy that workload contract | Keep it for simpler tasks or promote a stronger target for that workload. |

See [Error Reference](../reference/errors) for structured error types.
