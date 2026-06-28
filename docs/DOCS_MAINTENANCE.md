# Documentation Maintenance Runbook

This runbook is the internal source of truth for keeping operator docs, packaged docs, and hosted Docusaurus product docs aligned with shipped behavior. It is not part of the public Docusaurus site and should not be added to `docs-site/sidebars.js`.

## Documentation Surfaces

| Surface | Location | Audience | Private details allowed? | Release/package status |
|---|---|---|---|---|
| Root README | `README.md` | Developers and operators working from source | No raw secrets; avoid private host details except clearly internal pointers | Included in binary and Docker packages |
| Internal operator docs | `docs/*.md` | Metrum operators, implementation reviewers, deployment owners | Some internal process detail is acceptable; never include raw provider keys, raw router tokens, token hashes, real private keys, real customer payloads, or full production config | Only files in `scripts/package_docs_allowlist.txt` are packaged |
| Public hosted docs | `docs-site/docs/**` | Customers, evaluators, application developers, platform admins | No private hostnames, SSH usernames, key paths, raw secrets, token hashes, full config, internal source-control workflow, or private deployment procedures | Built into the router binary and served under `/docs/` |
| Deployment notes | `deployment.md`, selected `docs/DEPLOYMENT.md` sections | Internal deployment operators and packaged deployment readers | Keep live host-specific operations in internal notes only; packaged docs must remain generic | `deployment.md` is not packaged; `docs/DEPLOYMENT.md` is packaged |
| Config examples | `config.example.yaml`, `env.example.json`, docs snippets | Operators and evaluators | Placeholders only | `config.example.yaml` and `env.example.json` are packaged |

## Source-Of-Truth Map

Use this map when public Docusaurus content changes. Public pages should explain caller-visible product behavior and link to safe customer actions. Internal docs and runbooks should hold rollout, validation, troubleshooting, rollback, and security-review detail.

