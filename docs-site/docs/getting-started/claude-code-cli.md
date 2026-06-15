---
title: Claude Code CLI
---

# Claude Code CLI

Claude Code uses the Anthropic Messages API shape. For router traffic, use the router base URL and a router-issued bearer token.

<div class="contactBanner">
  <p>For Claude Code deployment access, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Required Environment

Use `ANTHROPIC_AUTH_TOKEN` for the router token. Do not also set `ANTHROPIC_API_KEY` for this router path; that variable is for direct Anthropic API keys and can cause client warnings or incorrect authentication behavior.

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="https://llm-api-engg.metrum.ai"
export ANTHROPIC_AUTH_TOKEN="rtr_metrum_<user>_<project>_<env>_<key>_<secret>"
```

## One-Shot Check

```bash
claude --bare --print --model big-coder \
  "Reply with exactly: router claude ok"
```

## Interactive Usage

```bash
claude --model big-coder
```

## Tool Smoke

```bash
claude --bare --print --model big-coder \
  --permission-mode bypassPermissions \
  --allowedTools "Write,Bash" \
  "Create claude_tool_smoke.txt containing exactly claude-tool-ok, run cat claude_tool_smoke.txt, then finish with claude-tool-ok."
```

The requested model must be allowed by the caller token. Standard keys can be limited to lower-cost model groups, while coding keys can allow `big-coder`.
