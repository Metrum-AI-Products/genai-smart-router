# Harbor Case Study

This internal note mirrors the customer-facing Harbor case study and keeps the operational evidence explicit. Harbor is an outcome-oriented agent evaluation harness: it runs a real task, captures the artifact, and scores it with an independent verifier.

The current production evidence below was captured on **2026-06-29** after deploying router build `762592b` to `https://llm-api-engg.metrum.ai`.

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

## Public Reporting Rules

- Public docs must not present old June 15 data as current production proof.
- Public docs must not publish exact competitor pricing unless it is revalidated during the task from primary sources and dated.
- Public docs should report upstreams as route families unless there is a product reason to name a provider/model and the model is active, validated, and current.
- Public docs should describe this as one deterministic run, not statistically significant evidence.

## Reproduction Checklist

1. Use the reusable production Harbor caller token from the production host token file.
2. Run Harbor with `AGENTS=codex,claude-code` and `MODEL_GROUPS=big-coder`.
3. Capture router build, config checksum, Harbor CLI version, client versions, timestamps, and result job IDs.
4. Compare reward, errors, elapsed time, token demand, selected route family, and produced artifact.
5. Follow up on any reward `0` result before promoting route weights.
