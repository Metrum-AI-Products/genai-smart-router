# HARBOR-06 · Disconnect after side-effect — no replay

Call `commit_side_effect` once. After the side effect is visible in the append-only ledger, the stream disconnects.

A healthy recovery must **not** automatically replay the committed action. The local counter/ledger must show exactly one committed entry for the action id.

A separate pre-commit failure control may retry only when the side effect never became visible.
