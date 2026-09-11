# Customer Scenario Playbook

Use this internal playbook when a buyer, evaluator, support lead, or customer team asks whether Metrum Smart Router will fit their deployment. Keep responses concrete, proof-oriented, and deployment-defined. Do not promise that routing improves every workload; prove the workload outcome and show the operator evidence.

Public counterpart: `docs-site/docs/evaluation/enterprise-faq.md`.

## Safe Response Pattern

1. Restate the concern in the customer's words.
2. Explain the product mechanism in terms of model groups, caller access, request-shape eligibility, provider custody, reports, or deployment pattern.
3. State what the customer owns: routing strategy, provider keys, private upstreams, validation gates, access policy, retention, and rollout criteria.
4. Ask for or propose a proof artifact.
5. Link to public docs instead of copying private deployment operations into email or tickets.

## Scenario Templates

| Question | Safe response | Proof artifact | Offer when appropriate |
|---|---|---|---|
| What if routing makes a workload worse? | A routed group is promoted only after it preserves the required outcome against a fixed-model baseline. Keep a fixed group until the routed candidate passes. | Baseline-versus-routed evaluation with pass rate, cost, latency, token volume, fallback, and selected target. | Evaluation endpoint, private managed pilot, or self-hosted package. |
| Can we force a workload to one model? | Yes. Use a static single-target group or a failover group scoped to approved targets. Routing remains inside the caller-requested group. | `/v1/models` allow-list check plus usage report proving selected target. | Configuration review. |
| Can teams own different strategies? | Yes. Use per-team groups, caller tokens, projects, separate routers, or hierarchical routers depending on ownership boundaries. | Two-token `/v1/models` check and per-team report grouping. | Deployment-pattern design session. |
| Can central governance coexist with team autonomy? | Central governance can own credentials, license, retention, admin access, and metrics while teams own group contracts and validation. | Central metrics/admin isolation check plus team-local model-group smoke. | Enterprise self-hosted or hierarchical design. |
| Can we keep provider keys and private models? | Yes. Provider keys and private upstream URLs stay server-side; clients get router tokens and model groups. | Direct private-upstream smoke plus router-level smoke with no key or private URL exposed to callers. | Self-hosted or private managed deployment. |
| Will existing clients work? | Validate the exact API shape: OpenAI Chat, OpenAI Responses, Anthropic Messages, tools, images, and output caps. | One smoke per required client or API shape. | Evaluation endpoint. |
| What about tools, images, structured outputs, and reasoning? | Capability metadata and eligibility filters should send those requests only to validated compatible targets or fail before upstream. | Positive capability smoke plus negative `no-eligible-target` test. | Capability validation matrix. |
| What if a provider is slow or exhausted? | Retryable upstream failures can fall back to another eligible target; terminal reports show attempts and fallback. | Staging fallback demonstration and request-attempt report. | Operational readiness review. |
| How do we control cost safely? | Cost changes must be evaluated against quality. Promote cheaper targets only when the workload verifier still passes. | Cost/latency/pass-rate comparison over the same workload set. | Evaluation or cost-governance review. |
| How do we prove savings? | Use stored request-time costs, token counts, upstream-billed cost fields when present, and an explicit baseline. | Report excerpt with actual spend, baseline spend, savings, tokens, and status. | Reporting walkthrough. |
| How do we handle bursts? | Set caller/project RPM, TPM, concurrency, traffic shaping, and budget limits that match the client workload. | Controlled burst test plus quota/report review. | Caller policy review. |
| Do you store content? | Default diagnostics are metadata-first. Governed content capture must be explicitly enabled and scoped. | Usage schema excerpt and sanitized report sample. | Security review. |
| Can we meet residency or VPC requirements? | Deployment shape controls region, network path, provider choice, private upstreams, and usage retention. | Topology diagram, readiness checks, and egress/retention review. | Self-hosted, private cloud, or private managed path. |
| How do we roll out safely? | Start with evaluation or staging, use allow-list expansion and canary groups, then promote after proof. | 30-minute proof, one-day workload proof, and rollback demonstration. | Evaluation or pilot. |
| What if we need auditability? | Usage/reporting can show request IDs, caller/project, selected target, status, attempts, latency, tokens, and cost. | Request-ID trace and report grouping. | Reporting and access-control review. |
| Can we use multiple routers? | Yes. Central, per-team, per-environment, regional, hierarchical, and private managed patterns are available. | Comparison of `/v1/models`, reports, and credential custody for each option. | Deployment-pattern design session. |
| What is the smallest proof this week? | Run `/v1/models`, one chat smoke, one agent/tool smoke, one report excerpt, and one rollback or negative-eligibility test. | 30-minute proof package plus one-day representative workload if needed. | Evaluation endpoint or package. |

## Which Path To Propose

| Customer situation | Suggested path |
|---|---|
| Wants quick client compatibility and report proof | Metrum-managed evaluation endpoint. |
| Needs customer infrastructure, private networking, or air-gapped operation | Enterprise self-hosted package with signed license. |
| Wants dedicated customer environment operated for them | Private managed deployment. |
| Has multiple autonomous platform or app teams | Per-team or hierarchical router design. |
| Needs deployment-specific legal or purchasing terms | Handle outside the OSS repository; do not imply an unshipped workflow. |

## What To Avoid Saying Publicly

- Do not claim routing improves every workload.
- Do not claim unvalidated provider, model, tool, image, structured-output, or reasoning support.
- Do not publish exact competitor pricing unless it was freshly revalidated for that task and sourced.
- Do not disclose private hostnames, SSH paths, router tokens, provider keys, token hashes, raw production config, or customer content.
- Do not describe internal build workflow details in customer-facing docs unless the page is explicitly about an open-source third-party product.

## Public Links To Use

- Public FAQ: `/docs/evaluation/enterprise-faq`
- Evaluation checklist: `/docs/evaluation/evaluate-smart-router`
- Proof plan: `/docs/evaluation/prove-router-quality`
- Deployment patterns: `/docs/operations/deployment-patterns`
- Customer-controlled routing: `/docs/routing/customer-controlled-routing`
- Usage reporting: `/docs/operations/usage-reporting`
- Security and trust: `/docs/evaluation/security-and-trust`
