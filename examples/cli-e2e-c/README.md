# CLI E2E C Harness

This is a small C program that invokes the locally installed Claude Code CLI and Codex CLI, sends a fixed prompt, and verifies that each CLI returns an expected line.

It is intended to test the router from outside the Go process using real developer tools.

## Mock Test

```bash
make test
```

This compiles the C program and uses local mock CLI scripts. It does not call providers.

## Live Test

Start the router first, then set:

```bash
export ROUTER_BASE_URL="http://127.0.0.1:18080"
export ROUTER_TOKEN="rtr_e2e_hello_world_local"
export ROUTER_MODEL="default"
make live
```

You can test one CLI at a time:

```bash
make live-claude
make live-codex
```

Optional overrides:

```bash
export CLAUDE_BIN="claude"
export CODEX_BIN="codex"
export CLI_TIMEOUT_SECONDS=180
export ROUTER_MODEL="claude-opus-4-8"
export CODEX_WIRE_API="responses"
```

The harness does not print provider keys. Claude is configured via `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`; Codex is configured with ephemeral `-c` overrides and `METRUM_ROUTER_KEY`.
