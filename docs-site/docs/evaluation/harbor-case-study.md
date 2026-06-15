---
title: Harbor Case Study
---

# Harbor Agentic Coding Case Study

This case study evaluates model groups behind Smart LLM Router using Harbor and real coding agents. Each run used a router token scoped to one `{agent, model_group}` pair so usage and cost signals could be compared cleanly.

<div class="contactBanner">
  <p>For a benchmark tailored to your workloads, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Task

- Dataset/task: `aider/polyglot_python_two-bucket`
- Goal: implement `two_bucket.py` so the verifier accepts the bucket-measuring algorithm and required `ValueError` behavior.
- Agents: Codex CLI and Claude Code CLI.
- Hosted router endpoint: Metrum Smart LLM Router.

## Reward Score

Harbor reports `1.0` when the submitted artifact passes the verifier for the task. A score of `0.0` means the verifier rejected the artifact. Agent runtime exceptions are tracked separately because an agent can produce a passing artifact but still exit nonzero.

## Model Groups Evaluated

| Group | Intended use |
|---|---|
| `default` | General-purpose route |
| `fast` | Lower-latency, budget-aware route |
| `small` | Lower-cost route for simple work |
| `medium` | Balanced development route |
| `high` | Stronger route for complex work |
| `big-coder` | Coding-oriented agent route |

The evaluated routing policy used validated tool-capable targets from OpenRouter, MiniMax, and Moonshot/Kimi, with low-weight compatible fallback paths where configured.

## Results

| Agent | Group | Status | Reward | Errors | Elapsed s | Harbor input | Harbor cache | Harbor output |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Codex | `default` | ok | 1 | 0 | 139 | 130360 | 80169 | 11314 |
| Codex | `fast` | ok | 1 | 0 | 358 | 122024 | 45589 | 23731 |
| Codex | `small` | ok | 1 | 0 | 156 | 145340 | 110951 | 8734 |
| Codex | `medium` | ok | 1 | 0 | 116 | 49735 | 34133 | 10382 |
| Codex | `high` | ok | 1 | 0 | 119 | 52790 | 7196 | 6519 |
| Codex | `big-coder` | ok | 1 | 0 | 91 | 38315 | 9650 | 4630 |
| Claude Code | `default` | ok | 1 | 0 | 405 | 134173 | 18162 | 25070 |
| Claude Code | `fast` | ok | 1 | 0 | 128 | 86152 | 37888 | 9236 |
| Claude Code | `small` | ok | 1 | 0 | 96 | 108230 | 39268 | 4887 |
| Claude Code | `medium` | ok | 1 | 0 | 131 | 85710 | 38656 | 11781 |
| Claude Code | `high` | ok | 1 | 0 | 267 | 85477 | 114 | 22646 |
| Claude Code | `big-coder` | ok | 1 | 0 | 171 | 85437 | 18162 | 15826 |

## Router Usage Summary

| Metric | Value |
|---|---:|
| Requests | 89 |
| Errors | 1 |
| Total tokens | 1,457,139 |
| Input tokens | 1,149,320 |
| Output tokens | 178,283 |
| Attempts | 86 |
| Fallbacks | 5 |
| Streaming requests | 82 |
| Average upstream output throughput | 68.86 tok/s |
| Average upstream total throughput | 2,889.24 tok/s |
| Average latency | 21,493 ms |
| Max latency | 278,090 ms |

## Cache Behavior

| Requests | Cacheable | Hits | Misses | Bypass | Hit rate | Latest occupancy |
|---:|---:|---:|---:|---:|---:|---:|
| 89 | 0 | 0 | 0 | 89 | n/a | 0.00% |

All Harbor requests in this run were agent/tool-bearing Codex or Claude Code requests. Tool calls can read and write files, run shell commands, and depend on container state, so the router bypassed response caching by design.

## Interpretation

Every final run passed with reward `1.0`, which means the routed model groups were able to complete the coding task under both Codex CLI and Claude Code CLI. The report also shows why model-group evaluation should include more than pass/fail status: elapsed time, tokens, fallbacks, provider mix, and throughput vary materially by agent and group.
