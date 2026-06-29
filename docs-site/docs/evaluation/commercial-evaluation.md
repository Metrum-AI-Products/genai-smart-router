---
title: Commercial Evaluation Path
doc_type: explanation
---

# Commercial Evaluation Path

GenAI Smart Router is evaluated as a governed enterprise gateway, not as a public shared model API-credit product. Metrum provides an evaluation path that lets buyers validate their own workloads, security requirements, reporting needs, and deployment model before choosing commercial access.

<div class="contactBanner">
  <p>To request an evaluation, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Deployment Paths

| Path | Best fit | What the evaluator receives |
|---|---|---|
| Metrum-managed hosted service | Teams that want the fastest evaluation with a managed endpoint | A router base URL, one or more router-issued caller tokens, allowed deployment-defined model groups, and administrator-provided report excerpts for the evaluation window. |
| Enterprise or on-prem deployment | Teams that need the router inside their own infrastructure | A licensed deployment package, signed JSON license file, sample config, operator docs, and support for connecting approved provider keys or private upstreams. |
| Private customer-cloud deployment | Teams that want cloud isolation under their own account or network controls | A deployment package and implementation plan for customer-owned cloud infrastructure, private networking, identity policy, usage database, reporting, and provider onboarding. |

Self-service portal checkout and license download are planned only for approved constrained evaluation, pilot, renewal, or top-up packages if the licensing portal and Stripe fulfillment work ships. They remain planned, not shipped router behavior, and they are not an arbitrary entitlement configurator or public API-credit wallet. Current evaluations should use the contact path above so Metrum and the customer can agree on the deployment shape, license template, provider access, data handling, workload proof points, and reporting package. Production enterprise access uses a **signed license file with capability + time + volume entitlements** combined with an annual, private managed, marketplace/private-offer, or volume-prepurchase commercial path.

## Buyer Journey

1. Request an evaluation and describe the intended clients, workloads, security requirements, provider preferences, and reporting goals.
2. Receive either a Metrum-managed endpoint and router token or a deployment package with a signed license for customer-controlled infrastructure.
3. Use [`/v1/models`](../getting-started/available-models) to discover the deployment-defined model groups the evaluation token can request.
4. Validate workloads through the same API shape the production client will use, such as OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, Codex CLI, Claude Code, or an SDK.
5. Inspect proof from [Admin Browser Reports](../operations/admin-browser-reports), [Report Examples](../operations/report-examples), and generated usage reports: selected providers/models, cost, savings, latency, fallback, cache, quota, and security access signals.
6. Review security posture, deployment readiness, data retention, provider onboarding evidence, rollback criteria, and any required procurement controls.
7. Choose commercial access: annual enterprise self-hosted, private managed, marketplace/private-offer, renewal, top-up, or other contracted terms as agreed in the commercial plan.

## Evaluation And License Options

Commercial evaluations use a license or managed endpoint sized for the proof:

| Option | Typical use | What changes at production time |
|---|---|---|
| `eval-72h` | Short hosted or partner proof | Replace with a pilot, enterprise annual, private managed, or marketplace agreement. |
| `pilot-30d` | Paid validation of workloads, reporting, private upstreams, and governance | Replace with an annual or private managed license after acceptance criteria pass. |
| `enterprise-annual` | Customer-operated production deployment | Renew or amend the license as feature, volume, retention, or deployment scope changes. |
| `credit-pack-5m` / `credit-pack-25m` | Prepaid volume or top-up | Replace with a new issued license when the volume envelope is exhausted or expires. |
| `marketplace-seat` | Procurement through a private cloud marketplace offer | Renew or modify through the marketplace private-offer process. |

The license controls product capabilities and deployment limits; provider keys, upstream choices, model groups, and caller access remain deployment-specific. Use `/v1/models` to see the model groups allowed for the evaluation token. For the full commercial access map, see [Choose a Deployment Path](../licensing/deployment-paths).

## Evaluation Evidence To Request

- Caller-facing compatibility: chat, agent, tool-call, image/VLM, streaming, structured-output, and max-token cap behavior for the client shapes that matter.
- Access control: user, project, membership, API-key, `/v1/models`, quota, rate-limit, and key-rotation examples.
- Cost governance: stored request-time actual cost, source-dated baseline assumptions, savings by user/project/key/group/provider-model, and caveats for historical rows that predate cost fields.
- Performance: downstream user latency and throughput plus upstream provider/model/dialect latency, TTFB, throughput, attempts, errors, and fallbacks.
- Trust and security: provider-key isolation, metrics-admin isolation, report-admin authorization, diagnostics redaction, security access reporting, private-upstream network controls, and signed-license status.
- Retention posture: raw operational rows, daily rollups, dry-run retention status, legal holds, archived exports, and which purge workflows are implemented or future.
- Provider onboarding: direct upstream smokes, router-level smokes, workload acceptance tests, source-dated pricing, tool/modality metadata, and rollback criteria.

## What Not To Expect In Public Docs

Public docs intentionally avoid private deployment hostnames, SSH paths, router tokens, token hashes, provider keys, raw prompts, raw images, raw tool outputs, customer-specific license payloads, and internal production procedures. Evaluation artifacts should use request IDs, public token IDs, anonymized report excerpts, and safe scalar metadata.

For the technical checklist, continue with [Evaluate GenAI Smart Router](./evaluate-smart-router). For deployment proof, review [Deployment Readiness](./deployment-readiness), [Deployment Security Assessment](./deployment-security-assessment), and [Model Group Quality Criteria](./model-group-quality).
