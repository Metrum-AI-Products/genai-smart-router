---
title: Enterprise FAQ
doc_type: explanation
---

# Enterprise FAQ

Use this guide to answer the first enterprise evaluation questions quickly. GenAI Smart Router helps platform teams expose approved model groups, keep provider keys server-side, route compatible requests across validated upstreams, enforce budgets and rate limits, and report what happened after each request.

The main evaluation path is:

1. Pick one real workload and one fixed-model baseline.
2. Expose one caller-visible model group for that workload.
3. Validate the exact client shape: Chat, Responses, Anthropic Messages, tools, images, reasoning, or large context.
4. Compare outcome, cost, latency, fallback, and selected provider/model in reports.
5. Promote, keep fixed, or roll back using the published decision rule.

For a first proof plan, start with [Evaluate GenAI Smart Router](./evaluate-smart-router), [Prove Router Quality](./prove-router-quality), and [Enterprise Deployment Patterns](../operations/deployment-patterns).

## Quick Answers

| Question | Short answer | Deeper page |
|---|---|---|
| Will routing make a working workload worse? | Keep a fixed-model group and promote a routed group only after the same workload passes outcome checks. | [Prove Router Quality](./prove-router-quality) |
| Can one provider limit cap all traffic? | Compatible targets can share traffic across providers, accounts, models, or private endpoints while caller and upstream shaping enforce limits. | [Provider Traffic Shaping](../configuration/provider-traffic-shaping) |
| Can teams keep different policies? | Yes. Use caller tokens, model-group allow lists, dedicated groups, separate routers, or hierarchical routers. | [Enterprise Deployment Patterns](../operations/deployment-patterns) |
| Are provider keys exposed to clients? | No. Clients receive router tokens and model-group names; provider credentials stay server-side. | [Security And Trust](./security-and-trust) |
| Does the router store prompts by default? | Ordinary usage and diagnostics are metadata-first. Governed content capture is separate and explicitly enabled. | [Usage Reporting](../operations/usage-reporting) |
| What is the smallest useful proof? | Run `/v1/models`, one chat smoke, one agent/tool or image smoke, one report excerpt, and one rollback or `no-eligible-target` check. | [Evaluate GenAI Smart Router](./evaluate-smart-router) |

## What if the router makes my model worse?

Treat each model group as a quality and cost contract, not as a promise that every workload improves automatically. A deployment can keep a fixed-model group, compare it against a routed group, and promote only when the routed group preserves the required outcome.

How GenAI Smart Router handles it: Model groups define intended workloads, API shapes, modalities, tools, cost targets, latency targets, validation harnesses, promotion criteria, and rollback criteria. Request-shape filtering prevents a text-only or non-tool target from serving a tool or image request. Fallback and usage telemetry show which provider/model served each request and whether another attempt was needed.

What you own: The fixed-model baseline, routed candidate group, success metric, allowed targets, weights or policy, evaluation window, and rollback rule.

Proof to request or run: Run the same workload through the fixed baseline and routed group, compare task success, cost, latency, token volume, fallback, and selected provider/model, then decide using the published decision rule.

Links: [Prove Router Quality](./prove-router-quality), [Model Group Quality Criteria](./model-group-quality), [Harbor Case Study](./harbor-case-study).

## What happens when an upstream provider rate-limits us?

Caller limits protect individual keys, and provider/model/target traffic shaping can protect shared upstream capacity across all keys. When an upstream returns `429` or a quota/billing signal, adaptive backoff can temporarily remove that affected target from eligibility while other validated targets continue serving traffic.

How GenAI Smart Router handles it: The router checks caller policy first, then request-shape eligibility, then shared upstream shaping. Weighted routing recalculates over targets that are not currently throttled. If every otherwise eligible target is locally throttled, callers receive `503 upstream-capacity-throttled` with a request ID and `Retry-After` when calculable. Provider quota or billing exhaustion remains a separate class from provider rate limiting.

What you own: Provider account capacity, shaping limits, model-group fallback mix, adaptive backoff windows, and the rollout/rollback policy for changing those values.

