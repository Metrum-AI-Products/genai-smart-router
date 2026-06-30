---
title: Coding-Agent Client Matrix
doc_type: reference
---

# Coding-Agent Client Matrix

GenAI Smart Router can serve coding-agent clients through deployment-defined model groups. Use `/v1/models` with your router token to see the groups your token may request; hosted examples may use names such as `big-coder`, but your deployment can choose different names.

## Compatibility Matrix

| Client | Router API shape | What to validate |
|---|---|---|
| Codex CLI | OpenAI Responses | Text, file edits, function tools, image attachments when supported, tiny output caps, and allowed model-group selection |
| Claude Code CLI | Anthropic Messages | Text, client tools, default thinking behavior where configured, image-bearing Messages payloads, and Anthropic auth environment |
| opencode | OpenAI-compatible Chat or Anthropic-compatible, depending on client config | Text, repository edit tasks, tools, and the configured provider shape |
| aider | OpenAI-compatible Chat in common router setups | Repository edit task, unit-test verifier, and explicit model-group selection |
| IDE/agent clients | Client-specific OpenAI or Anthropic adapter | Minimal manual smoke when headless validation is unavailable |

Successful smokes should prove task outcome, not just HTTP status. For repository edit tasks, run a unit test or check an exact file diff. For image tasks, verify the answer against an expected result. For disallowed model groups, expect a safe router error and no upstream provider call.

Different clients can see different effective target pools inside the same model group because they use different API surfaces. Codex uses OpenAI Responses, Claude Code uses Anthropic Messages, and many IDE clients use OpenAI Chat. A target validated for Chat tools is not automatically eligible for Responses function tools or Anthropic client tools. If one client receives `no-eligible-target` or appears to route to fewer upstreams than another client, ask the deployment admin to inspect effective provider-skin eligibility for that group.

For rollout decisions, keep client setup distinct from outcome evaluation. A setup smoke proves that a client can reach a compatible router path; a workload verifier such as Harbor, unit tests, browser-control checks, OCR goldens, or product acceptance tests proves whether the model group completes the job. See [Prove Router Quality](../evaluation/prove-router-quality) and the [Harbor Case Study](../evaluation/harbor-case-study) for evaluation patterns.

## Codex CLI

Codex CLI uses the OpenAI Responses-compatible router path. Set a router token in `METRUM_ROUTER_KEY` and configure `wire_api="responses"`.

```bash
export METRUM_ROUTER_KEY="rtr_metrum_<user>_<project>_<env>_<key>_<secret>"

codex exec --ignore-user-config --ephemeral \
  --skip-git-repo-check \
  -c 'model="<allowed-model-group>"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="https://<router-host>/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex ok" </dev/null
```

For image-bearing validation, attach an image with `--image` and use the same allowed coding group if that group includes validated multimodal Responses targets.

Codex requests use the Responses API shape. If a model group contains many OpenAI Chat targets but only a smaller set of Responses-compatible targets, Codex will route within that Responses subset. For example, a deployment may validate MiniMax `MiniMax-M3` as separate Chat, Responses, and Anthropic Messages provider skins; Codex can use only the Responses skin in that group.

## Claude Code CLI

Claude Code uses the Anthropic Messages-compatible router path. Use `ANTHROPIC_AUTH_TOKEN` for the router token and unset `ANTHROPIC_API_KEY` for router traffic.

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="https://<router-host>"
export ANTHROPIC_AUTH_TOKEN="rtr_metrum_<user>_<project>_<env>_<key>_<secret>"

claude --bare --print --model "<allowed-model-group>" \
  "Reply with exactly: router claude ok"
```

When a Messages request includes image content, the router filters the requested group to targets that advertise `image` in `input_modalities`.

## opencode

Configure opencode to use the router as the OpenAI-compatible or Anthropic-compatible provider that matches your deployment route. Use placeholder tokens in shared examples and keep real router tokens in local environment variables or ignored config files.

OpenAI-compatible shape:

```bash
export OPENAI_API_KEY="rtr_metrum_<user>_<project>_<env>_<key>_<secret>"
export OPENAI_BASE_URL="https://<router-host>/v1"

opencode run --model "<allowed-model-group>" \
  "In this fixture repo, create opencode_smoke.txt containing exactly opencode-ok."
```

If your opencode setup uses an Anthropic-compatible provider, use the router origin and the Anthropic auth variables from the Claude Code section instead. A passing smoke should create or edit the expected fixture file and should be visible in usage reports with the selected upstream provider/model.

## aider

Use an ignored local `.aider.conf.yml` or environment variables to point aider at the router. The exact model string can vary by aider and LiteLLM version; prefer an OpenAI-compatible configuration that sends requests to the router `/v1` base URL and uses a model group returned by `/v1/models`.

Example local config pattern:

```yaml
model: openai/<allowed-model-group>
openai-api-base: https://<router-host>/v1
openai-api-key: rtr_metrum_<user>_<project>_<env>_<key>_<secret>
auto-commits: false
```

Run aider in a disposable fixture repository and verify the edit with a unit test or exact diff:

```bash
aider --config .aider.conf.yml app.py tests.py \
  --message "Change route_label() to return after, then run the tests."
```

## IDE And Agent Clients

For Cursor, Continue.dev, Cline, Roo Code, SDK-based agents, LiteLLM adapters, and similar clients, prefer headless validation when the client supports it. When it does not, run a minimal manual smoke:

- confirm the client uses the router base URL and a router token;
- request a model group returned by `/v1/models`;
- perform a text request and one repository edit or tool task;
- when the client can attach images, run a mixed OpenAI Chat tools-plus-image smoke against the intended model group and require a target that supports tools and image input on the same OpenAI Chat skin;
- record client version, request time, request ID if shown, model group, and expected output;
- verify usage reports show the selected upstream provider/model/dialect, token totals, status, and no unexpected fallback.

## Common Errors

| Error | Meaning | Next step |
|---|---|---|
| `403` or model access failure | The token is not allowed to use that model group | Call `/v1/models` with the same token |
| `502 no-eligible-target` | The group has no target for the request's dialect, tools, modality, structured output, or cap requirement | Use a compatible group or ask the deployment admin to validate and enable a target |
| One client routes to fewer upstreams than another | The clients use different API surfaces and the group has different active targets per skin | Ask the admin to check provider catalog status for effective eligibility by skin |
| Claude Code calls Anthropic directly | Environment still has direct Anthropic settings | Unset `ANTHROPIC_API_KEY` and check `ANTHROPIC_BASE_URL` |
| Image request rejected before upstream | URL safety rejected the image URL, or no image-capable target is eligible | Use a public HTTPS image URL or an inline image, and check group modality support |
| Client output is wrong but transport succeeded | The selected model group did not meet the workload quality bar | Treat this as model-group evaluation evidence, not a client setup pass |
