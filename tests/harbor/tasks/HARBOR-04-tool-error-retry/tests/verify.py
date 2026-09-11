# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import json
from pathlib import Path

EXPECTED_ID = "record-42"
EXPECTED_VALUE = "harbor-04-ok"
MAX_ATTEMPTS = 3


def verify(workspace: Path) -> dict:
    path = workspace / "record.json"
    if not path.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing record.json"}
    data = json.loads(path.read_text(encoding="utf-8"))
    if data.get("id") != EXPECTED_ID or data.get("value") != EXPECTED_VALUE:
        return {"passed": False, "reward": 0.0, "detail": f"incorrect record: {data}"}

    attempts_path = workspace / "attempts.json"
    if attempts_path.is_file():
        attempts = json.loads(attempts_path.read_text(encoding="utf-8"))
        if len(attempts) > MAX_ATTEMPTS:
            return {"passed": False, "reward": 0.0, "detail": "unbounded retry"}
        if not any(a.get("is_error") for a in attempts):
            return {"passed": False, "reward": 0.0, "detail": "expected a controlled tool error before success"}
        if not any(not a.get("is_error") for a in attempts):
            return {"passed": False, "reward": 0.0, "detail": "missing successful retry"}

    return {"passed": True, "reward": 1.0, "detail": "error then retry succeeded"}