Proof to request or run: Configure a low traffic-shape limit on a smoke provider, send parallel requests from two caller tokens, verify one request routes around the throttled target or receives `upstream-capacity-throttled`, then inspect `request_upstream_shape_events` and `request_attempts` for safe scalar evidence.

Links: [Router Configuration](../configuration/router-config), [Routing Overview](../routing/overview), [Errors](../reference/errors), [Usage Reporting](../operations/usage-reporting).

## Can we combine provider rate limits instead of being capped by one vendor?

Yes, when the targets are validated for the same request shape and the deployment config intentionally puts them behind one model group. The router can distribute compatible traffic across providers, accounts, models, or private endpoints with separate upstream limits.

How GenAI Smart Router handles it: Caller limits and quotas run first. Request-shape eligibility then filters to targets that can satisfy the incoming API surface, tools, images, reasoning, context, and output-cap behavior. Routing strategy chooses among the remaining targets, and provider traffic shaping/adaptive backoff protects shared upstream accounts.

What you own: Provider account limits, target validation, group weights or policy, upstream traffic-shape settings, fallback policy, and the definition of equivalent-enough quality for the workload.

Proof to request or run: Use a smoke group with at least two compatible providers, run parallel traffic, verify the provider/model mix in reports, then lower one provider's shaping limit and confirm compatible traffic shifts or falls back without crossing caller quotas.

Links: [Model Groups](../configuration/model-groups), [Provider Traffic Shaping](../configuration/provider-traffic-shaping), [Usage Reporting](../operations/usage-reporting).

## How do we prevent one app, user, or coding agent from hurting everyone else?

Use caller tokens as the first fairness boundary, then provider shaping as the shared-capacity boundary. This separates user/team policy from upstream account protection.

How GenAI Smart Router handles it: Per-caller RPM, TPM, concurrency, daily/monthly quotas, lifetime budgets, and optional caller traffic shaping run before upstream selection. Provider/model/target shaping and adaptive backoff protect shared provider accounts after a target is selected. Reports show which caller, client, group, provider, and shaping bucket was affected.

What you own: Caller tiering, project budgets, developer-agent limits, provider account limits, and escalation rules for trusted production workloads.

Proof to request or run: Run a controlled burst from one caller and a normal request from another caller. Verify the burst is queued/rejected or shaped without consuming every upstream target, then inspect Traffic Shaping, Quotas, User Impact, and Provider Capacity reports.

Links: [Caller Traffic Shaping](../configuration/caller-traffic-shaping), [Provider Traffic Shaping](../configuration/provider-traffic-shaping), [Admin Browser Reports](../operations/admin-browser-reports).

## Can we force some workloads to a specific model?

Yes. A deployment can expose a model group with one static target, a failover list, or a policy that selects only within the requested group. The router does not silently route a caller into a group the caller was not allowed to request.

How GenAI Smart Router handles it: Static groups, failover groups, weighted groups, dynamic-score routing, TypeScript policy, and external policy all operate inside the caller-requested model group after token authorization. Eligibility filtering then removes targets that cannot satisfy the request shape.

What you own: The group name, caller allow list, target list, routing strategy, policy script or external service, target validation metadata, and rollback procedure.

Proof to request or run: Create one group with a single approved target, call `/v1/models` with the caller token, run a chat or tool smoke, and verify the usage report shows only that target for the group.

Links: [Customer-Controlled Routing](../routing/customer-controlled-routing), [Routing Overview](../routing/overview), [Router Configuration](../configuration/router-config).

## Can different teams own different routing strategies?

Yes. Teams can use different caller tokens, users, projects, model-group allow lists, dedicated groups, separate router instances, or hierarchical routers. The right pattern depends on the governance boundary and whether teams need independent provider keys or databases.

How GenAI Smart Router handles it: Caller tokens identify users/projects and restrict visible model groups. Deployment patterns include central enterprise gateways, per-environment routers, per-team routers, hierarchical routers, and private managed routers.

What you own: Team ownership boundaries, group contracts, token policies, provider-key custody, usage database boundaries, and report access.

Proof to request or run: Issue two caller tokens with different group allow lists, call `/v1/models` with each token, and run a request that proves each team sees only its approved groups.

Links: [Enterprise Deployment Patterns](../operations/deployment-patterns), [Available Models And Access](../getting-started/available-models), [Admin Authorization](../configuration/admin-authorization).

