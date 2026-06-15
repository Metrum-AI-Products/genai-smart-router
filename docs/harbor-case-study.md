# Harbor Agentic Coding Case Study

This case study records a production-hosted Smart LLM Router evaluation using Harbor with both Codex CLI and Claude Code CLI after the 2026-06-15 routing-policy update. The run validates that the router can serve OpenAI Responses-style Codex traffic and Anthropic Messages-style Claude Code traffic through the same hosted endpoint while active upstream model IDs remain limited to OpenRouter, MiniMax, and Kimi/Moonshot.

## Run Summary

- Run ID: `case-current-policy-20260615T004637Z`
- Hosted router: `https://llm-api-engg.metrum.ai`
- Harbor task: `aider/polyglot_python_two-bucket`
- Caller token case/environment: `case-current-policy-20260615t004637z`
- Agents: `codex`, `claude-code`
- Model groups: `default`, `fast`, `small`, `medium`, `high`, `big-coder`
- Matrix size: 12 cells, one Harbor trial per `{agent, model_group}`
- Result: 12/12 cells passed with Harbor reward `1.0` and zero Harbor exceptions
- Wall-clock agent time recorded by runner: 2694 seconds across all cells
- Production usage report: `examples/harbor-algotune-pca/reports/case-current-policy-20260615T004637Z/usage.md`

## Task And Goal

The Harbor task asked each agent to modify `two_bucket.py` for the classic two-bucket measuring problem. Given two bucket sizes, a target volume, and the bucket that must be filled first, the implementation must return the number of actions needed, which bucket contains the target volume, and the remaining volume in the other bucket. Impossible inputs must raise `ValueError` with a message. Harbor verified the solution with the task test suite and reported reward `1.0` when the generated implementation passed.

## Current Model Groups

Non-tool requests use the weighted targets. Tool-bearing Codex requests use the Responses-compatible tool target, and tool-bearing Claude Code requests use Anthropic-compatible MiniMax/Kimi targets. Tool requests bypass the response cache by design.