| Public Docusaurus section | Public files | Internal/operator source of truth | Keep aligned when changing |
|---|---|---|---|
| Product overview and solution positioning | `docs-site/docs/overview.mdx`, `docs-site/docs/solution-brief.md` | `README.md`, `docs/solution-brief.md`, `docs/PRODUCT_CAPABILITY_MATRIX.md` | Feature set, deployment models, commercial evaluation language, hosted-doc privacy boundaries |
| Getting started and model access | `docs-site/docs/getting-started/*.mdx` | `README.md`, `docs/SMOKE_TEST_MATRIX.md`, `config.example.yaml` | `/v1/models`, caller token access, CLI base URLs, token placeholders, tested client smoke paths |
| Installation | `docs-site/docs/installation/*.md`, `docs-site/docs/operations/deployment.md` | `README.md`, `docs/DEPLOYMENT.md`, `docs/DOCKER_DEPLOYMENT.md`, package scripts, release validation | Package contents, Docker Compose and binary install paths, runtime paths, health/readiness checks, generic upgrade and rollback guidance |
| Licensing | `docs-site/docs/licensing/*.md`, `docs-site/docs/operations/license-protected-deployments.md` | `docs/LICENSE_OPERATIONS.md`, license code/tests, release packaging checks, `docs/SECURITY_REVIEW_NOTES.md` | Customer install/renewal/status behavior publicly; signing, issuance, private keys, and development bypass operations internally |
| Router configuration | `docs-site/docs/configuration/router-config.md` | `config.example.yaml`, `README.md`, feature-specific runbooks in `docs/` | YAML fields, defaults, provider catalog vs model groups vs caller keys, validation behavior, rollout and rollback notes |
| Routing | `docs-site/docs/routing/overview.md`, `docs-site/docs/configuration/model-group-contracts.md`, `docs-site/docs/configuration/reasoning-routing.md`, `docs-site/docs/configuration/dynamic-score-routing.md`, `docs-site/docs/configuration/routing-typescript.md`, `docs-site/docs/configuration/external-routing-policy.md` | `docs/MODEL_GROUP_CONTRACTS.md`, `docs/DYNAMIC_SCORE_ROUTING.md`, `docs/EXTERNAL_ROUTING_POLICY.md`, `README.md`, `config.example.yaml`, routing tests | Model-group contracts, target eligibility, request-shape filtering, fallback behavior, dynamic score, TypeScript policy, external policy, reasoning controls, smoke and rollback criteria |
| TypeScript routing | `docs-site/docs/configuration/routing-typescript.md` | `README.md`, `config.example.yaml`, script examples and tests | Script context shape, safe egress, caller-visible errors, tested examples |
| External routing policy | `docs-site/docs/configuration/external-routing-policy.md` | `docs/EXTERNAL_ROUTING_POLICY.md`, `config.example.yaml`, policy examples and tests | Request/response schema, trusted-infrastructure boundary, failure behavior, safe metadata |
| Providers and models | `docs-site/docs/providers-models/overview.md`, `docs-site/docs/reference/model-metadata.md`, `docs-site/docs/reference/add-provider-model.md`, `docs-site/docs/configuration/self-hosted-upstreams.md` | `config.example.yaml`, `env.example.json`, `docs/SMOKE_TEST_MATRIX.md`, `docs/SELF_HOSTED_UPSTREAMS.md`, provider smoke notes | Catalog-only vs smoke group vs active target, pricing source/date, modalities, tool and structured-output metadata, direct/router smokes, rollout and rollback |
| Agents, tools, and vision | `docs-site/docs/agents-tools-vision/*.md`, `docs-site/docs/getting-started/codex-cli.mdx`, `docs-site/docs/getting-started/claude-code-cli.mdx`, `docs-site/docs/configuration/image-analysis-vlm.md` | `docs/SMOKE_TEST_MATRIX.md`, `docs/harbor-case-study.md`, `config.example.yaml`, client smoke scripts | OpenAI Chat tools, Responses function tools, Anthropic Messages tools, VLM/image input, combined capability validation, coding-agent workflow examples |
| Structured outputs | `docs-site/docs/agents-tools-vision/structured-outputs.md`, `docs-site/docs/reference/api-compatibility.md`, `docs-site/docs/reference/model-metadata.md` | `docs/SMOKE_TEST_MATRIX.md`, provider smoke notes, routing tests | Chat `response_format`, Responses `text.format`, eligibility metadata, combined tool plus structured-output smokes, limitations around validation and repair |
| PII filtering | `docs-site/docs/configuration/pii-filtering.md` | `docs/PII_FILTERING.md`, `config.example.yaml`, redaction tests | Model-group `pii_filter`, redaction timing, safe telemetry, non-persistence of placeholder maps |
| Self-hosted upstreams | `docs-site/docs/configuration/self-hosted-upstreams.md`, `docs-site/docs/providers-models/overview.md` | `docs/SELF_HOSTED_UPSTREAMS.md`, `config.example.yaml`, smoke scripts | vLLM/SGLang-style setup, served model IDs, tool parser validation, private URL handling |
| API compatibility and errors | `docs-site/docs/reference/api-compatibility.md`, `docs-site/docs/reference/errors.md` | `README.md`, router handlers, tests, `docs/SMOKE_TEST_MATRIX.md` | OpenAI/Anthropic compatibility, structured error types, auth, quota, license, no-eligible-target, and upstream error semantics |
| Usage reporting and admin reports | `docs-site/docs/usage-cost-reports/overview.md`, `docs-site/docs/operations/usage-reporting.md`, `docs-site/docs/operations/admin-browser-reports.md`, `docs-site/docs/operations/report-examples.mdx` | `docs/USAGE_REPORTING_PLAYBOOK.md`, `docs/USAGE_DB_DESIGN.md`, `docs/SECURITY_REVIEW_NOTES.md` | Report fields, scalar usage schema, request-time cost storage, latency/throughput views, authorization, redaction, retention, customer-safe examples |
| Security and governance | `docs-site/docs/security-governance/overview.md`, `docs-site/docs/evaluation/security-and-trust.md`, `docs-site/docs/evaluation/deployment-security-assessment.md` | `docs/SECURITY_REVIEW_NOTES.md`, `docs/AUTHORIZATION.md`, `docs/ADMIN_AUTH.md`, `docs/ADMIN_OIDC.md`, `docs/PII_FILTERING.md`, `docs/LICENSE_OPERATIONS.md` | Caller auth, admin auth, Casbin authorization, metrics-admin isolation, report security, PII boundaries, license status visibility, private upstream controls |
| Observability and operations | `docs-site/docs/operations/observability.md`, `docs-site/docs/operations/key-generation.md`, `docs-site/docs/evaluation/deployment-readiness.md`, `docs-site/docs/evaluation/operational-acceptance.md` | `docs/DEPLOYMENT.md`, `docs/DOCKER_DEPLOYMENT.md`, `docs/PRODUCTION_RUNBOOK.md`, `docs/USAGE_REPORTING_PLAYBOOK.md`, `docs/TROUBLESHOOTING_RUNBOOK.md` | Readiness/health/version, metrics, logs, request IDs, deployment validation, backup/rollback, public-safe operational acceptance |
| Troubleshooting | `docs-site/docs/troubleshooting/*.md`, `docs-site/docs/reference/errors.md`, usage/report pages | `docs/TROUBLESHOOTING_RUNBOOK.md`, `docs/USAGE_REPORTING_PLAYBOOK.md`, `docs/LICENSE_OPERATIONS.md`, `docs/SECURITY_REVIEW_NOTES.md` | Customer-safe error triage, request IDs, quota/license/upstream distinctions, slow-request analysis, internal-only escalation details |
| Evaluation overview and proof points | `docs-site/docs/evaluation/evaluate-smart-router.md`, `docs-site/docs/evaluation/commercial-evaluation.md`, `docs-site/docs/evaluation/product-capabilities.md` | `docs/PRODUCT_CAPABILITY_MATRIX.md`, `docs/SMOKE_TEST_MATRIX.md`, `docs/USAGE_REPORTING_PLAYBOOK.md` | Proof-package contents, evaluation access language, tested capabilities |
| Model-group quality and benchmarks | `docs-site/docs/evaluation/model-group-quality.md`, `docs-site/docs/evaluation/harbor-case-study.mdx` | `docs/SMOKE_TEST_MATRIX.md`, `docs/harbor-case-study.md`, usage reports, evaluation scripts | Success criteria, workload verifier, model-group contracts, source-dated benchmark data |
| Deployment readiness and operational acceptance | `docs-site/docs/evaluation/deployment-readiness.md`, `docs-site/docs/evaluation/operational-acceptance.md` | `docs/DEPLOYMENT.md`, `docs/DOCKER_DEPLOYMENT.md`, `deployment.md`, `docs/SMOKE_TEST_MATRIX.md` | Rollout, rollback, smoke tests, package validation, operational cleanup |
| Security and trust | `docs-site/docs/evaluation/security-and-trust.md`, `docs-site/docs/evaluation/deployment-security-assessment.md` | `docs/SECURITY_REVIEW_NOTES.md`, `docs/AUTHORIZATION.md`, `docs/USAGE_REPORTING_PLAYBOOK.md` | Metrics isolation, admin/report/content authorization, diagnostics redaction, dependency/container scan notes |
| Competitive landscape | `docs-site/docs/evaluation/competitive-landscape.md` | Source-dated market notes and current product capability docs | Primary-source citations, dated pricing, customer-value framing, no private deployment facts |
| Release notes and upgrades | `docs-site/docs/release-notes/*.md` | Package/release scripts, `README.md`, `docs/DEPLOYMENT.md`, `docs/DOCKER_DEPLOYMENT.md`, `docs/SECURITY_REVIEW_NOTES.md`, release validation notes | Customer-safe shipped behavior, config/database/license changes, validation checklist, rollback notes, no source-control or private deployment details |