## Can central platform governance coexist with team autonomy?

Yes. Central governance can own provider credential custody, license enforcement, retention, metrics isolation, and report authorization while teams own model-group contracts and workload validation. Hierarchical deployments can separate central egress control from team-local strategy.

How GenAI Smart Router handles it: A central router can enforce caller access, quotas, admin authorization, metrics restrictions, and provider-key isolation. Team-local routers can call an enterprise router or private upstreams when separate usage stores or policy release cycles are needed.

What you own: Which controls are central, which are team-local, where usage data lives, and who can update each routing policy.

Proof to request or run: Demonstrate a central policy rule, a team-specific model group, a metrics-admin isolation check, and a report filtered by team/project.

Links: [Enterprise Deployment Patterns](../operations/deployment-patterns), [Security And Trust](./security-and-trust), [Usage Reporting](../operations/usage-reporting).

## Can we keep our provider keys and private models?

Yes. Provider keys are configured server-side, and clients receive only router tokens and model-group names. Private OpenAI-compatible services can be added as upstream providers when they pass direct and router-level smokes.

How GenAI Smart Router handles it: Providers are configured with server-side credentials, base URLs, dialects, model catalogs, pricing metadata, modalities, and tool-support metadata. Self-hosted vLLM, SGLang, Baseten-style, and other OpenAI-compatible endpoints can remain on private networks behind the router.

What you own: Provider accounts, private upstream endpoints, network controls, model IDs, served templates/parsers, validation evidence, and rollout criteria.

Proof to request or run: Call the private upstream directly from the deployment network, call the same model through a router group, and verify clients never receive provider keys or private upstream URLs.

Links: [Self-Hosted Upstreams](../configuration/self-hosted-upstreams), [Providers And Models](../providers-models/overview), [Installation](../installation/).

## Will this work with Codex, Claude Code, Cursor, Warp, or our OpenAI SDK?

The router supports OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, and `/v1/models` discovery. Client compatibility still needs to be validated with the exact API shape, tool behavior, images, token caps, and model groups that the deployment exposes.

How GenAI Smart Router handles it: The router preserves caller-facing API dialects while selecting an eligible upstream target. Codex-style Responses traffic, Claude Code-style Anthropic Messages traffic, OpenAI-compatible SDKs, tool calls, and image-bearing requests can be routed when the group contains validated compatible targets.

What you own: The client list, allowed model groups, API shapes, tool dialects, image requirements, and smoke-test matrix.

Proof to request or run: Call `/v1/models`, run one OpenAI Chat smoke, one Responses or Codex smoke, one Anthropic Messages or Claude Code smoke, and one tool or image request for the target group.

Links: [API Compatibility](../reference/api-compatibility), [Codex CLI](../getting-started/codex-cli), [Claude Code CLI](../getting-started/claude-code-cli).

## What about tool calls, images, structured outputs, or reasoning/thinking?

Yes, when target metadata and validation are maintained correctly. Requests that require tools, images, structured outputs, or reasoning/thinking controls are filtered to compatible targets or fail before an unsafe upstream call.

How GenAI Smart Router handles it: Provider catalogs and target overrides record modalities, dialect support, tool support, structured-output behavior, reasoning controls, and max-token-cap behavior. Eligibility filtering runs before the routing policy selects a target.

What you own: The exact capability claims, direct upstream smoke results, router-level smoke results, and documentation of any target-specific limitations.

Proof to request or run: Run one passing request for each advertised capability and one negative test where a group without a compatible target returns `no-eligible-target`.

Links: [Agents, Tools, And Vision](../agents-tools-vision/overview), [Structured Outputs](../agents-tools-vision/structured-outputs), [Reasoning Routing](../configuration/reasoning-routing).

## What happens if a provider is slow, down, rate-limited, or removed?

Use failover, weighted routing, target removal, and rollback plans that are tied to the model-group contract. The router can try another eligible target when the failure class is retryable and another target exists.

How GenAI Smart Router handles it: Attempts, upstream errors, fallback use, latency, throughput, timeout, and terminal status are recorded for triage. Non-retryable malformed or policy errors stop fallback so the same bad request is not replayed unnecessarily.

