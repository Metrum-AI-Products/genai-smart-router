# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Controlled tool error + retry simulation for HARBOR-04."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

CORRECT_ID = "record-42"
PAYLOAD = {"id": CORRECT_ID, "value": "harbor-04-ok", "attempt_ok": True}
MAX_ATTEMPTS = 3


@dataclass
class FetchSession:
    workspace: Path
    attempts: list[dict] = field(default_factory=list)

    def fetch_record(self, *, call_id: str, record_id: str) -> dict:
        attempt = {"call_id": call_id, "record_id": record_id}
        if record_id != CORRECT_ID:
            result = {
                "is_error": True,
                "error": f"unknown record_id={record_id!r}; expected {CORRECT_ID!r}",
                "output": None,
            }
        else:
            result = {"is_error": False, "error": None, "output": dict(PAYLOAD)}
            (self.workspace / "record.json").write_text(
                json.dumps(PAYLOAD, indent=2) + "\n", encoding="utf-8"
            )
        attempt.update(result)
        self.attempts.append(attempt)
        return result


def run_reference(workspace: Path) -> FetchSession:
    session = FetchSession(workspace=workspace)
    # First attempt deliberately wrong → controlled error.
    session.fetch_record(call_id="call_1", record_id="record-00")
    # Retry with corrected args.
    session.fetch_record(call_id="call_2", record_id=CORRECT_ID)
    if len(session.attempts) > MAX_ATTEMPTS:
        raise RuntimeError("exceeded attempt bound")
    return session