## Behavior-Change Documentation Requirements

Every behavior, config, deployment, model, auth, CLI/API, telemetry, evaluation, or security change needs both sides of the documentation set unless the change is purely internal and not caller/operator visible.

For operator/deployment docs, update the relevant files in `README.md`, `docs/`, `deployment.md`, scripts, and config comments. Cover configuration shape, validation, rollout, smoke tests, rollback, operational impact, and security impact.

For public Docusaurus docs, update `docs-site/docs/**` so customers know what to request, what the proxy does, what errors to expect, and what evidence to ask operators for. Keep examples generic with placeholder endpoints and placeholder tokens. Do not add public pages to `docs-site/sidebars.js` unless the task explicitly includes navigation work.

Specific high-risk changes require extra coverage:

- Model/provider routing changes: document model-group intent, API shapes, modalities, tool dialects, validation evidence, price metadata, promotion criteria, and rollback criteria.
- Evaluation or benchmark changes: explain the product decision context, success criteria, verifier used, workload limits, cost/latency evidence, and date of data collection.
- Security or auth changes: document caller-visible errors, admin/operator controls, least-privilege policy, redaction boundaries, smoke tests, and audit/report fields.
- Usage/telemetry changes: document database/report fields as scalar queryable data, saved request-time cost inputs, latency/performance dimensions, retention impact, and customer-safe examples.
- TypeScript or external policy routing changes: document trusted input shape, safe context fields, egress boundaries, fail-closed behavior, caller-visible errors, and at least one tested runnable example.
- PII/content-capture changes: distinguish routing demos from outbound redaction, keep raw matched values and placeholder maps out of persisted telemetry, and document governed capture separately.

## Evaluation And Security Grouping

Keep public evaluation pages grouped around buyer and operator proof points, not private operational history:

