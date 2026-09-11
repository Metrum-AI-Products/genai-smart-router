# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Credential gating for real Harbor agent runs.

Missing credentials must yield a blocked disposition, never a green pass.
"""

from __future__ import annotations

import os
from typing import Mapping

# Any one of these is sufficient to attempt a real-agent Harbor cell.
REAL_AGENT_CREDENTIAL_ENV = (
    "HARBOR_ROUTER_TOKEN",
    "ROUTER_CALLER_TOKEN",
    "OPENAI_API_KEY",
    "ANTHROPIC_API_KEY",
)


def has_real_agent_credentials(env: Mapping[str, str] | None = None) -> bool:
    source = env if env is not None else os.environ
    for key in REAL_AGENT_CREDENTIAL_ENV:
        value = source.get(key, "")
        if isinstance(value, str) and value.strip():
            return True
    return False


def real_agent_run_disposition(env: Mapping[str, str] | None = None) -> str:
    """Return disposition for a real-agent Harbor trial.

    Offline verifier-integrity checks are separate and never use this path.
    """
    if has_real_agent_credentials(env):
        return "native_supported"
    return "blocked"


def blocked_evidence(*, reason: str = "missing real-agent credentials") -> dict:
    return {
        "disposition": "blocked",
        "passed": False,
        "reward": None,
        "reason": reason,
        "skip_is_pass": False,
    }
