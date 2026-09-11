# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import hashlib

CALLER = "synthetic-api-compat-caller"
DENIED_CALLER = "synthetic-api-compat-denied"
CALLER_DIGEST = hashlib.sha256(CALLER.encode()).hexdigest()
DENIED_CALLER_DIGEST = hashlib.sha256(DENIED_CALLER.encode()).hexdigest()
PROMPT_CANARY = "api-compat-prompt-canary"
TOOL_CANARY = "api-compat-tool-canary"
FORBIDDEN_ARTIFACT_VALUES = {
    CALLER,
    DENIED_CALLER,
    CALLER_DIGEST,
    DENIED_CALLER_DIGEST,
    "Authorization:",
    PROMPT_CANARY,
    TOOL_CANARY,
}

DISPOSITIONS = frozenset(
    {
        "native_supported",
        "supported_translation",
        "documented_normalization",
        "policy_rejection",
        "unsupported",
        "blocked",
    }
)
