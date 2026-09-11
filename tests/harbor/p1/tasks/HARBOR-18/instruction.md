# HARBOR-18: agent version canary (new content block / tool descriptor / header)

A canary agent introduces a new content block, tool descriptor, or protocol
header (e.g. `thinking_v2` / `X-Agent-Canary: thinking_v2`). Downstream must
record an **explicit** disposition:

- `supported` — acknowledge and forward the canary (no silent drop)
- `unsupported` — clean reject without pretending the trial succeeded

Silent loss of the canary or a false-green completion must fail verification.
