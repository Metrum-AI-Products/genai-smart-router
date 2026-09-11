# Harbor Case Study

This internal note mirrors the customer-facing Harbor case study and keeps the operational evidence explicit. Harbor is an open-source, sandboxed agent-evaluation framework: it runs an agent against reproducible tasks, captures the artifact and trajectory, and scores the result with an independent verifier.

The current production evidence below was captured on **2026-06-29** after deploying router build `762592b`.

Primary public sources checked for the external case study on **2026-06-30**:

- Harbor homepage: <https://harborframework.com/>
- Harbor docs introduction: <https://harbor-framework-harbor.mintlify.app/introduction>
- Harbor GitHub: <https://github.com/harbor-framework/harbor>
- Terminal-Bench: <https://www.tbench.ai/>
- Terminal-Bench paper: <https://arxiv.org/html/2601.11868v1>

Harbor is one repeatable agent-eval harness, not a Metrum Smart Router dependency. Terminal-Bench is one public benchmark family that uses the Harbor task format/harness for terminal and coding-agent tasks; it is not a universal proxy for customer success.

## Run Metadata

| Field | Value |
|---|---|
| Router build | `762592b` |
| Harbor task | `aider/polyglot_python_two-bucket` |
| Model group | `big-coder` |
| Harbor CLI | `0.13.2` |
| Codex CLI | `0.142.0` |
| Claude Code CLI | `2.1.186` |
| Caller token model | reusable production Harbor caller |

## Results

| Agent | Group | Status | Reward | Errors | Elapsed | Harbor input | Harbor cache | Harbor output | Job |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| `codex` | `big-coder` | ok | 1 | 0 | 68 s | 48,470 | 31,744 | 2,598 | `jobs/2026-06-29__19-42-34/result.json` |
| `claude-code` | `big-coder` | failed | 0 | 0 | 348 s | 278,176 | 276,886 | 22,530 | `jobs/2026-06-29__19-52-13/result.json` |

The Codex run passed the verifier. The Claude Code run completed without Harbor exceptions but failed quality: the produced `two_bucket.py` artifact was still the starter `pass` implementation, and the agent trajectory repeatedly issued read calls rather than editing the file.

## Operational Interpretation

- Production routing and authentication were healthy for `big-coder`.
- Codex CLI is validated for this task on this build and route.
- Claude Code is not validated for this task on this build and route.
- Treat this as a routing-quality follow-up, not an HTTP availability failure.
- Do not publish the route as fully green for Claude Code until a repeat Harbor run passes with reward `1`.
- Do not generalize this single task into a universal model ranking or all-customer quality claim.

## Public Reporting Rules

- Public docs must not present old June 15 data as current production proof.
- Public docs must not publish exact competitor pricing unless it is revalidated during the task from primary sources and dated.
- Public docs should report upstreams as route families unless there is a product reason to name a provider/model and the model is active, validated, and current.
- Public docs should describe this as one deterministic run, not statistically significant evidence.
- Public docs may cite Harbor-supported agent examples from current Harbor docs, but must state that this is agent context and not proof that each client has passed through our router.
- Public examples must use placeholders such as `https://router.example.com/v1`, `ROUTER_TOKEN`, `<router-model-group>`, and `<fixed-model-id>`.

## Reproduction Checklist

1. Use the approved reusable production Harbor caller without printing or copying its raw token.
2. Run Harbor with `AGENTS=codex,claude-code` and `MODEL_GROUPS=big-coder`.
3. Capture router build, config checksum, Harbor CLI version, client versions, timestamps, and result job IDs.
4. Compare reward, errors, elapsed time, token demand, selected route family, and produced artifact.
5. Follow up on any reward `0` result before promoting route weights.

## Generic Router-Versus-Fixed Pattern

For external docs and customer-shared examples, keep the command shape generic and placeholder-safe:

```bash
uv tool install harbor

harbor run -d <dataset-or-task> \
  --agent <agent> \
  --model <router-model-group>

harbor run -d <dataset-or-task> \
  --agent <agent> \
  --model <fixed-model-id>
```

Hold task, agent, versions, tools, Harbor-supported attempt and seed policy, token caps, timeouts, and verifier constant. Change only the model endpoint/group. Join Harbor rows to router reports by timestamp, caller/project, client, model group, request ID, or run label. Promote only when outcome, cost, latency, fallback, and error thresholds all match the model-group contract.

## Aligned Public Docs

Keep the public Docusaurus page `docs-site/docs/evaluation/harbor-case-study.mdx` aligned with this note. Related public pages:

- `docs-site/docs/evaluation/prove-router-quality.md`
- `docs-site/docs/evaluation/model-group-quality.md`
- `docs-site/docs/getting-started/coding-agent-clients.md`
- `docs-site/docs/operations/usage-reporting.md`
- `docs-site/docs/operations/admin-browser-reports.md`
