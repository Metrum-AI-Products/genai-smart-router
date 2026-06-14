# Harbor Agentic Coding Case Study

This case study records a production-hosted Smart LLM Router evaluation using Harbor with both Codex CLI and Claude Code CLI. The run validates that the router can serve OpenAI Responses-style agent traffic and Anthropic-style Claude Code traffic through the same hosted endpoint while preserving per-key attribution, model-group controls, tool calls, and usage reporting.

## Run Summary

- Run ID: `case-study-full-20260614T233205Z`
- Hosted router: `https://llm-api-engg.metrum.ai`
- Harbor task: `aider/polyglot_python_two-bucket`
- Caller token case: `case-20260614t211718z`
- Agents: `codex`, `claude-code`
- Model groups: `default`, `fast`, `small`, `medium`, `high`, `big-coder`
- Matrix size: 12 cells, one Harbor trial per `{agent, model_group}`
- Result: 12/12 cells passed with Harbor reward `1.0` and zero Harbor exceptions

## Task And Goal

The Harbor task asked each agent to modify `two_bucket.py` for the classic two-bucket measuring problem. Given two bucket sizes, a target volume, and the bucket that must be filled first, the implementation must return the number of actions needed, which bucket contains the target volume, and the remaining volume in the other bucket. Impossible inputs must raise `ValueError` with a message. The verifier ran the Exercism-style Python test suite for the task; each trial reported 9 collected tests.

Reward interpretation: Harbor reports reward `1.0` when the submitted artifact passes the verifier and `0.0` when it fails or the agent phase errors. In this run every cell received reward `1.0`.

## Model Groups Evaluated

Weights below are group-local relative weights from the production-synced config. `tool_only` targets are used for compatible agentic tool-call traffic and are excluded from ordinary non-tool weighted routing.

