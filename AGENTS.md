# GenAI Smart Router Agent Instructions

These instructions apply to the whole repository.

## Project Shape

- Go router implementation lives under `internal/router` and CLI entrypoints live under `cmd/`.
- Main checked-in sample config is `config.example.yaml`.
- Local real provider keys are in ignored `env.json`.
- Local production snapshot is ignored `config.production.yaml`; keep it synced with the deployed config when production changes.
- Deployment notes are in `deployment.md` and `docs/DOCKER_DEPLOYMENT.md`, but always verify live production state before acting.

## Core Rules

- Use `rtk` before shell commands in this repo.
- Do not print provider API keys, router tokens, token hashes, or full production config contents.
- `/metrics` is global operational telemetry and must remain restricted to callers with `metrics_admin: true`. Normal application caller keys must receive `403 metrics-forbidden`; do not add tenant-scoped data labels or token IDs to unauthenticated or ordinary-caller endpoints.
- Do not commit `env.json`, `config.production.yaml`, `ROUTER_TOKEN*.txt`, generated logs, DBs, or `dist/`.
- Do not commit real `license.json`, license state files, license private keys, signing-service credentials, or customer-specific license payloads. Test license fixtures must use test-only key IDs and non-customer values.
- Provider model catalogs are metadata only. Routing weights belong only under `models.<group>.targets[]`.
- Model group names are deployment-defined strings. Never treat reference names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` as product-required constants in code or docs. Public docs may mention them only as clearly labeled reference/hosted deployment examples or historical benchmark names.
- Keep broad stable production groups and experimental groups separate. New provider/model/API-skin combinations must first be cataloged, then isolated in a smoke or staging group such as a deployment-defined `*-staging` group with deliberate caller access. Do not promote directly into broad coding or high-trust groups based only on simple text, tiny tool, or marketing evidence.
- Treat model-group promotion as request-shape promotion, not just model promotion. Before adding or increasing weight in broad coding-agent groups, validate the exact caller surfaces that will use the group: OpenAI Chat, OpenAI Responses, Anthropic Messages, and any explicit bridge direction. Include representative large tool-bearing payloads, tool count, tool-choice mode, request byte buckets, serialized tool-schema byte buckets, output-cap fields, streaming behavior, and dialect translation evidence.
- Use request-shape eligibility gates instead of broad removals when compatibility is partial. If a target passed ordinary text/tool traffic but not large IDE or coding-agent payloads, keep it eligible only for validated shapes with metadata such as `request_shape_support.max_request_bytes`, tool-schema limits, supported inbound dialects, bridge flags, and modality/tool metadata. Prefer metadata-driven target filtering over provider-name branches in Go.
- Do not blanket-switch a stable model group to another API skin to resolve an incident. Determine whether the failing traffic is Chat, Responses, Anthropic Messages, or translated bridge traffic, then add or adjust only a separately validated provider skin or bridge target. Chat, Responses, Anthropic Messages, and bridge behavior all require independent direct upstream and router-level smokes for the exact provider/model/account.
- Treat repeated upstream HTTP 400 responses as compatibility evidence. Non-retryable `upstream_bad_request` usually means the selected target cannot accept that request shape; weighted routing can appear intermittent because manual retries may select a different target. Preserve safe scalar diagnostics such as request ID, provider/model/dialect, sanitized error class, request-shape buckets, candidate/filter reasons, attempts, and fallback state without storing raw prompts, raw tool schemas, raw images, tokens, token hashes, provider keys, or full config.
- Provider model catalogs must include current `input_price_per_million_usd`, `output_price_per_million_usd`, `pricing_source`, and `pricing_updated_at` for every active or cataloged upstream model when pricing is known. Use current primary/provider docs when possible; use OpenRouter model metadata for OpenRouter-hosted routes. For VLMs, add `image_input_price_per_million_tokens_usd` or `image_input_price_per_image_usd` only when the upstream/provider or enterprise chargeback model uses separate image pricing. For self-hosted models, use the enterprise chargeback rate or explicit `0.00` with `pricing_notes`.
- Provider model catalogs must include `input_modalities` and `output_modalities` for active vision, video, audio, or other multimodal targets. Do not mark a target with `image` until a direct upstream image smoke and a router-level image smoke pass for the exact provider/model/dialect/skin.
- Do not force coding-agent users to choose between a language route and a vision route for ordinary mixed tasks. Deployment-defined developer-accessible groups, for example the reference `default`, `fast`, `small`, `medium`, `high`, and coding groups, should include validated multimodal tool-capable targets for image-bearing Codex/Claude Code requests, while request-shape filtering keeps text-only traffic on the normal text/tool targets. A dedicated `vision` group is useful for explicit OCR/VLM traffic, but it must not be the only way an agent can send images.
- Tool capability metadata belongs in provider catalogs as `tool_support` and must be based on a real direct upstream smoke plus router-level smoke for the exact dialect/skin. Do not claim `openai_chat`, `openai_responses`, `anthropic_messages`, or `provider_hosted` support from marketing copy alone.
- OpenAI-compatible upstream encoding belongs in provider catalog or target metadata, not hardcoded provider-name branches in Go. Set `force_store_false` only after the exact upstream accepts `store:false`; leave it unset for providers such as Crusoe that reject `store`. Set `output_token_field: max_completion_tokens` for OpenAI Chat targets that require that field instead of `max_tokens`, and validate Chat, Responses, and translated paths independently.
- Usage persistence uses GORM. Keep the entire usage DB schema purely relational and queryable: no JSON/JSONB columns, no array columns, no serialized blobs for structured data, and no packed multi-value text fields. If one request needs multiple related rows, add a child table with scalar columns and a foreign key to `request_usage`. For open-ended or polymorphic telemetry such as routing signals, filters, score terms, capabilities, policy reasons, and evaluation outcomes, use normalized child tables with typed scalar columns, stable event/signal names, sequence numbers, and units so decisions remain fully traceable, reproducible, and SQL-queryable.
- When you change a diagnostic GORM model, also rerun `rtk make docs-diag-schema` and update `docs-site/docs/reference/diagnostics-schema.md`.
- Usage rows store request-time cost inputs, image/VLM event fields, calculated costs, and upstream-reported billed costs as scalar columns. Reports must sum stored cost values, not recalculate historical cost from current config.
- Usage reports must support performance triage from both sides of the proxy: downstream user/project/client latency and throughput, plus upstream provider/model/dialect latency, TTFB, duration, throughput, errors, attempts, and fallbacks. Do not add a report view that only explains spend if the same data can also explain slow user experience or slow upstream endpoints.
- SQL-backed usage/admin report changes must be valid for production PostgreSQL, not only local SQLite. Avoid SQLite-only query shapes such as grouping by literal constants for empty dimensions or using SELECT aliases inside arithmetic ORDER BY expressions; omit empty dimensions from `GROUP BY` and repeat aggregate expressions or use a subquery when PostgreSQL requires it. Add regression coverage for primary-only and primary-plus-secondary dimensions whenever report grouping logic changes.
- If a production report, deployment, routing, or diagnostics failure returns only a generic caller-facing error and logs do not expose a safe root-cause signal, create a detailed GitHub issue for sanitized operational logging before closeout. The issue must require safe scalar context such as report name, handler, database driver, request ID when available, sanitized error class/message, and filter summary, while explicitly forbidding tokens, token hashes, provider keys, Basic credentials, prompts, raw images, raw tool payloads, raw SQL with values, full config, and environment values.
- Customer-facing graphical report examples should be rendered in Docusaurus with Chart.js or a similar docs-layer component fed by anonymized report data. Keep the router usage-report CLI focused on stable Markdown tables and machine-reviewable metrics unless product requirements explicitly call for chart output from the binary.
- Model-group `pii_filter` redacts configured text before target selection, cache-key generation, routing-policy inputs, and upstream calls. Keep placeholder mappings in memory only unless a separate governed content-capture feature explicitly enables durable storage. Usage/log metadata may record only safe scalar values such as applied flag, mode, replacement count, and matched-rule count; never persist raw matched values, regex captures, placeholder maps, raw prompts, raw images, raw tool outputs, bearer tokens, provider keys, or token hashes.
- Do not activate unavailable or unvalidated provider models. Catalog-only is acceptable when a model exists but the current account is not entitled; active routes require current exact provider/model/dialect evidence and must match the deployment snapshot.
- Prefer structured YAML/JSON parsing for config changes. Avoid fragile text edits for production config.
- Keep sample config, local production snapshot, production config, docs, and tests in sync for behavior changes.
- Production incidents that affect routing, providers, request translation, quota or traffic shaping, diagnostics, or caller-visible errors require a sanitized production-derived regression fixture under `testdata/smokes/production-derived/` before closeout. Use synthetic payload templates only, and validate them with the local `ProductionDerived` tests plus staging/production smoke evidence when caller access and report access are available.
- Watch the repository like a hawk for important behavior, config, deployment, provider/model, auth, telemetry, CLI, docs-site, and production changes. Before finishing work, inspect nearby code/docs and search for stale or conflicting facts so internal docs and external Docusaurus docs remain correct, complete, and aligned with the shipped behavior.
- Maintain active API-compatibility evidence for caller-visible OpenAI Chat, OpenAI Responses, Anthropic Messages, streaming usage, and tool-call surfaces. When caller access is available, run bounded authenticated production probes for the exact deployment-defined group and request shape; preserve only safe scalar outcomes such as endpoint/dialect, group, HTTP status, stream-usage presence, tool-call shape, request ID, sanitized error class, latency bucket, and selected target metadata. Do not retain prompts, model output, raw tool payloads, authorization values, provider keys, token hashes, or full configuration. For any reproducible non-2xx response, required-field/schema mismatch, translation loss, malformed tool call, missing required usage chunk, or dialect discrepancy, create a detailed GitHub issue before closeout with expected versus actual behavior, a sanitized reproduction, affected request shape, mitigation/rollback notes, and explicit local plus production validation criteria; add a sanitized production-derived regression fixture when applicable.
- No stale docs. Before finishing any task that changes behavior, config, deployment, models, auth, CLI usage, tests, or production, search the repo for old names/status and update every matching doc or fixture. If a doc cannot be made current, mark the exact section as historical with a date and reason.
- Documentation updates must cover both customer-facing surfaces when a behavior affects routing, auth, models, CLI/API usage, telemetry, deployment, or production operations:
  - Operator/deployment docs (`README.md`, `docs/`, `deployment.md`, scripts, and config comments) must explain configuration, validation, rollout, rollback, and operational impact.
  - Product/API docs (`docs-site/`, embedded under `/docs/`) must explain what callers request, what behavior they can expect from the proxy, and any client-facing examples without private deployment details.
- When a task reveals redundant, obsolete, misleading, or superseded docs/content, either delete or update it when clearly safe; otherwise explicitly flag it in the PR or final response with a recommendation to delete, merge, or mark historical. Do not leave duplicate public guidance that can drift from the current product behavior.
- Customer-facing hosted docs live in `docs-site/` and are embedded into release binaries under `/docs/`. Keep these docs free of raw provider keys, real router tokens, private host paths, SSH details, and private deployment notes. Route interested readers to `mailto:contact@metrum.ai`.
- Do not embed dated `pricing_updated_at` or `pricing_source` strings in `docs-site/**`. Pull pricing source/date evidence from `config.example.yaml` or link readers to the shipped sample config as the source of truth.
- One canonical home per docs page in `docs-site/sidebars.js`; secondary discovery locations must use a single cross-reference paragraph instead of duplicating the page in another sidebar category.
- Customer-facing hosted docs must describe shipped binaries, deployment packages, runtime config, API behavior, and build timestamps. Do not expose source-control concepts such as source commits, repository state, source maps, or internal build workflow details in product docs unless the page is explicitly about an open-source third-party product.
- Customer-facing hosted docs must keep the per-page router version banner, `docs-version` metadata, releases index, release notes, and upgrade guide current. Before packaging a release, run `rtk make release-notes-from-git`, review the generated notes for customer-safe language, and run `rtk make docs-qa`.
- External Docusaurus product docs must maintain reference material for API compatibility, error responses, model metadata, provider/model onboarding, deployment options, CLI clients, usage reporting, and VLM/tool behavior when those areas change.
- Every Docusaurus docs page under `docs-site/docs` must include `doc_type: tutorial|howto|reference|explanation` frontmatter. `rtk make docs-build` runs the public-docs check that enforces this field.
- Public install docs use `docs-site/docs/installation/` as the canonical home for work before the first request lands: artifact selection, Docker Compose, binary, Kubernetes, package validation, license/config placement, first smoke tests, and rollback planning. `docs-site/docs/operations/` is the canonical home for post-deployment runtime topics such as scaling, observability, reporting, and recurring troubleshooting. Do not keep duplicate deployment-shape guidance in both sections; fold it into the canonical home and update the sidebar/link targets.
- Operator/deployment docs in `docs/` must maintain runbooks for production deployment, troubleshooting, smoke testing, usage reporting, and security review notes. Keep private hostnames, SSH details, backup paths, and production procedures out of public Docusaurus docs unless explicitly labeled as a historical case study.
- Competitive landscape or market-positioning docs must be primary-source-first and source-dated. Do not publish exact competitor pricing unless revalidated during that task. Public comparison docs should position GenAI Smart Router around customer value: high-performance routing, telemetry, budgets/rate limits, deployment-defined routing policy, TypeScript programmable policy, VLM/tool/agent-aware eligibility, private upstream support, outcome-based validation harnesses, usage/cost accounting, and governed caller access. Avoid public-doc wording such as "honest boundaries", "not positioned as", "this repository does not", "repo-visible", "unsupported claim", or "defensible"; describe fit, deployment model, complementary products, and validated capabilities instead.
- Deployment validation docs must treat each model group as a quality and cost contract. For every exposed group, document intended workloads, API shapes, modalities, tool dialects, success criteria, validation harness or objective test, cost/latency targets, promotion criteria, and rollback criteria. Emphasize that not every task needs the most expensive model; validation should preserve task outcomes while enabling lower-cost provider/model mixes where they pass.
- Public case studies and benchmark docs must explain the product decision context before presenting tables. State that different GenAI/VLM/agent models are good at different jobs and to different degrees, that the operational goal is the best cheapest model or model mix that still completes the job, and that model-group sufficiency should be proven with objective outcome evaluation. Harbor may be used as one example agent-eval harness, but docs should make clear that teams can use any workload-appropriate verifier such as unit tests, extraction accuracy checks, OCR targets, tool-call correctness, browser-control tasks, golden datasets, or product acceptance tests.
- Public deployment readiness docs should include security assessment, dependency/container scan expectations, diagnostics redaction, metrics-admin isolation, private-upstream network controls, client acceptance, model-group quality criteria, operational readiness, rollback, and post-deploy cleanup.
- TypeScript routing changes require both admin and proxy-user docs. Include the script context shape, model-group configuration, caller-visible behavior, and at least one tested example when documenting a new script policy pattern.
- TypeScript PII-aware routing examples are routing demos only unless they use model-group `pii_filter`. Do not claim a routing script redacts outbound request content; labels and telemetry must not include raw matched PII.
- External routing policy service changes require both internal docs and external Docusaurus docs. Document the `strategy: external` config, policy request/response schema, security boundaries, failure behavior, caller-visible errors, and a tested runnable example. The external policy service receives prompt/message context, safe caller metadata, eligible target metadata, pricing, tools, and modalities, so treat it as trusted deployment infrastructure; never pass raw router tokens, token hashes, provider API keys, or full production config.
- Outcome-calibrated external-routing examples must distinguish synthetic wiring tests from provider-backed evidence. A provider-backed run must use a real embedding API call, real candidate upstream responses, an objective or reviewer-recorded outcome, router request IDs, and authenticated policy-selection readback. Use a unique run marker when response caching is enabled so a cache hit cannot bypass policy selection. Keep datasets, response-bearing reviews, audit credentials, and live evidence in the protected deployment boundary; do not commit them or hardcode a specific deployment's providers, models, domains, or credentials into product behavior.
- Self-hosted upstream changes or examples require both admin and proxy-user docs. Cover enterprise-hosted vLLM/SGLang-style OpenAI-compatible services, private `/v1` base URLs, served model IDs, parser/chat-template requirements, tool-call behavior, caller-visible model groups, direct upstream smokes, router smokes, and rollback/operational notes. Verify current upstream documentation online before documenting vLLM, SGLang, or similar fast-moving serving frameworks.
- Public API examples in `docs-site/` must be tested before deployment. For Python examples, use `uv` in an ignored temporary project under `tmp/`, run the exact documented dependency/install flow, and keep docs generic with placeholder router tokens.
- Caller-token or model-group access behavior changes must keep the public Available Models And Access docs current. `/v1/models` is the caller-facing source of truth for allowed router model groups; examples that require a model value should link users there instead of assuming they already know an allowed group name.
- Harbor benchmark traffic in production should use the reusable production caller `harbor-reusable-prod`, whose raw token is stored only on the production host at `/opt/smart-llmrouter/compose/ROUTER_TOKEN_HARBOR.txt`. This caller is intentionally allowed to all deployed model groups so Harbor can compare groups without creating temporary per-run API keys. Do not generate one caller token per `{agent, model_group}` for routine Harbor runs; use the run matrix, client, model group, timestamps, and usage-report filters to separate results. Temporary Harbor keys are acceptable only for isolated investigations and must be removed from production config and quota state after the run.

## Local Work-Item Tracking And Dashboard

- Any instruction to create, read, or update the plan, local plan, task list, task status, next steps, or pending work always refers to the reconciled checked-in NDJSON state from `work-items.ndjson` plus `work-item-events.ndjson`. Never maintain a competing Markdown plan, agent-only todo list, dashboard-owned state, generated projection, or ignored local plan.
- `work-items.ndjson` is the durable baseline and `work-item-events.ndjson` is the append-only task journal. Do not hand-edit the journal or rewrite old events; use `scripts/work_items.py` for every task write so locking, optimistic status checks, revision and dependency validation, global task-ID uniqueness, and `fsync` protect concurrent agents.
- Every task has explicit `status_reason`, `next_steps`, and `human_actions`. Active statuses (`pending`, `ready`, `in_progress`, and `blocked`) require a next step; closed statuses may omit it. A status transition must replace all three fields so stale instructions cannot survive.
- Every `blocked` task must make the stop condition and exit path answerable. Its description and `status_reason` must name each concrete blocker and the observable conditions that will unblock it. Its `human_actions` must be a non-empty ordered list of specific questions, each ending in `?`, whose answers resolve every outstanding decision; never use a vague instruction such as "review" or "approve." Its `next_steps` must say how the answers and dependency evidence will be verified and must keep mutation disabled until all unblock conditions pass. When any task becomes blocked, update these fields in the same event rather than leaving the dashboard to infer what is missing.
- Put related GitHub issues (`#<number>`), pull requests (`PR#<number>`), and full HTTP(S) references in status context or structured references. Never put credentials, secret values, full configs, or private operational URLs in task records because dashboard users can see them.


  ```bash
  # Validate and inspect reconciled checked-in state.
  rtk python3 scripts/work_items.py validate
  rtk python3 scripts/work_items.py list
  rtk python3 scripts/work_items.py list --status in_progress --json

  # Add a tracked task. IDs must be globally unique and start with task.
  rtk python3 scripts/work_items.py --actor <agent-or-user> add task.<name> \
    --title "<title>" \
    --description "<scope and intended result>" \
    --stream <stream> \
    --phase <number> \
    --priority <critical|high|normal|low> \
    --status-reason "<why this task has its initial status>" \
    --next-step "<next concrete action>" \
    --human-action "<optional action a person can take>"

  # Append a revisioned update; use --expect-status to reject stale writes.
  rtk python3 scripts/work_items.py --actor <agent-or-user> update task.<name> \
    --expect-status <current-status> \
    --status <pending|ready|in_progress|blocked|done|cancelled> \
    --status-reason "<why the status changed>" \
    --replace-next-steps "<next concrete action>" \
    --replace-human-actions "<optional action a person can take>"

  # Replace title/description when an existing task is deliberately broadened or narrowed.
  rtk python3 scripts/work_items.py --actor <agent-or-user> update task.<name> \
    --expect-status <current-status> \
    --title "<current consolidated scope>" \
    --description "<current intended result and boundaries>"

  # Replace the entire dependency list atomically; omit record IDs to clear it.
  rtk python3 scripts/work_items.py --actor <agent-or-user> update task.<name> \
    --expect-status <current-status> \
    --replace-requires <record-id> <record-id>

  # Replace structured task lists atomically; omit values after any option to clear that list.
  rtk python3 scripts/work_items.py --actor <agent-or-user> update task.<name> \
    --expect-status <current-status> \
    --replace-actions "<action>" "<action>" \
    --replace-acceptance "<criterion>" \
    --replace-evidence "<evidence>" \
    --replace-commands "<command>" \
    --replace-references "<reference>"

  # Update context without changing status; append or atomically replace lists.
  rtk python3 scripts/work_items.py --actor <agent-or-user> update task.<name> \
    --expect-status <current-status> \
    --status-reason "<current explanation>" \
    --replace-next-steps "<next concrete action>" \
    --replace-human-actions
  ```

- After every task write, validate with `rtk python3 scripts/work_items.py validate`, inspect `work-items.ndjson` and `work-item-events.ndjson`, and commit the journal change with the work it describes. A plan or status update is not durable until its NDJSON event is checked in.
- `work-items.sql` and the live Mosaic dashboard under `work-dashboard/` are read-only derived views. The running dashboard must reconcile both NDJSON sources and auto-refresh directly from their current contents within about one second, without manual import, synchronization, rebuild, or restart. If a valid NDJSON write is not reflected automatically, treat that as a dashboard bug; never patch a generated database or dashboard state to hide it. Configure `WORK_ITEMS_DASHBOARD_PORT` in ignored `env.json`, then run:

  ```bash
  cd work-dashboard
  rtk npm install
  rtk npm run dev
  ```

  For a built preview, run `rtk npm run build` followed by `rtk npm run preview`. Both commands bind to `0.0.0.0` on the configured port; restrict access with localhost binding, a firewall, VPN, or an authenticated reverse proxy. The dashboard has no authentication and may display task titles, assignments, references, and acceptance details.
- WeasyPrint does not execute the JavaScript dashboard. Generate the checked-in helper's self-contained, reconciled HTML snapshot first, then render that HTML to an offline PDF:

  ```bash
  mkdir -p tmp
  rtk python3 scripts/work_dashboard_snapshot.py \
    --output tmp/work-dashboard.html \
    --title "GenAI Smart Router Work Dashboard"
  rtk uvx --from weasyprint weasyprint \
    tmp/work-dashboard.html \
    tmp/work-dashboard.pdf
  ```

  The static snapshot includes status summaries, the current task register, revisions, descriptions, actions, acceptance criteria, evidence, dependencies, and references. It requires no live dashboard server after generation. `tmp/` is ignored; do not commit generated HTML/PDF artifacts unless a release or audit requirement explicitly calls for them. Regenerate the snapshot after every task-state change rather than treating an older PDF as current state.

## Development Workflow

1. Inspect current state:
   - `rtk git status --short`
   - `rtk rg -n "<term>" config.example.yaml internal docs README.md scripts`
   - For production-impacting work, inspect `config.production.yaml` and the live host summary.
2. Make scoped edits using `apply_patch`.
3. Run formatting when Go files change:
   - `rtk gofmt -w <go files>`
4. Run tests:
   - `rtk go test ./...`
5. For provider/model changes, run direct live provider smoke tests with the relevant key from `env.json` before activating the model in routing.
6. For router behavior changes, run a router-level smoke test locally or against production, depending on the requested scope.
7. For pricing/tool/modality metadata changes, verify current pricing/capability docs online, update `config.example.yaml`, ignored `config.production.yaml`, production config when requested, tests, and public/internal docs together.
8. Update docs for any user-facing config, model, deployment, CLI, or operational change.
9. Run a stale-doc search for changed concepts before final response. Examples:
   - `rtk rg -n "old-model|old-provider|old-image-tag" README.md docs deployment.md config.example.yaml internal scripts`
   - `rtk rg -n "MiniMax-Text-01|text-01|openrouter/pareto|moonshotai/kimi|qwen|glm|hy3|kat-coder|nemotron|mercury|ling-2\\.6|big-coder.*failover" README.md docs deployment.md internal scripts`
   - `rtk rg -n "request_usage|request_attempts|request_trace_events|request_traffic_shape_events|request_upstream_shape_events|request_errors|diagnostics-schema" README.md docs docs-site internal scripts`
10. When feature-branch work is complete and thoroughly tested, prepare a focused GitHub pull request with acceptance, QA/check, rollback, and safe demo evidence. Follow the repository's approval and branch-protection requirements; never force, bypass, or infer merge authority.
11. For actionable post-review findings, first search open issues and their source review-comment URLs. Record a detailed linked follow-up issue unless the approved review workflow explicitly authorizes a fix in the current change. Do not create parallel implementation PRs for individual review comments: consolidate all open review-derived work under one labeled master issue, one active NDJSON task, one clean implementation branch/worktree, and one PR. Deduplicate already-merged behavior and overlapping acceptance criteria explicitly. The master must use native sub-issue links plus one hidden `<!-- review-followup-rollup:v1 children=<comma-separated issue numbers> -->` marker and the `review-followup-rollup` label. After the consolidated PR merges and evidence is recorded, closing the master invokes `.github/workflows/close-review-followups.yml` to comment on and close any still-open listed children. Route sensitive security findings through the approved private security path. After merge, remove only clean confirmed-merged worktrees/local branches.
12. When explicitly asked to defer review follow-ups, audit every merged PR at or above the user-specified PR number for review comments, create one date-stamped `review-followup-rollup` backlog issue, and list every unaddressed finding with its source URL and existing follow-up issue. Do not amend historical PRs or start fixes from that audit. Continue merging eligible current PRs and clean their merged worktrees without an additional review wait. GitHub permits a sub-issue only one parent; if findings already belong to an active master, retain that native hierarchy and cross-link the deferred backlog instead of moving or duplicating its children. The future owner must reconcile current source and live state, deduplicate shipped behavior, update the active NDJSON task, and deliver one focused remediation PR.

## Live Provider Testing

- Canonical provider/model onboarding procedure: `docs/onboard-model.md`. Use it with the capability smoke templates in `docs/smoke-commands-reference.md` before adding or promoting any upstream model, endpoint, or API skin.
- New upstream model activation methodology:
  - Verify current pricing, context limits, modalities, and advertised tool support from primary/current provider sources before editing config. For OpenRouter models, prefer the model/provider page or `/api/v1/models`; remember Nitro suffixes may need a real completion call even when the catalog shows the base model ID.
  - Direct-smoke the exact provider/model ID and suffix before adding active routes. At minimum run text, realistic-budget image/VLM when image is claimed, and a real tool-call request when tools are claimed.
  - Validate every router skin that will be active: OpenAI Chat for ordinary chat targets, OpenAI Responses for Codex/tool passthrough, and Anthropic Messages for Claude Code/tool passthrough. A model passing one skin does not imply the other skins pass.
  - Test tool-choice behavior explicitly. Some reasoning models support `tool_choice: "auto"` but reject forced/object tool choice while thinking mode is enabled. Document that nuance in `pricing_notes`/metadata and either avoid forced-tool routes, add a compatible per-target setting, or keep the model catalog-only for that skin.
  - If a candidate passes text but fails image quality, tool quality, or a specific dialect/skin, do not generalize the pass. Add it only to the safe catalog/route surface, or override active target `input_modalities` to prevent unsafe image/tool selection.
  - After config edits, validate sample and production snapshots with structured YAML parsing, run router tests, run local router smokes for the exact new static/weighted routes, then update production with a timestamped backup and repeat production API plus Codex CLI/Claude Code CLI smokes when client compatibility is affected.
  - Update `AGENTS.md`, internal docs, public docs, deployment notes, and stale references in the same change whenever a model activation changes routing, capabilities, pricing, tool support, or production behavior.
- OpenAI Responses smoke:
  - `POST https://api.openai.com/v1/responses`
  - body: `{"model":"<model>","input":"Reply OK only.","max_output_tokens":16}`
- OpenAI-compatible chat smoke:
  - `POST <base_url>/chat/completions`
  - body: `{"model":"<model>","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}`
- Baseten Model APIs:
  - Configure as `dialect: openai-chat` with `base_url: https://inference.baseten.co/v1`, `api_key_env: BASETEN_API_KEY`, and the served model slug from Baseten docs or `/v1/models`.
  - Baseten publishes standard input/output pricing plus discounted cache-input pricing. Store standard `input_price_per_million_usd` and `output_price_per_million_usd` in catalog metadata; mention cache-input pricing in `pricing_notes` until the router has a separate cache-input price field for upstream-billed prompt-cache tokens.
  - Before active routing, run direct non-streaming chat, direct streaming chat with `stream_options.include_usage` and `continuous_usage_stats` if streaming behavior is documented, and direct OpenAI Chat tool-call smoke if `tool_support.openai_chat` will be claimed. Baseten Anthropic Messages support is configured as a separate `dialect: anthropic` provider with `base_url: https://inference.baseten.co` and bearer auth; add a Baseten model to Claude Code/Anthropic tool routes only after direct `/v1/messages` text and client-tool smokes pass for that exact model. Add Baseten only to OpenAI Chat and separately validated Anthropic skins unless a Responses skin is separately validated.
  - On 2026-06-17, `nvidia/Nemotron-120B-A12B` passed direct Baseten non-streaming chat, streaming chat with usage chunks, and OpenAI Chat function-call smoke with `tool_choice: "auto"`. It is text-only in router metadata and should not be added to `vision` or multimodal agent fallback routes unless a Baseten vision model passes direct plus router image smokes.
  - On 2026-06-22, `openai/gpt-oss-120b` passed direct Baseten OpenAI Chat non-streaming text, streaming with usage chunks, auto tool calls, forced OpenAI Chat `tool_choice`, Anthropic Messages text, and Anthropic Messages client-tool smokes. Reference and production configs use Baseten GPT OSS 120B as the replacement for active OpenRouter Qwen/DeepSeek text routes; do not put OpenRouter Qwen or DeepSeek back into broad active groups without explicit fresh validation and a reason to prefer that upstream.
- xAI/Grok candidates:
  - Official Grok 4.3 docs checked on 2026-06-17 list `grok-4.3` with text+image input, text output, function calling, structured outputs, configurable reasoning, 1M context, and $1.25/M input plus $2.50/M output pricing.
  - Direct xAI `grok-4.3` smokes passed on 2026-06-17 after billing was funded: text returned `OK`, receipt-image OCR returned `Rite Aid`, and xAI usage reported image tokens. Local router-level text and image smokes also passed; the image request logged `input_image_tokens`, separated image cost, and no warnings after `image_input_price_per_million_tokens_usd: 1.25` was configured.
- Vision/OpenAI-compatible chat smoke:
  - `POST <base_url>/chat/completions`
  - body: `{"model":"<model>","messages":[{"role":"user","content":[{"type":"text","text":"Read the receipt image carefully. Reply with only the merchant/store chain name printed on the receipt."},{"type":"image_url","image_url":{"url":"https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}]}],"max_tokens":512,"stream":false}`
  - Always use a realistic VLM budget for acceptance, normally `max_tokens >= 512` for OpenAI-compatible chat or `max_output_tokens >= 512` for Responses. A tiny 16/64-token budget is useful only as a low-budget behavior probe and must not be used to reject or activate a vision model.
  - Tiny caller budgets are still mandatory cap-enforcement tests. When validating router behavior, assert positive caller-supplied `max_tokens` or `max_output_tokens` values are forwarded exactly to the upstream dialect, including `max_tokens: 1` for Anthropic Messages routes that may translate to Anthropic or OpenAI Chat upstreams. Defaults such as 1024 are allowed only when the caller omitted a max-token field. If a provider accepts the request but returns far more output than requested, keep the model cataloged if otherwise useful and set `honors_max_tokens: false` on the provider model or target override so capped requests skip it.
  - Record the selected upstream model, finish reason, prompt tokens, completion tokens, image tokens when reported, and response text. A general `vision` route may keep lower-weight image-capable models that accept and process images but are weaker on OCR; document that quality caveat instead of silently removing them. For OCR-specific or browser-control routes, require a useful OCR answer such as the receipt merchant name before adding or increasing weight. If the response is empty, spends all tokens on reasoning, times out, or merely accepts the image without reading it, isolate the exact provider/model/dialect before changing the route; do not generalize one target failure to the entire group.
- Self-hosted vLLM/SGLang OpenAI-compatible smoke:
  - Validate `GET <base_url>/models` and confirm the served model ID matches `providers.<name>.models.<ref>.model`.
  - Run a direct `/chat/completions` text smoke before routing traffic through the router.
  - For tool-capable routes, run a direct `/chat/completions` request with the exact `tools`, `tool_choice`, parser/chat-template, streaming mode, and model version expected in production; then repeat through the router group.
  - Do not mark a self-hosted target `tool_only` or add it to active tool routing until the direct and router-level tool smokes return correctly shaped tool calls.
- OpenRouter Nitro variants may not appear as separate IDs in `/models`; validate by making a real completion call with the `:nitro` suffix.
- OpenRouter provider configs should set deployment-owned upstream headers in config, such as `headers.HTTP-Referer` and `headers.X-Title`; do not hardcode these headers in router code.
- When validating OpenRouter locally, watch for stale shell `OPENROUTER_API_KEY` values. `env.json` intentionally does not override an existing process env var, so run local router smokes with `env -u OPENROUTER_API_KEY ...` when the ignored local `env.json` key must be authoritative.
- OpenRouter catalog modality is not enough for activation. Direct OpenRouter and local router-level receipt-image smokes passed on 2026-06-17 for `qwen/qwen3.7-plus:nitro`, `qwen/qwen3.6-flash:nitro`, `anthropic/claude-sonnet-4.6`, `x-ai/grok-4.3`, and `minimax/minimax-m3`, but the 2026-06-22 reference and production routing policy keeps OpenRouter Qwen out of active broad groups. Separate "processed the image" from "returned the correct OCR answer": a model can stay cataloged for future general VLM/coding-agent image analysis if it demonstrably reads the image, but OCR-specific routes should require the expected merchant/name/value answer before activation or increased weight. If a production weighted smoke selects `minimax/minimax-m3` or any other target and returns empty content, retest that exact upstream directly through OpenRouter and through the router with `max_tokens >= 512`, compare against direct-provider MiniMax when relevant, and remove or lower the target if the behavior is reproducible. Keep `qwen/qwen3-vl-32b-instruct:nitro` catalog-only because it answered loyalty-program text rather than merchant name. Do not add OpenRouter Gemini routes such as `google/gemini-3.5-flash`, `google/gemini-3.1-flash-lite`, or `google/gemini-3.1-pro-preview` to active routing for the current account until access is fixed; they returned provider-privacy 404s on 2026-06-17. Do not activate candidates that return empty content, fail to inspect the image, or produce non-image-grounded answers.
- OpenRouter reasoning-heavy models such as `z-ai/glm-5.2:nitro` can return HTTP 200 with empty assistant content when `max_tokens` is too small because the budget is spent on reasoning. Before activating or increasing weight for such models, smoke test both a tiny budget and a realistic budget. For GLM 5.2, use a realistic smoke such as `max_tokens: 1024`; note if low-budget requests need a `reasoning.max_tokens` cap or should not be used as acceptance evidence.
- Baseten `zai-org/GLM-5.2` passed direct realistic-budget text (`max_tokens: 1024`), `max_tokens: 1` cap, and OpenAI Chat tool-call smokes on 2026-06-18. It is reasoning-heavy: `max_tokens: 128` can be consumed entirely by reasoning and return empty final content. Active reference and production routes use Baseten for GLM 5.2 instead of OpenRouter GLM.
- Crusoe Managed Inference:
  - Crusoe docs checked on 2026-06-24 describe Managed Inference as OpenAI-compatible at `https://api.inference.crusoecloud.com/v1`, with API keys generated from Intelligence Foundry. Configure it with `dialect: openai-chat`, `auth_scheme: bearer`, and `api_key_env: CRUSOE_API_KEY`.
  - Direct Crusoe `/v1/models` validation on 2026-06-24 required an explicit `User-Agent` and returned exact IDs including `openai/gpt-oss-120b`, `google/gemma-4-31b-it`, `meta-llama/Llama-3.3-70B-Instruct`, `zai/GLM-5.2`, `moonshotai/Kimi-K2.6`, `Qwen/Qwen3-235B-A22B-Instruct-2507`, and `nvidia/NVIDIA-Nemotron-3-Super-120B-A12B`.
  - Direct Crusoe OpenAI Chat smokes passed on 2026-06-24 for `meta-llama/Llama-3.3-70B-Instruct`: non-streaming text, streaming text, `max_tokens: 1`, auto tools, forced `tool_choice`, and `response_format` JSON schema. Local router-level smokes passed the same day for `/v1/models`, non-streaming text, streaming text, `max_tokens: 1`, OpenAI Chat tools, structured outputs, usage, request-time cost, latency, attempts, and no fallback. The reference config may declare `tool_support.openai_chat: [tools, tool_choice, structured_outputs]` for that exact model.
  - Direct Crusoe OpenAI Chat smokes passed on 2026-06-24 for `google/gemma-4-31b-it`: non-streaming text, streaming text, `max_tokens: 1`, auto tools, forced `tool_choice`, `response_format` JSON schema, and combined tools plus structured-output fields. Local router-level smokes passed the same day for OpenAI Chat tool-only routing through the reference `big-coder` group, usage, request-time cost, latency, attempts, and no fallback. Crusoe Gemma is an OpenAI Chat target; do not use it as a replacement for Anthropic Messages targets unless Crusoe exposes and passes an Anthropic Messages-compatible skin.
  - The reference config catalogs Crusoe text models with source-dated pricing from Crusoe's pricing page and includes dedicated `crusoe-smoke` and `crusoe-gemma-smoke` groups. The 2026-06-24 production-requested `big-coder` promotion adds Crusoe Gemma 4 31B-it at 20% for ordinary OpenAI Chat text routing after direct and router-level smokes, and keeps a separate tool-only Crusoe Gemma target for OpenAI Chat tool traffic. Do not use Crusoe Gemma as an Anthropic Messages target unless Crusoe exposes and passes an Anthropic Messages-compatible skin.
  - Do not claim Crusoe OpenAI Responses, Anthropic Messages, image, video, or audio support from public marketing/docs alone. Add non-text modalities or agent-group targets only after exact direct and router-level validation for the target provider/model/dialect/skin.
  - Crusoe pay-as-you-go pricing includes cached-token rates. Store standard input/output per-million-token prices in provider catalogs and mention cached-token rates in `pricing_notes` until the router has a dedicated cached-input upstream billing field.
- For OpenRouter candidates intended for coding agents, validate all configured skins before adding them to active groups: `/chat/completions`, `/responses` with a function tool, and `/messages` with an Anthropic-style tool. Keep `openrouter`, `openrouter_responses`, and `openrouter_anthropic` model catalogs in sync for models that pass all three checks.
- Fireworks Chat coding-agent targets must pass direct and router-level large-payload smokes, not only tiny text/tool probes, before broad coding-agent activation. On 2026-06-29, `accounts/fireworks/models/deepseek-v4-flash` passed direct and local router-level OpenAI Chat smokes with a synthetic 524 KB request, 24 tools, about 50 KB of serialized tool schemas, `max_tokens:32`, and about 91K prompt tokens; the production dedicated `fireworks-gpt-oss-20b-smoke` group passed the same request shape with about 90K prompt tokens. Use `scripts/large_payload_chat_smoke.py` with safe filler data and keep evidence source-dated. Revalidate before adding larger shapes, image-bearing requests, Anthropic Messages, Responses, or provider-hosted tools.
- MiniMax Codex smoke should use MiniMax-M3 through a Responses-compatible endpoint and Codex `wire_api="responses"`.
- Codex CLI image smoke should use `codex exec --image <file>` against the router Responses provider config. Claude Code image support should be validated with the Anthropic Messages image payload shape and, where the installed CLI supports direct image attachment, with the actual CLI workflow.
- If a direct provider smoke returns 403 or model-not-found, do not add that model to active route targets.

## Production Host

- This is the current Metrum-managed engineering deployment, not a product-default endpoint. GenAI Smart Router can also be licensed for on-prem or enterprise-cloud deployments with different hostnames, model group names, provider sets, and caller policies.
- Host: `ubuntu@100.30.225.66`
- SSH key: `~/.ssh/chetan-jun-2026.pem`
- Public URL: `https://llm-api-engg.metrum.ai`
- Compose directory: `/opt/smart-llmrouter/compose`
- Runtime config: `/opt/smart-llmrouter/compose/config/config.yaml`
- Provider keys: `/opt/smart-llmrouter/compose/config/env.json`
- Router token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN.txt`

Useful commands:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose ps'
rtk curl -fsS https://llm-api-engg.metrum.ai/readyz
```

## EKS Delivery Boundaries

- `metrum-fleetctl` is shipped only in binary packages and is the #555 Fleet
  lifecycle authority. It provides the bounded local safe-contract registry,
  deterministic plan, fake-adapter idempotent deploy, exact-job status, and
  separately approved delete. Live EKS/RDS mutation remains disabled.
  `metrum-smartrouterctl` is a one-release rename notice only.
- `scripts/eks_delivery.py` and the `eks-*` Make targets are source-only,
  target-policy-bound staging delivery controls. They are not a generic
  customer CLI and must not be presented or packaged as the #555 provisioner.
- #555 is the sole implementation epic for one-command customer EKS deployment.
  Extend `metrum-fleetctl`; do not create a second provisioner, registry,
  lifecycle, activation authority, or smoke framework. The public lifecycle is
  limited to deterministic `plan`, idempotent `deploy`, bounded `status`, and
  separately approved `delete`.
- A customer `deploy` must consume an approved profile plus a strict reference-only intent, create or resume one isolated licensed Router, and require no manual AWS, Kubernetes, RDS, DNS, certificate, secret, license, or workload steps after invocation. Raw credentials, DSNs, kubeconfigs, license payloads, full Router configs, and shell commands are forbidden inputs and evidence.
- Configuration and release updates use the same deployment job with a new approved immutable intent: exact config/license revisions and one image digest are pinned across retries. Do not add an out-of-band config mutation path.
- Keep mutations disabled until the #555 fake-adapter contract/security/activation suites, disposable non-production EKS E2E, and independent security/operations review pass. Production profiles remain rejected until #518 authorizes them. #545 and #586 supply approved customer/config/license intent; #507 supplies migration compatibility.

### Current Metrum staging boundary

- `https://smartrouter.apps.metrum.ai` is a one-replica staging-only EKS deployment. It must not receive ordinary production traffic or become a production DNS target; the existing Compose deployment remains production authority during validation.
- The staging caller token is in Secrets Manager at `smartrouter/staging/caller-token`; the browser-admin credential is separate at `smartrouter/staging/basic-admin`. Never print either. Only the bcrypt browser-admin hash belongs in the runtime Secret.
- Staging uses the `smartrouter-gp3` state PVC, a dedicated caller, the configured license fingerprint, and a fresh private encrypted single-AZ `db.t4g.medium` RDS PostgreSQL 18.3 database. Do not copy EC2 usage or file-backed state into staging.
- Runtime `config.yaml`, `env.json`, `license.json`, the RDS CA bundle, and `ROUTER_USAGE_DB_DSN` belong only in Kubernetes Secret `smartrouter-staging-runtime`; never commit, render, log, or paste their values.
- The reviewed overlay is `deploy/kubernetes/overlays/metrum-staging`. Follow `docs/EKS_STAGING_MIGRATION.md`; verify namespace RBAC, `nginx` ingress, namespace-local wildcard TLS, the `smartrouter-gp3` StorageClass, RDS TLS, and exact trusted-proxy CIDRs. The current ingress pod range is `192.168.0.0/16`, never the legacy Compose-only `172.18.0.0/16`.
- Do not scale staging horizontally while quota and license state are file-backed. Production EKS cutover requires separate approval, EC2 write freeze, logical PostgreSQL migration/reconciliation, DNS transition, client acceptance, and a rollback window.

## Production Config Update Process

For config-only production changes:

1. Update `config.example.yaml`.
2. Update ignored `config.production.yaml` locally.
3. Validate both with Python/YAML and confirm no forbidden references remain.
4. Run `rtk go test ./...`.
5. On the host, back up and patch `/opt/smart-llmrouter/compose/config/config.yaml` with a structured YAML script.
6. Run:
   - `sudo docker compose config >/dev/null`
   - `sudo docker compose restart router`
   - `sudo docker compose ps router`
7. Verify:
   - `curl -fsS https://llm-api-engg.metrum.ai/readyz`
   - local `config.production.yaml` SHA-256 matches the remote config SHA-256
   - an authenticated router smoke test hits the expected route/model when relevant
8. Update `deployment.md` or docs if the deployed image, model groups, operational process, or validated targets changed.
9. Search for stale deployment facts such as old image tags, removed models, and outdated route descriptions.

Never edit production config without a timestamped backup:

```text
config/config.yaml.bak.<UTC timestamp>
```

## Production Package Deployment Process

For code changes that affect runtime behavior or embedded hosted docs:

0. If local production-impacting work has diverged from `origin/main`, reconcile it first on the deployment branch, resolve conflicts by preserving both intended feature sets, and run the full verification set after reconciliation. Do not package from an unmerged branch or a dirty worktree. Stash unrelated local edits before building and restore them only after deployment verification.
1. Run relevant tests:
   - Prefer `rtk go test ./cmd/... ./internal/...` for router code.
   - `rtk go test ./...` may fail on generated Harbor/job artifact directories; if so, report that separately and do not treat it as a router package failure.
   - For public docs examples, run the exact curl/Python commands against the intended endpoint using ignored local credentials.
2. Run `rtk make docs-build` when `docs-site/` changes.
3. Commit source/docs changes before packaging so `VERSION=$(git describe --tags --always --dirty)` is a stable commit tag and not `-dirty`.
4. Build both Docker package architectures:
   - `rtk make package-docker`
   Package targets copy Markdown only from `scripts/package_docs_allowlist.txt` and run package-content validation. Do not add private production runbooks, private host/IP markers, SSH usernames/key paths, live production compose config/env/token paths, raw router tokens, token hashes, or provider keys to release artifacts.
5. Copy the package matching the production host CPU architecture, currently `dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz`, to the host with `scp`.
6. On the host:
   - back up `/opt/smart-llmrouter` to `/opt/smart-llmrouter.backup.<purpose>-<UTC timestamp>`
   - unpack the package into a fresh `/opt/smart-llmrouter`
   - copy forward live `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt` from the backup
   - set `SMART_LLMROUTER_VERSION=<version>-linux-amd64` in `compose/.env`
   - `sudo docker load -i images/smart-llmrouter-<version>-linux-amd64.tar`
   - `sudo docker compose config >/dev/null`
   - `sudo docker compose up -d`
7. Verify health, route behavior, and hosted docs when relevant:
   - `curl -fsS https://llm-api-engg.metrum.ai/readyz`
   - `curl -fsS https://llm-api-engg.metrum.ai/docs/...`
   - authenticated `/v1/models` or completion smoke for API compatibility
8. Clean up production deployment leftovers after verification: remove uploaded package/config files from the host, remove superseded temporary unpack directories, keep only intentional timestamped backups, and run `sudo docker system prune -f` when stale images/build cache/containers have accumulated and the current deployment is healthy.
9. Update `deployment.md` with image/package tag, source commit, backup path when useful, cleanup performed, and validation results.
10. Commit the deployment note after production verification.

## Router Smoke Tests

Authenticated production chat smoke:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && TOKEN=$(sudo cat ROUTER_TOKEN.txt) && curl -fsS https://llm-api-engg.metrum.ai/v1/chat/completions -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" -d "{\"model\":\"high\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply OK only.\"}],\"max_tokens\":16,\"stream\":false}"'
```

Use the current deployment's deterministic failover-first check group, for example `high` on the current Metrum-managed engineering deployment, and use repeated calls for weighted deployment-defined groups. When validating weighted groups that include reasoning-heavy OpenRouter targets, include a realistic `max_tokens` budget; a `max_tokens:16` smoke can produce false failures for GLM-style models that spend the completion budget on reasoning before emitting final content.

## Production Error And Timeout Triage

- Start from the caller-visible `X-Request-Id` or error-body `request_id`.
- Join `request_usage` to diagnostic child tables by `request_id`: `request_attempts`, `request_trace_events`, and `request_errors`.
- Use `request_attempts` to distinguish slow upstreams, provider 429s, provider 5xxs, decode errors, per-attempt timeouts, and client cancellations. Do not rely only on the terminal `request_usage.target_provider`/`target_model`, because fallback failures before the terminal attempt matter.
- Keep diagnostic rows sanitized. Never persist raw prompts, image payloads, bearer tokens, provider keys, token hashes, full upstream headers, or unsanitized provider response bodies.
- Timeout mitigation should prefer configurable group/target `attempt_timeout_ms` and evidence from per-attempt rows before removing otherwise useful models from active routing.
- Caller-facing errors should remain actionable: `504 upstream-timeout`, `503 upstream-rate-limited`, `502 upstream-failed`, or `502 no-eligible-target`, all with request ID and attempted target details when a response can still be sent.

## CLI E2E Expectations

- Claude Code should use router bearer token settings:
  - `ANTHROPIC_BASE_URL=https://llm-api-engg.metrum.ai`
  - `ANTHROPIC_AUTH_TOKEN=$ROUTER_TOKEN`
  - Do not set `ANTHROPIC_API_KEY` for router traffic.
  - In scripts, use `env -u ANTHROPIC_API_KEY ... claude ...` so a developer shell cannot accidentally force direct Anthropic `X-Api-Key` auth.
- Codex CLI should use an OpenAI-compatible provider config pointing at:
  - `https://llm-api-engg.metrum.ai/v1`
  - env key such as `METRUM_ROUTER_KEY`
  - `model_providers.<name>.wire_api="responses"`
- After production package or config changes that affect routing, models, modalities, tools, auth, or client compatibility, always run real production CLI smokes with both Codex CLI and Claude Code CLI. Raw `/v1/responses` and `/v1/messages` API smokes are useful diagnostics, but they do not replace the actual CLIs because the CLIs add their own startup, model-list, tool, and auth behavior.
- Production CLI smokes should use a deployment-defined coding-agent group, for example `big-coder` in the reference deployment. Run at least:
  - Codex CLI text smoke through `wire_api="responses"`.
  - Codex CLI image smoke with `codex exec --image <file>` when model/modalities changed.
  - Claude Code text smoke with `env -u ANTHROPIC_API_KEY ANTHROPIC_BASE_URL=... ANTHROPIC_AUTH_TOKEN=... claude -p --model <group> --output-format json ...`; assert `.result` and `modelUsage` so a quiet text-output run cannot be mistaken for a pass.
  - Claude Code tool smoke when tool routing changed, asserting the created file contents.
  - Claude-compatible image smoke through `/v1/messages`, and the actual Claude Code image workflow when the installed CLI exposes a noninteractive image attachment path.
- For CLI-generated C program tests, the CLI must generate the C program. Do not replace that with a static harness.
- For agent-tool validation, use real tool calls:
  - Claude Code via Anthropic Messages API and `claude-tools-smoke`.
  - Codex via OpenAI Responses API and `agent-tools-smoke`.
  - Warp-style OpenAI Chat Completions via `warp-agent-smoke`, using `/v1/chat/completions` with `tools`, `tool_choice`, `parallel_tool_calls`, and `stream: true`; assert downstream SSE contains `delta.tool_calls` and `finish_reason:"tool_calls"`.
  - OpenRouter-specific Claude Code via `claude-tools-smoke-openrouter`.
  - OpenRouter-specific Codex via `agent-tools-smoke-openrouter`.
  - Assert the created file contents, not only text printed by the assistant.
- Requests with tools must bypass response caching; keep regression coverage for this.
- `/v1/models` is consumed by OpenAI/Codex clients that currently accept only `text` and `image` input modalities. Keep richer internal modalities such as `video` in provider metadata for routing, but do not expose client-incompatible modality strings in the public model-list response.

## Documentation Expectations

Update docs whenever changing:

- model catalogs or active model groups
- caller token behavior or allowed groups
- provider keys/env requirements
- production deployment commands or image tags
- Codex CLI or Claude Code usage examples
- TypeScript routing script behavior, context fields, or model-group policy examples
- self-hosted vLLM/SGLang/OpenAI-compatible upstream deployment, served model IDs, parser/chat-template flags, or tool-call validation behavior
- provider model pricing metadata, pricing source/update dates, tool support metadata, or upstream capability claims
- usage reporting, caching, telemetry, or auth behavior
- model-group PII filtering, redaction/restoration behavior, privacy controls, or content-capture interactions
- DB driver/schema behavior, including SQLite/Postgres config, usage report fields, or durability expectations
- API compatibility, error semantics, model metadata, provider/model onboarding, production runbooks, smoke tests, or security expectations
- competitive positioning, product capability matrices, buyer evaluation docs, or public claims about other products

Keep docs concrete and tested. Include working commands, but redact secrets.

## Google Workspace Announcements

- Use Google Chat/Workspace announcements for production rollouts, new supported functionality, externally visible behavior changes, and important validation results when the user asks for team notification.
- Treat Google Chat incoming webhook URLs as secrets. Do not commit them, add them to docs, echo them in final responses, or store them in tracked scripts. If the user provides a webhook URL in chat, use it only for the requested post.
- Keep announcements concise and caller-focused. Include the feature or deployment outcome, production URL when appropriate, binary version or build timestamp when relevant, and high-signal validation results such as health checks, API smokes, and Codex/Claude Code CLI smokes.
- Do not include provider API keys, router tokens, private SSH details, full production config contents, or sensitive internal host paths in Workspace messages.
- After posting, it is acceptable to record only the non-secret message name/id and a short summary in the final response or deployment notes when useful. The 2026-06-17 multimodal agent routing announcement was posted successfully to Google Chat and returned message `spaces/AAAAuyaen6A/messages/khuaarPuWeo.khuaarPuWeo`.

Before finalizing, check at minimum:

```bash
rtk rg -n "MiniMax-Text-01|text-01|big-coder.*failover|failover route|does not yet have access|old image|openrouter/pareto|moonshotai/kimi|qwen|glm|hy3|kat-coder|nemotron|mercury|ling-2\\.6" README.md docs deployment.md internal scripts || true
rtk rg -n "openai/gpt|anthropic/claude|claude-sonnet|MiniMax-M2\\.7|m27-highspeed" config.example.yaml README.md docs deployment.md scripts || true
```

## Fleet And Customer CLI Boundary

- `metrum-fleetctl` is the only #555 Fleet lifecycle authority. It owns
  `plan`, `deploy`, `status`, and approved `delete` through the one normalized
  lifecycle registry. `metrum-smartrouterctl` is a one-release rename notice
  only; it MUST NOT retain lifecycle behavior.
- `smartrouterctl` is customer-local. It MAY validate/diff local config,
  generate a caller token into a new mode-`0600` file exactly once, and read
  safe local config/license/model/aggregate-usage status. It MUST NOT access
  AWS/EKS/RDS/DNS, Fleet state, cross-customer state, config activation, key
  rotation, or license signing.
- Fleet binaries belong only in binary packages. Customer Docker images MAY
  contain `smartrouterctl` but MUST NOT contain `metrum-fleetctl` or the
  compatibility command.
- SQLite is the default deployment path: exactly one Router container and one
  replica, with no RDS provision or binding. Dedicated RDS is OPTIONAL and
  requires an explicit manifest `database_profile` equal to the protected
  profile's approved database profile. Its action precedes runtime-secret
  binding only on that branch.
- Dedicated-RDS plans MUST retain only deterministic safe scalar identity and
  profile evidence—NEVER a DSN, endpoint, credential, secret reference, or
  raw adapter response. `tenant_deployment_rds.go` is a typed AWS adapter for
  private, encrypted, RDS-Proxy-disabled instances with ownership-safe final
  snapshots. It MUST stay unattached unless a validated, time-bounded
  non-production admission is supplied; the shipped CLI MUST NOT create that
  admission. Production profiles remain rejected until #518.
- Every `metrum-smartrouterctl` occurrence outside its compatibility command
  and package validation MUST explicitly say `one-release compatibility` or
  `one-release rename notice`; treat those occurrences as intentional until
  the compatibility release is removed, not as ambiguous stale references.

## References

- Codex AGENTS.md guidance: https://developers.openai.com/codex/guides/agents-md
- Codex best practices: https://developers.openai.com/codex/learn/best-practices
- Codex CLI install/update: https://developers.openai.com/codex/cli
- Claude Code auth precedence/env vars: https://code.claude.com/docs/en/authentication
