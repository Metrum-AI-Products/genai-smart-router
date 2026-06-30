# Model And Endpoint Onboarding Guide

Source-only internal runbook. Do not add this file to `scripts/package_docs_allowlist.txt` or the public Docusaurus sidebar.

Use this procedure before adding or promoting any upstream model, provider endpoint, self-hosted service, or alternate API skin. It is agent-agnostic: the same evidence standard applies whether the caller is an application, Codex, Claude Code, Cursor, Warp, opencode, aider, Harbor, or another client.

Related references:

- Capability smoke curl templates: [smoke-commands-reference.md](smoke-commands-reference.md)
- Smoke matrix and deterministic gates: [SMOKE_TEST_MATRIX.md](SMOKE_TEST_MATRIX.md)
- Public-safe provider/model guide: `docs-site/docs/reference/add-provider-model.md`
- Self-hosted vLLM/SGLang notes: [SELF_HOSTED_UPSTREAMS.md](SELF_HOSTED_UPSTREAMS.md)

## 1. Define The Candidate And Intended Surface

Record the provider, endpoint, served model ID, router provider name, model reference name, intended model groups, caller API shapes, expected clients, and whether the candidate is catalog-only, smoke-only, or intended for active routing.

Treat model group names as deployment-defined strings. Do not hardcode reference names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` as product constants.

## 2. Verify Current External Facts

Before editing config, verify current primary/provider documentation or provider metadata for:

- endpoint base URL and API path;
- authentication scheme and required headers;
- exact served model ID or suffix;
- context limits and max output behavior;
- text, image, audio, video, and tool support;
- streaming, structured-output, reasoning, and hosted-tool behavior;
- current input, output, cache, image, or enterprise chargeback pricing;
- account, region, billing, and entitlement availability.

Use source-dated notes in internal docs or config comments when the fact may change. For OpenRouter-hosted models, prefer OpenRouter model metadata and validate Nitro suffixes with a real completion call.

## 3. Direct-Smoke The Exact Upstream

Run direct provider smokes before involving the router. Test every API skin and request shape that will be claimed:

- OpenAI Chat text, streaming, max-token cap, tools, forced `tool_choice`, structured output, reasoning, and image inputs as applicable;
- OpenAI Responses text, function tools, tool-result continuation, streaming, structured output, reasoning, and image inputs as applicable;
- Anthropic Messages text, client tools, thinking, max-token cap, streaming, and image inputs as applicable.

Use the placeholder curl templates in [smoke-commands-reference.md](smoke-commands-reference.md). A pass on one skin does not imply a pass on another skin. If the upstream returns `401`, `403`, model-not-found, empty content, malformed tools, or non-grounded image answers, keep the candidate out of active routing until the exact failure is understood. For `403` or `404`, distinguish invalid provider credentials from account entitlement, region/project restriction, policy/privacy block, and model-access denial; only the exact provider key, model ID, dialect, and request shape that passed the smoke should be promoted.

## 4. Capture Capability Probe Results

Record public-safe evidence for each probe:

- date, provider, endpoint family, model ID, dialect, skin, and account/region class;
- request shape, streaming mode, max-token cap, tool-choice mode, structured-output mode, modality, and reasoning settings;
- status code, selected upstream model, finish reason, token usage, image-token usage when reported, latency, and fallback/no-fallback status;
- observed response summary, not raw prompts, raw images, raw tool schemas, raw tool outputs, bearer tokens, provider keys, router tokens, token hashes, or full config;
- pass/fail decision and the exact metadata field or route eligibility the evidence supports.

Separate transport success from task quality. A model that accepts an image is not necessarily good enough for OCR, browser control, or agent workflows.

## 5. Add Catalog Metadata Only

Add or update `providers.<provider>.models.<model_ref>` with structured metadata:

- exact upstream model ID;
- input and output modalities;
- per-million-token pricing and pricing source/update evidence when known;
- image or per-image pricing only when the upstream or chargeback model uses separate image pricing;
- `tool_support` by skin only after real smokes pass;
- structured-output, reasoning, hosted-tool, max-token, and provider-specific notes only after exact validation.

Keep routing weights only under `models.<group>.targets[]`. Keep unavailable or unentitled models catalog-only.

## 6. Add A Restricted Smoke Group

Create a deployment-defined smoke group with the candidate as the only target, or as the only target for the specific request shape being validated. Restrict caller access to test tokens or internal operators. Validate sample and local production snapshots with structured YAML parsing before starting the router.

Do not promote directly from catalog to broad active groups. The smoke group proves router encoding, target eligibility, diagnostics, cost calculation, and error behavior without exposing ordinary callers.

## 7. Router-Smoke The Same Shapes

Run router-level smokes through the same API shapes that passed directly upstream:

- `/v1/chat/completions` for OpenAI Chat clients;
- `/v1/responses` for Responses-compatible clients and Codex-style tool flows;
- `/v1/messages` for Anthropic-compatible clients and Claude Code-style tool flows.

For each route, verify selected provider/model, usage, request-time cost, latency, attempts, fallback status, and safe diagnostics. Include max-token cap checks, no-eligible-target checks for unsupported shapes, and negative media URL safety checks for image-capable targets. For tool or agent routes, run the actual client smoke in a disposable sandbox when client compatibility is part of the claim.

## 8. Promote Conservatively With Rollback

Move the candidate into active routing only after direct and router smokes, workload acceptance gates, and docs/config review pass. Start with low weight or a narrow group, then monitor status, latency, usage, costs, fallback, provider errors, and caller complaints.

Define rollback before promotion:

1. remove or lower the active target weight;
2. remove capability metadata that made unsafe requests eligible;
3. isolate the target back into a smoke group;
4. restore the previous config snapshot and restart;
5. rerun `/readyz`, `/v1/models`, and affected API smokes.

## 9. Update Docs, Tests, And Evidence

Keep behavior, config, docs, and tests aligned in the same change:

- update `config.example.yaml`, local production snapshots, and production config only when requested and validated;
- update internal runbooks and public Docusaurus docs when routing, auth, models, CLI/API behavior, telemetry, deployment, or production behavior changes;
- update diagnostics schema docs when GORM diagnostic models change;
- run stale-doc searches for old provider/model names, capability claims, route status, and private markers;
- run relevant validation such as `rtk go test ./...`, targeted router tests, `rtk make docs-qa`, and live smokes when provider/model activation is in scope.

Before final handoff, include changed files, validation commands/results, safe onboarding evidence, and any intentionally deferred live-provider or production steps.
