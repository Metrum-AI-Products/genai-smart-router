---
name: launch-wsh-subagent
description: Launch a Codex CLI, Claude Code, or OpenCode coding subagent in an existing Deepgram WSH terminal-server session. Use when a Tugduck-enabled project needs isolated, monitorable console automation for an agile software-development task.
---

# Launch a WSH subagent

Launch one coding agent per isolated WSH session. PM2 owns both the WSH server
daemon and the WSH client process that runs each coding agent. Run PM2 on the
same host as the named WSH server; do not use this workflow to manage a remote
server from a different host.

## Configuration

Resolve the config path in this order:

1. `TUGDUCK_CONFIG`, if set.
2. `<project-root>/.tugduck/config.json`.

On first use, Tugduck creates the project config from the plugin-level
`config.example.json`, sets its permissions to owner-only, and stops so its
placeholder values can be replaced. Keep it out of version control because it
may contain credentials. `wsh.listen_address`, `wsh.listen_port`, `wsh.token`,
and `wsh.server_name` are required; Tugduck derives the PM2-managed `wsh
server` command and HTTP health endpoint from them.
`pm2.server` declares the PM2 process name and WSH binary; `pm2.client`
declares how PM2 starts WSH clients.
`environment` is optional and is passed only to the child agent client. Use it
for project-scoped values such as `METRUM_AI_API_KEY`, `GEMINI_API_KEY`, and
`OPENAI_API_KEY` when the selected agent needs them.

WSH may bind without TLS when the configured server intentionally allows it.
Never change the bind address, token, server name, or PM2 process settings.
Never print, persist, or include token/API-key values in an agent task, chat,
logs, or source files.

## Launch workflow

1. Confirm the task is scoped, independent, and safe to run in one workspace.
   For parallel work, use a separate worktree or directory for every agent
   that could edit the same files.
2. Choose `codex`, `claude`, or `opencode`. The selected profile must be defined
   in `agents` in the config; do not guess a binary or CLI flags.
3. Preview the launch:

   ```bash
   python3 plugins/tugduck/skills/launch-wsh-subagent/scripts/launch.py \
     --project-root "$PWD" --agent codex --task "<scoped task>" --dry-run
   ```

4. If Tugduck created the config, set its values and rerun. Otherwise launch
   after reviewing the PM2 server/client commands, session name, working
   directory, and agent profile:

   ```bash
   python3 plugins/tugduck/skills/launch-wsh-subagent/scripts/launch.py \
     --project-root "$PWD" --agent codex --task "<scoped task>"
   ```

5. Monitor the PM2 namespace and the returned WSH session. Use WSH's send →
   wait → read loop; inspect prompts before granting approvals. Review the
   session scrollback and diff before accepting its work. Stop/delete the PM2
   client and destroy the WSH session when the work is done.

## Guardrails

- Launch multiple agents when requested, but give every agent a distinct
  `--session-name`. Tugduck uses that name for both the WSH session and its
  PM2-managed client process in the `tugduck` namespace.
- Use a separate worktree or directory for every concurrently editing agent.
- Do not auto-approve installs, destructive commands, credential prompts, or
  changes outside the scoped task.
- Forward `WSH_TOKEN` from `wsh.token` to the PM2-managed child-agent client,
  together with configured project API keys. Do not print these values.
- Stop and report a PM2, config, server, or session-name conflict rather than
  retrying with changed settings.
