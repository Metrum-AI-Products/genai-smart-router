# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""AGENT-01 pinned Harbor / agent baselines.

Required reproducibility runs must install these pins (or the exact versions
recorded in a dated evidence bundle). Never install an unbounded ``latest``
release inside a required gate.

Current-stable canary rows are reported separately and are not acceptance for
a required cell.
"""

from __future__ import annotations

# Machine-readable pins consumed by offline tests and docs.
PINNED_BASELINE: dict[str, str] = {
    "harbor": "0.13.2",
    "codex_cli": "0.153.4",
    "claude_code": "2.1.220",
    # Chat-agent probe used by coding-agent matrix mocks (shape coverage).
    "chat_agent_probe": "coding_agent_matrix.py@repo",
}

# Separately reported canary track. Values may move; they must never replace
# PINNED_BASELINE inside a required reproducibility run.
CANARY_TRACK: dict[str, str] = {
    "harbor": "current-stable (report version at canary date; not a required pin)",
    "codex_cli": "current-stable (report version at canary date; not a required pin)",
    "claude_code": "current-stable (report version at canary date; not a required pin)",
}

# Harbor task/container references used by the example case study. Digests are
# recorded when a dated evidence run captures them; the logical task ID is the
# stable offline pin.
PINNED_TASKS: dict[str, str] = {
    "primary_task": "aider/polyglot_python_two-bucket",
    "container_digest_policy": "record digest in dated evidence; never latest in required runs",
}
