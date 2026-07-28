#!/usr/bin/env bash
# Approved worker launch adapter. Its positional arguments are rendered only
# from the planner's name-validated profile and lease values.
set -euo pipefail

if [[ $# -ne 6 ]]; then
  echo "usage: planner_wsh_worker.sh <server-name> <session-name> <assignment-tag> <lease-tag> <worktree> <assignment-prompt>" >&2
  exit 64
fi

server_name=$1
session_name=$2
assignment_tag=$3
lease_tag=$4
worktree=$5
assignment_prompt=$6

# WSH accepts its command as one shell string. Quote the fixed supported Codex
# argv here rather than accepting profile-supplied shell text. `codex [PROMPT]`
# is the interactive TUI form; `--cd` makes its AGENTS.md discovery root exact.
printf -v codex_command '%q ' codex --cd "$worktree" "$assignment_prompt"
wsh -L "$server_name" -i --name "$session_name" --tag "$assignment_tag" --tag "$lease_tag" -c "$codex_command"
exec wsh -L "$server_name" attach "$session_name" --scrollback none
