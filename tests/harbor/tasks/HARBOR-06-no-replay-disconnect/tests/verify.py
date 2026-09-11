# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import json
from pathlib import Path


def verify(workspace: Path) -> dict:
    ledger_path = workspace / "ledger.json"
    if not ledger_path.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing ledger.json"}
    ledger = json.loads(ledger_path.read_text(encoding="utf-8"))
    entries = ledger.get("entries") or []
    action_entries = [e for e in entries if e.get("action_id") == "action-1"]
    if len(action_entries) != 1:
        return {
            "passed": False,
            "reward": 0.0,
            "detail": f"expected exactly one committed action-1, got {len(action_entries)}",
        }
    if int(ledger.get("counter", 0)) != 1:
        return {
            "passed": False,
            "reward": 0.0,
            "detail": f"counter expected 1, got {ledger.get('counter')}",
        }

    session_path = workspace / "session.json"
    if session_path.is_file():
        session = json.loads(session_path.read_text(encoding="utf-8"))
        if int(session.get("automatic_replays", 0)) != 0:
            return {"passed": False, "reward": 0.0, "detail": "automatic replay of committed action"}
        if not session.get("disconnected_after_commit"):
            return {"passed": False, "reward": 0.0, "detail": "expected disconnect after visible commit"}

    return {"passed": True, "reward": 1.0, "detail": "no replay after post-commit disconnect"}
