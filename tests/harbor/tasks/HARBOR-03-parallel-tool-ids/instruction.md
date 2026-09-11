# HARBOR-03 · Parallel same-name tools, different IDs

Issue several parallel `lookup` tool calls that share the tool name but use distinct call IDs. Completions may arrive in any order.

Write `results.json` as a map from each call ID to the looked-up value. Matching by tool name or arrival order is incorrect.
