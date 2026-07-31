#!/usr/bin/env bash
# Approved planner adapter: creates one named/tagged WSH planner identity and
# keeps its tmux window attached to the documented interactive Codex TUI.
set -euo pipefail

if [[ $# -ne 5 ]]; then
  echo "usage: planner_wsh_planner.sh <server-name> <session-name> <planner-tag> <worktree> <assignment-prompt>" >&2
  exit 64
fi

server_name=$1
session_name=$2
planner_tag=$3
worktree=$4
assignment_prompt=$5

printf -v codex_command '%q ' codex --cd "$worktree" "$assignment_prompt"
wsh -L "$server_name" -i --name "$session_name" --tag "$planner_tag" -c "$codex_command"
exec wsh -L "$server_name" attach "$session_name" --scrollback none