| Group | Normal weighted targets | Tool-call eligible targets |
|---|---|---|
| `default` | `minimax/MiniMax-M3` 48 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 96 (60.0%)<br>`openai/gpt-5.4-nano` 1 (0.6%)<br>`openai/gpt-5.4-mini` 1 (0.6%)<br>`openai/gpt-5.4` 1 (0.6%)<br>`minimax/MiniMax-M2.7-highspeed` 1 (0.6%)<br>`groq/qwen/qwen3-32b` 1 (0.6%)<br>`groq/llama-3.3-70b-versatile` 1 (0.6%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 1 (0.6%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 1 (0.6%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 1 (0.6%)<br>`openrouter/inception/mercury-2` 1 (0.6%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (0.6%)<br>`openrouter/z-ai/glm-5.1:nitro` 1 (0.6%)<br>`openrouter/tencent/hy3-preview:nitro` 1 (0.6%)<br>`openrouter/openai/gpt-oss-120b:nitro` 3 (1.9%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |
| `fast` | `minimax/MiniMax-M3` 39 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 78 (60.0%)<br>`openai/gpt-5.4-nano` 1 (0.8%)<br>`openai/gpt-5.4-mini` 1 (0.8%)<br>`openai/gpt-5.4` 1 (0.8%)<br>`groq/llama-3.1-8b-instant` 1 (0.8%)<br>`groq/groq/compound-mini` 1 (0.8%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 1 (0.8%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 1 (0.8%)<br>`openrouter/inception/mercury-2` 1 (0.8%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (0.8%)<br>`minimax/MiniMax-M2.7-highspeed` 1 (0.8%)<br>`openrouter/openai/gpt-oss-120b:nitro` 3 (2.3%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |
| `small` | `minimax/MiniMax-M3` 30 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 60 (60.0%)<br>`openai/gpt-5.4-nano` 1 (1.0%)<br>`openai/gpt-5.4-mini` 1 (1.0%)<br>`groq/llama-3.1-8b-instant` 1 (1.0%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 1 (1.0%)<br>`openrouter/inception/mercury-2` 1 (1.0%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (1.0%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 1 (1.0%)<br>`openrouter/openai/gpt-oss-120b:nitro` 3 (3.0%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |
| `medium` | `minimax/MiniMax-M3` 39 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 78 (60.0%)<br>`openai/gpt-5.4-mini` 1 (0.8%)<br>`openai/gpt-5.4` 1 (0.8%)<br>`openai/gpt-5.4-nano` 1 (0.8%)<br>`groq/qwen/qwen3-32b` 1 (0.8%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 1 (0.8%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 1 (0.8%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 1 (0.8%)<br>`openrouter/inception/mercury-2` 1 (0.8%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (0.8%)<br>`openrouter/z-ai/glm-5.1:nitro` 1 (0.8%)<br>`openrouter/openai/gpt-oss-120b:nitro` 3 (2.3%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |
| `high` | `minimax/MiniMax-M3` 42 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 84 (60.0%)<br>`openai/gpt-5.5` 1 (0.7%)<br>`openai/gpt-5.4` 1 (0.7%)<br>`openai/gpt-5.4-mini` 1 (0.7%)<br>`openai/gpt-5.4-nano` 1 (0.7%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 1 (0.7%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 1 (0.7%)<br>`openrouter/inception/mercury-2` 1 (0.7%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (0.7%)<br>`openrouter/z-ai/glm-5.1:nitro` 1 (0.7%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 1 (0.7%)<br>`openrouter/tencent/hy3-preview:nitro` 1 (0.7%)<br>`openrouter/openai/gpt-oss-120b:nitro` 3 (2.1%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |
| `big-coder` | `minimax/MiniMax-M3` 50 (50.0%)<br>`kimi/kimi-k2.7-code` 30 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 20 (20.0%) | `openai/gpt-5.5`<br>`openrouter_anthropic/anthropic/claude-sonnet-4.6:nitro` |

## Harbor Results

Harbor token counters are the CLI/job-level counters reported by Harbor result JSON. Throughput here is computed as tokens divided by wall-clock elapsed seconds for each Harbor cell; the router usage appendix reports per-request upstream and downstream throughput from production telemetry.

| Agent | Group | Reward | Duration s | Input Tokens | Cache Tokens | Output Tokens | Total Tokens | Output tok/s | Total tok/s | Harbor Job |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| `codex` | `default` | 1 | 73 | 57,175 | 47,488 | 1,667 | 58,842 | 22.84 | 806.05 | `jobs/2026-06-14__23-32-05/result.json` |
| `codex` | `fast` | 1 | 60 | 58,367 | 40,832 | 1,771 | 60,138 | 29.52 | 1002.30 | `jobs/2026-06-14__23-33-19/result.json` |
| `codex` | `small` | 1 | 66 | 33,573 | 17,536 | 1,584 | 35,157 | 24.00 | 532.68 | `jobs/2026-06-14__23-34-18/result.json` |
| `codex` | `medium` | 1 | 61 | 58,589 | 26,496 | 1,874 | 60,463 | 30.72 | 991.20 | `jobs/2026-06-14__23-35-24/result.json` |
| `codex` | `high` | 1 | 64 | 33,587 | 10,880 | 1,601 | 35,188 | 25.02 | 549.81 | `jobs/2026-06-14__23-36-25/result.json` |
| `codex` | `big-coder` | 1 | 64 | 33,641 | 18,560 | 1,653 | 35,294 | 25.83 | 551.47 | `jobs/2026-06-14__23-37-30/result.json` |
| `claude-code` | `default` | 1 | 112 | 97,211 | 72,035 | 5,567 | 102,778 | 49.71 | 917.66 | `jobs/2026-06-14__23-38-34/result.json` |
| `claude-code` | `fast` | 1 | 103 | 97,300 | 91,659 | 4,496 | 101,796 | 43.65 | 988.31 | `jobs/2026-06-14__23-40-25/result.json` |
| `claude-code` | `small` | 1 | 103 | 97,321 | 91,630 | 4,532 | 101,853 | 44.00 | 988.86 | `jobs/2026-06-14__23-42-08/result.json` |
| `claude-code` | `medium` | 1 | 93 | 96,993 | 91,506 | 3,876 | 100,869 | 41.68 | 1084.61 | `jobs/2026-06-14__23-43-51/result.json` |
| `claude-code` | `high` | 1 | 103 | 97,493 | 91,708 | 4,986 | 102,479 | 48.41 | 994.94 | `jobs/2026-06-14__23-45-25/result.json` |
| `claude-code` | `big-coder` | 1 | 120 | 97,217 | 91,585 | 6,024 | 103,241 | 50.20 | 860.34 | `jobs/2026-06-14__23-47-08/result.json` |

## Production Router Usage

The router usage report below was generated from the production usage database for the UTC run window and the normalized Harbor caller environment. Tool-using agent requests are intentionally cache-bypassed, so cache hit rate is not expected for this workload.

- Period UTC: `2026-06-14T23:32:00.000Z` to `2026-06-14T23:49:30.000Z`
- Requests: `54`
- Tokens: `314599` total, `274968` input, `39631` output
- Cache: `0` hits, `0` misses, `54` bypass
- Upstream attempts: `72`; fallbacks: `13`; streaming requests: `48`
- Latency: `10132 ms` avg, `67190 ms` max
- Throughput: upstream `59.00` output tok/s / `1871.45` total tok/s; downstream `707367.36` output tok/s / `4887102.08` total tok/s

### External Models Actually Used


| Provider | Model | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| openai | gpt-5.4-nano | 11 | 0 | 132212 | 128382 | 3830 | 0 | 0 | 11 | 31 | 11 | 11 | 50.87 | 3244.47 | 255772.73 | 8664045.45 | 5322 | 20635 | 5320 | 20633 |
| openai | gpt-5.5 | 8 | 0 | 94723 | 89938 | 4785 | 0 | 0 | 8 | 8 | 0 | 8 | 48.38 | 3833.38 | 251641.67 | 6687925.00 | 8578 | 20347 | 8575 | 20341 |
| openai | gpt-5.4 | 3 | 0 | 35443 | 34027 | 1416 | 0 | 0 | 3 | 3 | 0 | 3 | 136.20 | 4959.39 | 472000.00 | 11814333.33 | 2947 | 3878 | 2945 | 3876 |
| openrouter_anthropic | anthropic/claude-sonnet-4.6:nitro | 24 | 0 | 29517 | 36 | 29481 | 0 | 0 | 24 | 24 | 0 | 24 | 59.95 | 60.35 | 1149666.67 | 1151083.33 | 16862 | 67190 | 16861 | 67190 |
| openai | gpt-5.4-mini | 2 | 0 | 22704 | 22585 | 119 | 0 | 0 | 2 | 6 | 2 | 2 | 18.98 | 3573.46 | 59500.00 | 11352000.00 | 3208 | 3622 | 3206 | 3620 |
|  |  | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |

### Router Model Group Usage


| Model Group | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| fast | 9 | 0 | 64640 | 58373 | 6267 | 0 | 0 | 9 | 15 | 3 | 9 | 71.48 | 2325.92 | 443222.22 | 5537833.33 | 9085 | 47790 | 9084 | 47787 |
| default | 9 | 0 | 64415 | 57181 | 7234 | 0 | 0 | 9 | 17 | 4 | 9 | 43.78 | 1722.81 | 719611.11 | 5778111.11 | 11666 | 59073 | 11664 | 59071 |
| medium | 9 | 0 | 64345 | 58595 | 5750 | 0 | 0 | 9 | 15 | 3 | 9 | 75.20 | 1965.95 | 580388.89 | 5763000.00 | 8266 | 42265 | 8264 | 42264 |
| big-coder | 7 | 0 | 41324 | 33647 | 7677 | 0 | 0 | 7 | 7 | 0 | 7 | 55.44 | 1857.77 | 977047.62 | 3093857.14 | 15014 | 67190 | 15013 | 67190 |
| high | 7 | 0 | 40180 | 33593 | 6587 | 0 | 0 | 7 | 9 | 2 | 7 | 56.31 | 1944.28 | 930714.29 | 4972928.57 | 12807 | 52925 | 12805 | 52923 |
| small | 7 | 0 | 39695 | 33579 | 6116 | 0 | 0 | 7 | 9 | 1 | 7 | 47.94 | 1297.63 | 701471.43 | 3486128.57 | 13030 | 49326 | 13028 | 49325 |
|  | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |

### Internal Key Attribution


| Token ID | User | Project | Env | Caller ID | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Attempts | Fallbacks | Avg Upstream Output tok/s | Avg Downstream Output tok/s | Avg Latency ms | Max Latency ms |
|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `rtr_metrum_codex-medium_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-medium` | codex-medium | harbor-algotune-pca | case-20260614t211718z | `codex-medium-harbor-algotune-pca-case-20260614t211718z` | 6 | 0 | 60463 | 58589 | 1874 | 0 | 0 | 11 | 3 | 86.19 | 269500.00 | 3164 | 6394 |
| `rtr_metrum_codex-fast_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-fast` | codex-fast | harbor-algotune-pca | case-20260614t211718z | `codex-fast-harbor-algotune-pca-case-20260614t211718z` | 6 | 0 | 60138 | 58367 | 1771 | 0 | 0 | 11 | 3 | 85.32 | 267300.00 | 2883 | 5885 |
| `rtr_metrum_codex-default_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-default` | codex-default | harbor-algotune-pca | case-20260614t211718z | `codex-default-harbor-algotune-pca-case-20260614t211718z` | 6 | 0 | 58842 | 57175 | 1667 | 0 | 0 | 13 | 4 | 29.45 | 181900.00 | 5267 | 17860 |
| `rtr_metrum_codex-big-coder_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-big-coder` | codex-big-coder | harbor-algotune-pca | case-20260614t211718z | `codex-big-coder-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 35294 | 33641 | 1653 | 0 | 0 | 3 | 0 | 46.72 | 271777.78 | 5951 | 19922 |
| `rtr_metrum_codex-high_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-high` | codex-high | harbor-algotune-pca | case-20260614t211718z | `codex-high-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 35188 | 33587 | 1601 | 0 | 0 | 5 | 2 | 45.17 | 524833.33 | 6149 | 20635 |
| `rtr_metrum_codex-small_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-codex-small` | codex-small | harbor-algotune-pca | case-20260614t211718z | `codex-small-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 35157 | 33573 | 1584 | 0 | 0 | 5 | 1 | 37.57 | 126100.00 | 6534 | 20347 |
| `rtr_metrum_claude-code-big-coder_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-big-coder` | claude-code-big-coder | harbor-algotune-pca | case-20260614t211718z | `claude-code-big-coder-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 6030 | 6 | 6024 | 0 | 0 | 4 | 0 | 61.98 | 1506000.00 | 20325 | 67190 |
| `rtr_metrum_claude-code-default_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-default` | claude-code-default | harbor-algotune-pca | case-20260614t211718z | `claude-code-default-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 5573 | 6 | 5567 | 0 | 0 | 4 | 0 | 61.69 | 1391750.00 | 18348 | 59073 |
| `rtr_metrum_claude-code-high_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-high` | claude-code-high | harbor-algotune-pca | case-20260614t211718z | `claude-code-high-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 4992 | 6 | 4986 | 0 | 0 | 4 | 0 | 64.67 | 1235125.00 | 16263 | 52925 |
| `rtr_metrum_claude-code-small_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-small` | claude-code-small | harbor-algotune-pca | case-20260614t211718z | `claude-code-small-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 4538 | 6 | 4532 | 0 | 0 | 4 | 0 | 55.72 | 1133000.00 | 16268 | 49326 |
| `rtr_metrum_claude-code-fast_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-fast` | claude-code-fast | harbor-algotune-pca | case-20260614t211718z | `claude-code-fast-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 4502 | 6 | 4496 | 0 | 0 | 4 | 0 | 54.17 | 663125.00 | 16118 | 47790 |
| `rtr_metrum_claude-code-medium_harbor-algotune-pca_case-20260614t211718z_case-20260614t211718z-claude-code-medium` | claude-code-medium | harbor-algotune-pca | case-20260614t211718z | `claude-code-medium-harbor-algotune-pca-case-20260614t211718z` | 4 | 0 | 3882 | 6 | 3876 | 0 | 0 | 4 | 0 | 61.47 | 969000.00 | 13852 | 42265 |

### Caller IP Attribution


| Caller IP | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 69.212.113.95 | 54 | 0 | 314599 | 274968 | 39631 | 0 | 0 | 54 | 72 | 13 | 48 | 59.00 | 1871.45 | 707367.36 | 4887102.08 | 10132 | 67190 | 11396 | 67190 |

## Observations

- Both client dialects passed: Codex used the router OpenAI Responses/Codex-compatible path; Claude Code used the router Anthropic-compatible path.
- All 12 cells produced a passing `two_bucket.py` artifact and Harbor reward `1.0`.
- Production telemetry recorded 54 router requests, 72 upstream attempts, 13 fallbacks, and zero errors during the run window.
- Cache was bypassed for every request because this was an agentic tool-call workload; caching tool-call conversations would risk replaying stale tool state.
- The external model table shows the concrete upstream models selected by the router during this run. The model-group table shows how usage attributed back to internal model names.

## Artifacts

- Runner results: `examples/harbor-algotune-pca/runs/case-study-full-20260614T233205Z/results.tsv`
- Production usage report: `examples/harbor-algotune-pca/reports/case-study-full-20260614T233205Z/usage.md`
- Captured Harbor artifacts are under each job directory listed in the results table, for example `artifacts/two_bucket.py`.
