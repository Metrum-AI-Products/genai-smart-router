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

When one upstream model exposes multiple API skins, create separate provider entries for each validated skin instead of overriding target dialects on an unrelated provider. For example, MiniMax `MiniMax-M3` uses separate reference providers for OpenAI Chat (`minimax`), OpenAI Responses (`minimax_responses`), and Anthropic Messages (`minimax_anthropic`) so Codex/Responses, Cursor/OpenAI Chat, and Claude Code/Messages eligibility can be validated and rolled back independently.

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

Catalog metadata is not active routing eligibility by itself. If one upstream model is exposed through multiple provider skins, add and validate each skin as a separate provider or target dialect before expecting callers to use it. For example, `tool_support.openai_responses` on a catalog entry inherited by an `openai-chat` target documents metadata for that model, but Responses clients will not select that target unless an `openai-responses` skin or an explicitly validated bridge target is active in the requested group. Before promotion, check `/admin/reports/api/provider-catalog-status` and confirm each intended group shows the right `activeEligibilitySkin`, nonempty `effectiveToolSupport`, and no surprising `inactiveToolSupport` for the caller surface being validated.

## 6. Add A Restricted Smoke Group

Create a deployment-defined smoke group with the candidate as the only target, or as the only target for the specific request shape being validated. Restrict caller access to test tokens or internal operators. Validate sample and local production snapshots with structured YAML parsing before starting the router.

Do not promote directly from catalog to broad active groups. The smoke group proves router encoding, target eligibility, diagnostics, cost calculation, and error behavior without exposing ordinary callers.

## 7. Router-Smoke The Same Shapes

Run router-level smokes through the same API shapes that passed directly upstream:

- `/v1/chat/completions` for OpenAI Chat clients;
- `/v1/responses` for Responses-compatible clients and Codex-style tool flows;
- `/v1/messages` for Anthropic-compatible clients and Claude Code-style tool flows.

For Chat-only upstreams exposed to Responses callers through the stateless bridge, keep the target dialect `openai-chat` and add explicit `responses_to_chat` metadata only after validation. The minimum bridge evidence is direct OpenAI Chat text, cap, and function-tool smoke plus router-level `/v1/responses` text and function-tool smokes through a restricted model group. Verify the upstream attempt uses `/chat/completions`, the caller receives Responses output, usage shows inbound `openai-responses` and target `openai-chat`, and translation diagnostics record `bridge_direction = responses_to_chat`. Stateful `previous_response_id`, provider-hosted tools, file/code/computer tools, images, reasoning, structured output, and streaming remain unsupported until separate bridge flags and smokes pass.

For each route, verify selected provider/model, usage, request-time cost, latency, attempts, fallback status, and safe diagnostics. Include max-token cap checks, no-eligible-target checks for unsupported shapes, and negative media URL safety checks for image-capable targets. For tool or agent routes, run the actual client smoke in a disposable sandbox when client compatibility is part of the claim.

When a group is intended to serve several client surfaces, run the same smoke matrix against the same model group for each surface and compare selected provider/model/dialect distribution. If all traffic for one surface unexpectedly goes to a single fallback, inspect provider catalog status first: the group may have multiple active targets overall but only one effective target for that surface's native skin.

For OpenAI Chat coding-agent routes that claim both tools and image input, include a combined request with multiple messages, representative function-tool schemas, one image part, `stream:true`, and no caller output cap. The same configured target must satisfy the OpenAI Chat dialect, tool support, and image modality together. Record a negative no-eligible smoke for a group that lacks such a combined target and verify zero upstream attempts plus safe candidate/filter diagnostics.

For Chat-to-Responses bridge routes, first validate the exact upstream through direct OpenAI Responses text and function-tool smokes. Then create a restricted router smoke group whose target has `dialect: openai-responses` and explicit `bridges.chat_to_responses` metadata. Send Chat Completions non-streaming text and function-tool requests through `/v1/chat/completions`; verify the upstream path is `/v1/responses`, the target dialect in usage is `openai-responses`, the inbound dialect remains `openai-chat`, and `request_translation_shapes` plus field events contain only safe scalar buckets and field names. If `bridges.chat_to_responses.stateful_sessions.enabled` will be enabled, send two same-session requests with the configured header and verify the second upstream Responses request includes the first upstream response `id` as `previous_response_id`, while a different caller or session header value does not reuse it. Also run negative smokes for unsupported shapes such as `stream:true`, image input, structured output, or reasoning when the bridge metadata does not enable them.

For OpenAI Chat coding-agent routes that will receive large repository or IDE sessions, also run a sanitized large-payload fixture with representative message count and tool schemas. Use `scripts/large_payload_chat_smoke.py` or an equivalent synthetic helper, not captured customer content. Run it direct to the upstream first, then through a restricted local router smoke group pinned to the same target, and finally through production only when a deployment-owned safe caller token and smoke group are available. Record request bytes, tool count, serialized tool-schema bytes, output cap, prompt-token scale, status, finish reason, latency, token usage, attempts, fallback, and safe request-shape buckets. If production prerequisites are missing, record the missing base URL, caller access, or report access as a blocker without printing raw tokens, token hashes, prompts, tool schemas, provider keys, or full config.

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
