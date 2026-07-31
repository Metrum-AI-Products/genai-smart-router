# Project runtime configuration selectors

This is a selector map, not a secret store. Resolve the local profile in this order: `$METRUM_AGENTS_CONFIG`; `$METRUM_AGENTS_ROOT/runtime/metrum-dev-agents.conf.json`; then `<repo>/.metrum-agents/runtime/metrum-dev-agents.conf.json`. The planner must inject the first two variables when it launches a worker from an external worktree, because that worktree may not yet contain the unmerged control-plane baseline. The profile is ignored and must be owner-readable only.

The committed nonsecret channel identity and delivery contract live in `google-workspace-chat.json`. It identifies the intended Chat space and runtime selectors; it never contains a webhook URL, API key, or token.

| Purpose | JSON selector | Required behavior |
| --- | --- | --- |
| Google Workspace Chat escalation | `workspace.google_chat_webhook_url` | Send the approved concise status/escalation. Never print or persist the URL or credentials. If delivery is non-2xx, report only the sanitized status/class to the planner and record the escalation in the tracker and linked GitHub issue. |
| Google Workspace tracker | `workspace.escalation_spreadsheet_url` | Record the durable escalation/status link when authorized. Never log a private URL. |
| WSH server identity/profile | `wsh.server_name`, `wsh.connection`, `wsh.bind_address`, `wsh.port`, `wsh.tls_mode` | Validate identity before inventory, relay, create, or close a session. Fail closed on mismatch, ambiguity, or missing required values. |
| WSH authentication | `wsh.token` | Use only through the approved client/adapter; never print, pass in prompts, or commit. |
| Planner state | `planner_controller.state_directory` | Keep observed state local and ignored beneath `.metrum-agents/runtime/`. |

The expected selector existing does not prove that a remote service accepts it. For example, an `INVALID_ARGUMENT` response is an authenticated-delivery failure to escalate and repair—not evidence that an agent should guess another URL or token. A missing selector is a provisioning defect: the planner must restart the worker with the two control-plane environment variables, not ask the worker to discover another checkout.
