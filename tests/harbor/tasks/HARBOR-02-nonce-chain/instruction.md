# HARBOR-02 · Nonce chain via tool output

Complete a four-step tool chain. A fresh nonce is supplied **only** by tool output; a static canned answer cannot pass.

1. Call `lookup` with key `session` to obtain a nonce.
2. Call `compute` with that nonce to obtain a digest.
3. Call `write_result` to persist `{"nonce": ..., "digest": ...}` into `artifact.json`.
4. Call `inspect` and confirm the written artifact matches.

Tools live under `tools/` for local simulation. Preserve call IDs across the chain.
