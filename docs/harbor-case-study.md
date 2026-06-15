# Harbor Cleaned-Model Case Study

- Case ID: `harbor-cleaned-20260615T031649Z`
- Task: `aider/polyglot_python_two-bucket`
- Hosted router: `https://llm-api-engg.metrum.ai`
- Agents: `codex`, `claude-code`
- Goal: implement `two_bucket.py` so Harbor verifier accepts the bucket-measuring algorithm and required `ValueError` behavior.
- Reward score: Harbor reports `1.0` when the submitted artifact passes the verifier for the task; `0.0` means the verifier rejected the artifact. Exceptions are tracked separately because an agent can produce a passing artifact but still exit nonzero.

## Cleaned Routing Policy

OpenRouter `moonshotai/kimi-k2.7-code:nitro` and unvalidated candidate models were removed from local and production configs. Kimi K2.7 Code remains through the direct Moonshot AI endpoint. Active tool-capable routes use MiniMax M3, direct Moonshot Kimi, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro where validated. Original OpenAI is retained as a 1% non-tool target; original Anthropic is not active because no Anthropic key is currently present in local or production env.

| Model Group | Normal Target Weights | Tool-Capable Targets |
|---|---|---|
| `default` | DeepSeek 56%, MiniMax-M3 28%, Gemma 8%, direct Kimi 7%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |
| `fast` | DeepSeek 61%, MiniMax-M3 28%, Gemma 5%, direct Kimi 5%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |
| `small` | DeepSeek 61%, MiniMax-M3 30%, Gemma 4%, direct Kimi 4%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |
| `medium` | DeepSeek 56%, MiniMax-M3 27%, Gemma 8%, direct Kimi 8%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |
| `high` | DeepSeek 51%, MiniMax-M3 28%, Gemma 10%, direct Kimi 10%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |
| `big-coder` | DeepSeek 20%, MiniMax-M3 49%, direct Kimi 30%, OpenAI GPT-5.5 1% (non-tool only) | Codex: MiniMax M3 and OpenRouter DeepSeek V4 Flash Nitro. Claude Code: MiniMax M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, and OpenRouter Gemma 4 26B Nitro. |

## Harbor Results

| Agent | Group | Status | Reward | Errors | Elapsed s | Harbor Input | Harbor Cache | Harbor Output | Job | Notes |
|---|---|---|---:|---:|---:|---:|---:|---:|---|---|
| `codex` | `default` | ok | 1 | 0 | 139 | 130360 | 80169 | 11314 | `jobs/2026-06-15__03-17-15/result.json` |  |
| `codex` | `fast` | ok | 1 | 0 | 358 | 122024 | 45589 | 23731 | `jobs/2026-06-15__03-19-35/result.json` |  |
| `codex` | `small` | ok | 1 | 0 | 156 | 145340 | 110951 | 8734 | `jobs/2026-06-15__03-25-32/result.json` |  |
| `codex` | `medium` | ok | 1 | 0 | 116 | 49735 | 34133 | 10382 | `jobs/2026-06-15__03-56-49/result.json` | clean rerun after original reward 1.0 / NonZeroAgentExitCodeError |
| `codex` | `high` | ok | 1 | 0 | 119 | 52790 | 7196 | 6519 | `jobs/2026-06-15__03-32-50/result.json` |  |
| `codex` | `big-coder` | ok | 1 | 0 | 91 | 38315 | 9650 | 4630 | `jobs/2026-06-15__03-34-48/result.json` |  |
| `claude-code` | `default` | ok | 1 | 0 | 405 | 134173 | 18162 | 25070 | `jobs/2026-06-15__03-36-20/result.json` |  |
| `claude-code` | `fast` | ok | 1 | 0 | 128 | 86152 | 37888 | 9236 | `jobs/2026-06-15__03-43-04/result.json` |  |
| `claude-code` | `small` | ok | 1 | 0 | 96 | 108230 | 39268 | 4887 | `jobs/2026-06-15__03-45-13/result.json` |  |
| `claude-code` | `medium` | ok | 1 | 0 | 131 | 85710 | 38656 | 11781 | `jobs/2026-06-15__03-46-50/result.json` |  |
| `claude-code` | `high` | ok | 1 | 0 | 267 | 85477 | 114 | 22646 | `jobs/2026-06-15__03-49-01/result.json` |  |
| `claude-code` | `big-coder` | ok | 1 | 0 | 171 | 85437 | 18162 | 15826 | `jobs/2026-06-15__03-53-28/result.json` |  |

## Scoped Router Usage

- Requests: `89`
- Errors: `1`
- Tokens: `1457139` total, `1149320` input, `178283` output
- Attempts: `86`; fallbacks: `5`; streaming requests: `82`
- Cache: `0` hits, `0` misses, `89` bypass
- Avg upstream throughput: `68.86` output tok/s, `2889.24` total tok/s
- Avg latency: `21493 ms`; max latency: `278090 ms`

### Cache Stats