What you own: Retry/fallback policy, target list, timeout settings, provider account health, rollback criteria, and monitoring thresholds.

Proof to request or run: Temporarily remove a target or use a staging group with a failing first target, then verify fallback attempts and terminal status appear in the usage/report data.

Links: [Observability](../operations/observability), [Usage Reporting](../operations/usage-reporting), [Troubleshooting](../troubleshooting/).

## How do we control cost without degrading quality?

Cost control should be tied to outcome validation. Use group contracts to define the required result, then tune provider mix, cache behavior, and access policies only when the workload still passes.

How GenAI Smart Router handles it: The router records request-time cost inputs and selected provider/model, supports budget/rate-limit policies, can cache safe deterministic workloads, and can compare routed groups with fixed baselines using reports.

What you own: Baseline model, success threshold, cheaper candidate targets, cache eligibility, token budgets, and promotion criteria.

Proof to request or run: Compare fixed baseline and routed group over the same evaluation set, including pass rate, cost/request, latency, fallback, and token volume.

Links: [Cost Governance](./cost-governance), [Prove Router Quality](./prove-router-quality), [Report Examples](../operations/report-examples).

## How do we prove savings are real?

Savings should be calculated from stored request-time cost data, not recalculated later from whatever the config says today. Reports should show token counts, request-time prices, upstream-reported billed costs when available, and the chosen baseline.

How GenAI Smart Router handles it: Usage rows store scalar cost fields, token counts, provider/model, request status, latency, cache state, and upstream-reported billed values when the upstream supplies them. Reports can group by caller, project, group, provider, model, status, and time window.

What you own: Baseline price assumption, report window, grouping dimensions, and whether a route qualifies as successful enough to count as savings.

Proof to request or run: Request a report for the evaluation window showing actual routed spend, baseline spend, savings, tokens, selected targets, and failed or fallback attempts.

Links: [Usage Reporting](../operations/usage-reporting), [Usage, Cost, And Reports](../usage-cost-reports/overview), [Report Examples](../operations/report-examples).

## How do we handle rate limits and coding-agent bursts?

Rate limits, budgets, and traffic shaping should be assigned to caller tokens and model groups based on expected workload. Large-context developer tools need TPM, RPM, concurrency, output-cap, and burst-smoothing policies that match their real request shape.

How GenAI Smart Router handles it: Caller policies can enforce request, token, concurrency, daily, monthly, or lifetime limits before provider calls. Optional traffic shaping smooths request starts, estimated input-token throughput, output reservations, and total reserved-token throughput before upstream calls. Usage and attempt data help distinguish hard quota failures, `429 traffic-shaped` burst smoothing, and upstream provider quota or billing exhaustion.

What you own: Caller tiering, developer-group access, burst limits, queue or reject behavior, provider account capacity, and escalation procedure for production-critical agents.

Proof to request or run: Run a controlled burst test with a known token cap, verify expected `429 traffic-shaped` or hard-limit behavior, and inspect usage reports for caller, project, model group, traffic-shaping bucket, queue wait, retry-after, and upstream attempts.

Links: [Troubleshooting Requests](../troubleshooting/requests), [Usage Reporting](../operations/usage-reporting), [Available Models And Access](../getting-started/available-models).

## Do you store prompts, responses, images, or tool outputs?

The default operating model is metadata-first usage and diagnostics, not raw prompt or image capture. Governed content capture should be explicitly enabled, scoped, retained, and reviewed before production use.

How GenAI Smart Router handles it: Usage data records safe scalar metadata such as request IDs, caller/project fields, model group, provider/model, status, token counts, latency, cost, attempts, cache behavior, and sanitized errors. PII filtering can redact configured text before routing and upstream calls.

What you own: Retention policy, content-capture policy, PII rules, diagnostics access, report authorization, and legal hold/export process.

Proof to request or run: Review the usage schema and an evaluation report excerpt, then confirm no raw prompts, raw images, tool outputs, bearer tokens, token hashes, or provider keys are present in ordinary diagnostics.

Links: [Security And Trust](./security-and-trust), [PII Filtering](../configuration/pii-filtering), [Usage Reporting](../operations/usage-reporting).