- Evaluation proof path: `commercial-evaluation`, `evaluate-smart-router`, `product-capabilities`, `model-group-quality`, and `harbor-case-study`.
- Operational rollout path: `deployment-readiness` and `operational-acceptance`.
- Security proof path: `security-and-trust` and `deployment-security-assessment`.
- Cost and market path: `cost-governance`, `report-examples`, and `competitive-landscape`.

Internal runbooks should mirror those groups:

- Evaluation evidence lives in `docs/SMOKE_TEST_MATRIX.md`, `docs/PRODUCT_CAPABILITY_MATRIX.md`, evaluation scripts, and source-dated case-study artifacts.
- Security evidence lives in `docs/SECURITY_REVIEW_NOTES.md`, `docs/AUTHORIZATION.md`, admin/report runbooks, scan output summaries, and deployment sign-off records.
- Operational evidence lives in `docs/DEPLOYMENT.md`, `docs/DOCKER_DEPLOYMENT.md`, `deployment.md`, package validation, smoke-test logs, and rollback notes.

When one slice affects more than one group, update all affected public pages and internal sources in the same change. For example, a new admin security report changes security/trust, usage reporting, error reference, router config, authorization docs, and deployment readiness.

## Stale-Doc Search Checklist

Before finishing a docs-affecting change, run targeted searches for old names, outdated model status, removed fields, stale errors, and private markers. Start with the changed concept and then search adjacent surfaces from the source-of-truth map.

Recommended commands:

```bash
rtk rg -n "<changed-field>|<old-field>|<new-field>" README.md docs docs-site config.example.yaml internal scripts
rtk rg -n "<old-model>|<new-model>|<provider>|<group-name>" README.md docs docs-site config.example.yaml internal scripts
rtk rg -n "current|active|reference|production|hosted|catalog-only|fallback|failover" README.md docs docs-site
rtk rg -n "metrics-forbidden|reports-forbidden|content-forbidden|model-not-allowed|no-eligible-target" README.md docs docs-site internal
rtk rg -n "raw prompt|raw image|token hash|provider key|private key|full config|SSH|hostname" README.md docs docs-site scripts
```

For provider/model/pricing/capability work, also search source-dated model names and old route policy labels. Mark historical sections with a date and reason when old benchmark output must remain for comparison.

## Secret-Safety Checks

Docs and examples must never expose:

- provider API keys;
- raw router tokens or token suffixes;
- token hashes;
- real `license.json` payloads, signing keys, signing-service credentials, or customer-specific license data;
- full production config contents;
- raw prompts, raw images, raw tool outputs, raw policy request/response JSON, cookies, OIDC tokens, or unsanitized provider responses;
- private hostnames, IP addresses, SSH usernames, key paths, live compose paths, backup paths, or production token-file paths in public Docusaurus docs;
- private upstream URLs in public examples unless written as generic placeholders.

Use placeholders such as `https://router.example.com`, `ROUTER_TOKEN`, `rtr_metrum_<user>_<project>_<env>_<key>_<secret>`, `PROVIDER_API_KEY`, and `config/config.yaml`.

Run:

```bash
rtk make docs-qa
rtk make secret-check
```

`docs-qa` checks public-facing docs for known private deployment markers and stale current-route claims. `secret-check` verifies environment examples, package-content validation tests, and license SKU checks. For public docs changes, run `rtk make docs-build` when feasible.

## Packaging Boundaries

Do not add this runbook to `scripts/package_docs_allowlist.txt` unless the release package should intentionally include internal maintenance guidance. Packaged docs should remain customer/operator safe and must pass package validation.

If a new internal runbook contains production-specific operations, keep it out of `docs-site/`, out of the package allowlist, and out of public examples. If customer-facing behavior depends on that runbook, write a separate sanitized Docusaurus explanation.

## Final Review Checklist

- The changed public docs have matching internal/operator source-of-truth updates.
- The changed internal docs point to any public caller-facing behavior that must be kept aligned.
- Public docs contain no private hostnames, SSH details, raw secrets, token hashes, full configs, or internal-only deployment procedures.
- Model groups are described as deployment-defined contracts, not hardcoded product constants.
- Pricing, competitive, provider, and serving-framework claims are source-dated or revalidated during the task.
- Evaluation docs state the verifier, workload, success criteria, date, and limits of the evidence.
- Security docs distinguish ordinary caller access, metrics-admin access, browser-admin identity, admin-report authorization, and content-capture maintenance authorization.
- Stale-doc searches and relevant docs checks have been run or explicitly reported as not feasible.