| Requests | Cacheable | Hits | Misses | Bypass | Hit Rate | Bypass Rate | Latest Items | Latest Bytes | Max Bytes | Latest Occupancy | Avg Occupancy | Max Occupancy |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 89 | 0 | 0 | 0 | 89 | n/a | 100.00% | 0 | 0 | 134217728 | 0.00% | 0.00% | 0.00% |

All Harbor requests in this run were agent/tool-bearing Codex or Claude Code requests, so the router bypassed response caching by design. Tool calls can read and write files, run shell commands, and depend on container state, so reusing a cached assistant response would be unsafe even when prompts look similar.

### Usage By External Model

|Provider|Model|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|(router)|(metadata)|8|1|0|0|0|0|0|1|n/a|n/a|10|84|
|kimi_anthropic|kimi-k2.7-code|8|0|172326|39300|3490|9|1|8|26.37|3957.61|12642|55614|
|minimax|MiniMax-M3|22|0|456843|452073|4770|22|0|22|35.06|4915.38|4929|14942|
|minimax_anthropic|MiniMax-M3|4|0|93049|64476|28573|4|0|4|55.31|889.89|95888|276137|
|openai|gpt-5.5|5|0|82268|58764|23504|9|4|5|50.56|2335.98|60010|278090|
|openrouter_anthropic|deepseek/deepseek-v4-flash:nitro|9|0|235221|196113|39108|9|0|9|107.16|2932.31|32928|74085|
|openrouter_anthropic|google/gemma-4-26b-a4b-it:nitro|6|0|151315|133040|18275|6|0|6|79.37|1832.87|30973|81864|
|openrouter_responses|deepseek/deepseek-v4-flash:nitro|27|0|266117|205554|60563|27|0|27|99.28|1540.81|19905|73196|

### Usage By Model Group

|Group|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|(metadata)|7|0|0|0|0|0|0|0|n/a|n/a|0|0|
|big-coder|8|0|143710|105590|20456|9|1|8|73.68|1930.38|22925|63119|
|default|18|0|300419|246371|36384|18|0|18|62.51|3569.96|25354|276137|
|fast|11|0|241143|170288|32967|12|1|11|50.94|3366.08|36587|278090|
|high|10|0|167318|138153|29165|10|0|10|84.13|1588.67|30481|94882|
|medium|19|1|358962|274616|45690|19|1|19|88.56|2763.33|20986|73196|
|small|16|0|245587|214302|13621|18|2|16|54.21|3229.55|10443|39477|

### Usage By Agent Client

|Client|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|claude-code|27|0|651911|432929|89446|28|1|27|69.36|2689.20|35810|276137|
|codex|62|1|805228|716391|88837|58|4|55|68.61|2989.26|15258|278090|

### Usage By Internal Case Token

|User|Requested Model|Token ID|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|claude-code-big-coder|big-coder|rtr_metrum_claude-code-big-coder_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-big-coder|4|0|100765|67275|15826|4|0|4|83.51|1760.73|33218|63119|
|claude-code-default|default|rtr_metrum_claude-code-default_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-default|6|0|158745|116011|25070|6|0|6|50.26|3000.21|60997|276137|
|claude-code-fast|fast|rtr_metrum_claude-code-fast_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-fast|4|0|95388|48264|9236|4|0|4|65.28|4019.25|22404|74085|
|claude-code-high|high|rtr_metrum_claude-code-high_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-high|4|0|108009|85363|22646|4|0|4|105.16|961.79|57179|94882|
|claude-code-medium|medium|rtr_metrum_claude-code-medium_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-medium|4|0|97491|47054|11781|4|0|4|84.19|3266.55|22821|47087|
|claude-code-small|small|rtr_metrum_claude-code-small_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-claude-code-small|5|0|91513|68962|4887|6|1|5|43.75|2914.78|11682|32670|
|codex-big-coder||rtr_metrum_codex-big-coder_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-big-coder|1|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-big-coder|big-coder|rtr_metrum_codex-big-coder_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-big-coder|4|0|42945|38315|4630|5|1|4|63.84|2100.03|12632|33283|
|codex-default||rtr_metrum_codex-default_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-default|1|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-default|default|rtr_metrum_codex-default_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-default|12|0|141674|130360|11314|12|0|12|68.63|3854.83|7533|41253|
|codex-fast||rtr_metrum_codex-fast_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-fast|1|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-fast|fast|rtr_metrum_codex-fast_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-fast|7|0|145755|122024|23731|8|1|7|42.74|2992.84|44692|278090|
|codex-high||rtr_metrum_codex-high_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-high|1|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-high|high|rtr_metrum_codex-high_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-high|6|0|59309|52790|6519|6|0|6|70.10|2006.58|12682|55450|
|codex-medium||rtr_metrum_codex-medium_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-medium|2|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-medium|medium|rtr_metrum_codex-medium_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-medium|15|1|261471|227562|33909|15|1|15|89.81|2619.55|20497|73196|
|codex-small||rtr_metrum_codex-small_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-small|1|0|0|0|0|0|0|0|n/a|n/a|0|0|
|codex-small|small|rtr_metrum_codex-small_harbor-algotune-pca_harbor-cleaned-20260615t031649z_harbor-cleaned-20260615t031649z-codex-small|11|0|154074|145340|8734|12|1|11|58.96|3372.63|9880|39477|