## Can we meet data residency, VPC, or on-prem requirements?

GenAI Smart Router can be deployed as enterprise self-hosted, private managed, or private customer-cloud infrastructure. Provider choices, private upstreams, usage stores, retention, and network controls are deployment decisions.

How GenAI Smart Router handles it: Release packages can run in customer infrastructure with signed license enforcement. Private upstreams and regional routers can keep traffic inside approved network boundaries when the deployment is designed that way.

What you own: Deployment region, network topology, provider contracts, private upstream placement, usage database, retention, backups, and access controls.

Proof to request or run: Review the deployment topology, run readiness checks inside the target environment, and verify provider egress paths and retention settings.

Links: [Installation](../installation/), [Enterprise Deployment Patterns](../operations/deployment-patterns), [Deployment Readiness](./deployment-readiness).

## How do we roll this out safely?

Start with an evaluation endpoint or staging router, then use allow-list expansion, canary groups, monitoring, and explicit rollback criteria. Promote model groups by evidence, not by assumption.

How GenAI Smart Router handles it: The router exposes caller-token allow lists, group-specific routing policy, reports, attempts, fallback telemetry, health endpoints, license status, and version metadata. Deployments can keep fixed-model groups while routed candidates mature.

What you own: Evaluation scope, staging environment, canary audience, acceptance checklist, rollback steps, and production change window.

Proof to request or run: Execute the 30-minute evaluation path, then a one-day workload evaluation with report excerpts and rollback demonstration.

Links: [Evaluate GenAI Smart Router](./evaluate-smart-router), [Deployment Readiness](./deployment-readiness), [Operational Acceptance](./operational-acceptance).

## What if we need auditability?

Yes, within the metadata captured by the deployment. The router records request IDs, caller/project metadata, model group, selected target, attempts, errors, cost fields, latency, and throughput without requiring raw content capture.

How GenAI Smart Router handles it: The usage database and reports separate request-level rows, upstream attempts, errors, trace events, quotas, and report/admin access. Metrics remain restricted to authorized metrics-admin subjects.

What you own: User/project naming, access-control policy, report-admin authorization, retention period, and audit export procedure.

Proof to request or run: Pick a request ID from a smoke test and ask the operator to show the sanitized usage, attempt, and error records plus the report grouping that includes it.

Links: [Usage Reporting](../operations/usage-reporting), [Admin Browser Reports](../operations/admin-browser-reports), [Security And Governance](../security-governance/overview).

## Can we use multiple router instances?

Yes. A single central router is simplest, but per-team, per-environment, regional, hierarchical, or private managed routers can be a better fit when ownership boundaries differ.

How GenAI Smart Router handles it: Each router instance can have its own config, license, provider credentials, usage database, model groups, caller tokens, and reports. A hierarchical design can place a team router in front of an enterprise egress router when that boundary is intentional.

What you own: Instance boundaries, shared versus separate usage data, provider-key custody, policy update process, and support model.

Proof to request or run: Compare a central-router proof with a team-router or hierarchical proof, then verify `/v1/models`, reports, and provider-key custody match the chosen boundary.

Links: [Enterprise Deployment Patterns](../operations/deployment-patterns), [Choose a Deployment Path](../licensing/deployment-paths), [Installation](../installation/).

## What is the smallest proof we can run this week?

A useful proof can be small if it tests the real client shape and produces operator evidence. Do not stop at a single chat response if the production workload depends on tools, images, coding agents, cost reports, or rollback.

How GenAI Smart Router handles it: The router can expose an evaluation endpoint or customer deployment package, then capture caller-visible results and operator evidence for the same test window.

What you own: The representative task, success rule, evaluation token, target group, baseline, and proof artifacts to archive.

Proof to request or run: In 30 minutes, call `/v1/models`, run one chat smoke, run one agent or tool smoke, request one report excerpt, and demonstrate one rollback or no-eligible-target behavior. In one day, add a representative workload set, a fixed-model baseline, Harbor or another verifier, a cost/latency comparison, and a deployment-readiness checklist.

Links: [Evaluate GenAI Smart Router](./evaluate-smart-router), [Prove Router Quality](./prove-router-quality), [Deployment Readiness](./deployment-readiness).
