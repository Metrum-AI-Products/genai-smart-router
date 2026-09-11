# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import json
from pathlib import Path

EXPECTED = {
    "call_a": "value-alpha",
    "call_b": "value-bravo",
    "call_c": "value-charlie",
}


def verify(workspace: Path) -> dict:
    path = workspace / "results.json"
    if not path.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing results.json"}
    data = json.loads(path.read_text(encoding="utf-8"))
    if data == EXPECTED:
        return {"passed": True, "reward": 1.0, "detail": "call-id map exact"}
    return {"passed": False, "reward": 0.0, "detail": f"mismatch: got={data} expected={EXPECTED}"}