### Usage By Caller IP

|Caller IP|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|69.212.113.95|89|1|1457139|1149320|178283|86|5|82|68.86|2889.24|21493|278090|

### Hourly Usage

|Hour UTC|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Streams|Avg Upstream Output tok/s|Avg Upstream Total tok/s|Avg Latency ms|Max Latency ms|
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|2026-06-15T03:00Z|89|1|1457139|1149320|178283|86|5|82|68.86|2889.24|21493|278090|

# Case Study #2: Go Sublist On Hosted Default Route

- Case ID: `harbor-go-sublist-default-20260615T063200Z`
- Task: `aider/polyglot_go_sublist`
- Hosted router: `https://llm-api-engg.metrum.ai`
- Agents: `codex`, `claude-code`
- Model group: `default`
- Goal: implement `/app/sublist.go` so Harbor verifier accepts `Sublist(l1, l2 []int) Relation`.
- Reward score: Harbor reports `1.0` when the submitted artifact passes the verifier.

## Go Sublist Results

| Agent | Group | Status | Reward | Errors | Elapsed s | Harbor Input | Harbor Cache | Harbor Output | Job |
|---|---|---|---:|---:|---:|---:|---:|---:|---|
| `codex` | `default` | ok | 1 | 0 | 100 | 77886 | 50807 | 2190 | `jobs/2026-06-15__06-32-23/result.json` |
| `claude-code` | `default` | ok | 1 | 0 | 116 | 172354 | 104011 | 2689 | `jobs/2026-06-15__06-34-04/result.json` |

Both final trials passed with reward `1.0` and zero Harbor exceptions.

## Go Sublist Scoped Router Usage

- Requests: `19`
- Errors: `0`
- Tokens: `189764` total, `146229` input, `4879` output
- Attempts: `16`; fallbacks: `0`; streaming requests: `16`
- Cache: `0` hits, `0` misses, `19` bypass
- Avg upstream throughput: `37.11` output tok/s, `3185.94` total tok/s
- Avg latency: `5350 ms`; max latency: `19578 ms`
- Caller IP: `69.212.113.95`

### Go Sublist Usage By Agent Token

|User|Calls|Errors|Tokens|Input|Output|Attempts|Fallbacks|Avg Upstream Output tok/s|Avg Latency ms|
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|codex-default|10|0|80076|77886|2190|8|0|41.80|4087|
|claude-code-default|9|0|109688|68343|2689|8|0|32.42|6753|

### Go Sublist Usage By External Model

|Provider|Model|Calls|Tokens|Input|Output|Attempts|Fallbacks|Avg Upstream Output tok/s|Avg Latency ms|
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
|minimax|MiniMax-M3|6|53584|52034|1550|6|0|39.48|5057|
|minimax_anthropic|MiniMax-M3|5|46770|44272|2498|5|0|38.26|10408|
|kimi_anthropic|kimi-k2.7-code|2|41042|2302|84|2|0|14.65|2965|
|openai|gpt-5.5|2|26492|25852|640|2|0|48.76|5265|
|openrouter_anthropic|google/gemma-4-26b-a4b-it:nitro|1|21876|21769|107|1|0|38.74|2805|

### Go Sublist Cost Comparison

Pricing assumptions: GPT 5.5 at `$5.00 / 1M input` and `$30.00 / 1M output`, Opus 4.8 at `$5.00 / 1M input` and `$25.00 / 1M output`, and Metrum routed benchmark at `$0.10 / 1M input` and `$0.20 / 1M output`.

| Scope | Input tokens | Output tokens | GPT 5.5 cost | Opus 4.8 cost | Metrum routed cost | Savings vs GPT 5.5 | Savings vs Opus 4.8 |
|---|---:|---:|---:|---:|---:|---:|---:|
| All runs | 146229 | 4879 | $0.88 | $0.85 | $0.016 | $0.86 / 98.22% | $0.84 / 98.17% |
| Codex CLI | 77886 | 2190 | $0.46 | $0.44 | $0.008 | $0.45 / 98.19% | $0.44 / 98.15% |
| Claude Code CLI | 68343 | 2689 | $0.42 | $0.41 | $0.007 | $0.42 / 98.25% | $0.40 / 98.20% |

### Go Sublist Cache Stats

| Requests | Cacheable | Hits | Misses | Bypass | Hit Rate | Bypass Rate | Latest Occupancy |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 19 | 0 | 0 | 0 | 19 | n/a | 100.00% | 0.00% |

The Go sublist benchmark used tool-bearing agent requests, so response caching was bypassed by design.