| Group | Weighted Non-Tool Targets | Tool-Compatible Targets |
|---|---|---|
| `default` | `minimax/MiniMax-M3` 48 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 96 (60.0%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 2 (1.2%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 2 (1.2%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 2 (1.2%)<br>`openrouter/inception/mercury-2` 2 (1.2%)<br>`openrouter/inclusionai/ling-2.6-flash` 2 (1.2%)<br>`openrouter/z-ai/glm-5.1:nitro` 2 (1.2%)<br>`openrouter/tencent/hy3-preview:nitro` 2 (1.2%)<br>`minimax/MiniMax-M2.7-highspeed` 1 (0.6%)<br>`kimi/kimi-k2.7-code` 1 (0.6%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |
| `fast` | `minimax/MiniMax-M3` 39 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 78 (60.0%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 2 (1.5%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 2 (1.5%)<br>`openrouter/inception/mercury-2` 2 (1.5%)<br>`openrouter/inclusionai/ling-2.6-flash` 2 (1.5%)<br>`minimax/MiniMax-M2.7-highspeed` 3 (2.3%)<br>`kimi/kimi-k2.7-code` 2 (1.5%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |
| `small` | `minimax/MiniMax-M3` 30 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 60 (60.0%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 2 (2.0%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 2 (2.0%)<br>`openrouter/inception/mercury-2` 2 (2.0%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (1.0%)<br>`minimax/MiniMax-M2.7-highspeed` 2 (2.0%)<br>`kimi/kimi-k2.7-code` 1 (1.0%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |
| `medium` | `minimax/MiniMax-M3` 39 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 78 (60.0%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 2 (1.5%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 2 (1.5%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 2 (1.5%)<br>`openrouter/inception/mercury-2` 2 (1.5%)<br>`openrouter/inclusionai/ling-2.6-flash` 1 (0.8%)<br>`openrouter/z-ai/glm-5.1:nitro` 2 (1.5%)<br>`kimi/kimi-k2.7-code` 2 (1.5%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |
| `high` | `minimax/MiniMax-M3` 42 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 84 (60.0%)<br>`openrouter/kwaipilot/kat-coder-pro-v2:nitro` 2 (1.4%)<br>`openrouter/nvidia/nemotron-3-nano-30b-a3b` 2 (1.4%)<br>`openrouter/inception/mercury-2` 2 (1.4%)<br>`openrouter/inclusionai/ling-2.6-flash` 2 (1.4%)<br>`openrouter/z-ai/glm-5.1:nitro` 2 (1.4%)<br>`openrouter/qwen/qwen3.6-flash:nitro` 2 (1.4%)<br>`openrouter/tencent/hy3-preview:nitro` 1 (0.7%)<br>`kimi/kimi-k2.7-code` 1 (0.7%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |
| `big-coder` | `minimax/MiniMax-M3` 50 (50.0%)<br>`kimi/kimi-k2.7-code` 30 (30.0%)<br>`openrouter/deepseek/deepseek-v4-flash:nitro` 20 (20.0%) | `minimax/MiniMax-M3` (openai-responses)<br>`minimax_anthropic/MiniMax-M3` (anthropic)<br>`kimi_anthropic/kimi-k2.7-code` (anthropic) |

## Harbor Results

| Agent | Model Group | Status | Elapsed Seconds | Reward | Errors | Job Result |
|---|---|---|---:|---:|---:|---|
| `codex` | `default` | ok | 165 | 1 | 0 | `jobs/2026-06-15__00-47-31/result.json` |
| `codex` | `fast` | ok | 125 | 1 | 0 | `jobs/2026-06-15__00-50-16/result.json` |
| `codex` | `small` | ok | 210 | 1 | 0 | `jobs/2026-06-15__00-52-20/result.json` |
| `codex` | `medium` | ok | 79 | 1 | 0 | `jobs/2026-06-15__00-55-50/result.json` |
| `codex` | `high` | ok | 89 | 1 | 0 | `jobs/2026-06-15__00-57-10/result.json` |
| `codex` | `big-coder` | ok | 201 | 1 | 0 | `jobs/2026-06-15__00-58-39/result.json` |
| `claude-code` | `default` | ok | 313 | 1 | 0 | `jobs/2026-06-15__01-01-59/result.json` |
| `claude-code` | `fast` | ok | 440 | 1 | 0 | `jobs/2026-06-15__01-07-12/result.json` |
| `claude-code` | `small` | ok | 278 | 1 | 0 | `jobs/2026-06-15__01-14-33/result.json` |
| `claude-code` | `medium` | ok | 108 | 1 | 0 | `jobs/2026-06-15__01-19-11/result.json` |
| `claude-code` | `high` | ok | 433 | 1 | 0 | `jobs/2026-06-15__01-20-59/result.json` |
| `claude-code` | `big-coder` | ok | 253 | 1 | 0 | `jobs/2026-06-15__01-28-11/result.json` |

## Measurement Highlights

- Requests: `121`
- Errors: `1`
- Tokens: `1682613` total, `1105777` input, `111172` output
- Cache: `0` hits, `0` misses, `121` bypass
- Upstream attempts: `115`; fallbacks: `1`; streaming requests: `114`
- Latency: `15727 ms` avg, `264658 ms` max
- Throughput: upstream `54.26` output tok/s / `3080.14` total tok/s; downstream `790700.29` output tok/s / `11954589.23` total tok/s
- One upstream `502` occurred during `claude-code/high` on `minimax_anthropic/MiniMax-M3`; the router logged one fallback and the Harbor cell still passed with reward `1.0`.
- Cache hits and misses are zero because all agent/tool-bearing requests bypass the response cache.

### Usage By External Model

| Provider | Model | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| minimax | MiniMax-M3 | 67 | 0 | 894058 | 852511 | 41547 | 0 | 0 | 67 | 67 | 0 | 67 | 61.19 | 3112.19 | 304240.80 | 8403135.57 | 8220 | 83368 | 8217 | 83365 |
| kimi_anthropic | kimi-k2.7-code | 22 | 0 | 527309 | 38071 | 23574 | 0 | 0 | 22 | 22 | 0 | 22 | 36.01 | 4891.11 | 1071545.45 | 23968590.91 | 22241 | 113653 | 22240 | 113652 |
| minimax_anthropic | MiniMax-M3 | 25 | 1 | 261246 | 215195 | 46051 | 0 | 0 | 25 | 26 | 1 | 25 | 51.66 | 1330.63 | 1891291.67 | 10856229.17 | 34520 | 264658 | 24930 | 254254 |
|  |  | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 7 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |

### Usage By Router Model Group

| Model Group | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| fast | 30 | 0 | 460173 | 231644 | 15281 | 0 | 0 | 30 | 30 | 0 | 30 | 57.60 | 3078.20 | 394104.44 | 13899950.00 | 7911 | 25727 | 7910 | 25726 |
| small | 28 | 0 | 427687 | 358430 | 30089 | 0 | 0 | 28 | 28 | 0 | 28 | 61.20 | 3301.11 | 829375.00 | 10572630.95 | 14021 | 157768 | 14019 | 157767 |
| big-coder | 16 | 0 | 288516 | 187456 | 21956 | 0 | 0 | 16 | 16 | 0 | 16 | 53.58 | 3144.74 | 1001218.75 | 14402937.50 | 22879 | 113653 | 22877 | 113652 |
| default | 17 | 0 | 204324 | 157612 | 29048 | 0 | 0 | 17 | 17 | 0 | 17 | 50.66 | 2266.17 | 1561200.98 | 9285323.53 | 21308 | 254256 | 21306 | 254254 |
| high | 11 | 1 | 151001 | 82999 | 9890 | 0 | 0 | 11 | 12 | 1 | 11 | 54.61 | 2940.23 | 775491.67 | 11150158.33 | 40070 | 264658 | 17610 | 86080 |
| medium | 12 | 0 | 150912 | 87636 | 4908 | 0 | 0 | 12 | 12 | 0 | 12 | 35.50 | 3753.00 | 332388.89 | 11503111.11 | 8670 | 54256 | 8668 | 54255 |
|  | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 7 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |

### Usage By Client

| Client | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| codex | 73 | 0 | 894058 | 852511 | 41547 | 0 | 0 | 73 | 67 | 0 | 67 | 61.19 | 3112.19 | 304240.80 | 8403135.57 | 7544 | 83368 | 8217 | 83365 |
| claude-code | 47 | 1 | 788555 | 253266 | 69625 | 0 | 0 | 47 | 48 | 1 | 47 | 44.18 | 3033.47 | 1499239.13 | 17127358.70 | 28772 | 264658 | 23644 | 254254 |
| curl/8.5.0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |

### Usage By Status

| Status | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 200 | 120 | 0 | 1682613 | 1105777 | 111172 | 0 | 0 | 120 | 113 | 0 | 113 | 54.26 | 3080.14 | 790700.29 | 11954589.23 | 13653 | 254256 | 14497 | 254254 |
| 502 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | 2 | 1 | 1 | n/a | n/a | n/a | n/a | 264658 | 264658 | 0 | 0 |

## Artifacts

- Runner results: `examples/harbor-algotune-pca/runs/case-current-policy-20260615T004637Z/results.tsv`
- Production usage report: `examples/harbor-algotune-pca/reports/case-current-policy-20260615T004637Z/usage.md`
- Raw Harbor job outputs live under the local Harbor `jobs/` directory and are not committed.
