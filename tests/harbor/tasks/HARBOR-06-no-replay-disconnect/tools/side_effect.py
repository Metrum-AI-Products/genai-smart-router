# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Side-effect ledger + disconnect/replay controls for HARBOR-06."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class SideEffectSession:
    workspace: Path
    committed_visible: bool = False
    disconnected_after_commit: bool = False
    automatic_replays: int = 0
    events: list[dict] = field(default_factory=list)

    def _read(self) -> dict:
        path = self.workspace / "ledger.json"
        return json.loads(path.read_text(encoding="utf-8"))

    def _write(self, data: dict) -> None:
        (self.workspace / "ledger.json").write_text(
            json.dumps(data, indent=2) + "\n", encoding="utf-8"
        )

    def commit_side_effect(self, *, action_id: str) -> dict:
        data = self._read()
        data["counter"] = int(data.get("counter", 0)) + 1
        entry = {"action_id": action_id, "seq": data["counter"]}
        data.setdefault("entries", []).append(entry)
        self._write(data)
        self.committed_visible = True
        self.events.append({"type": "commit", "action_id": action_id, "seq": data["counter"]})
        return {"ok": True, "entry": entry}

    def disconnect_after_visible_commit(self) -> None:
        if not self.committed_visible:
            raise RuntimeError("disconnect-after-commit requires a visible commit")
        self.disconnected_after_commit = True
        self.events.append({"type": "disconnect_after_commit"})

    def recover(self, *, replay_committed: bool) -> dict:
        """Model post-disconnect recovery.

        replay_committed=True simulates illegal automatic replay of a committed action.
        """
        if self.disconnected_after_commit and replay_committed:
            # Illegal: automatic replay of already-visible side effect.
            self.automatic_replays += 1
            self.commit_side_effect(action_id="action-1")
            self.events.append({"type": "illegal_replay"})
            return {"ok": False, "reason": "replayed_committed_action"}
        self.events.append({"type": "recover_no_replay"})
        return {"ok": True, "reason": "no_replay"}

    def pre_commit_failure_then_retry(self, *, action_id: str) -> dict:
        """Control: failure before the side effect is visible may retry once."""
        self.events.append({"type": "pre_commit_failure", "action_id": action_id})
        # No ledger mutation yet — retry is allowed.
        return self.commit_side_effect(action_id=action_id)


def run_reference_post_commit_disconnect(workspace: Path) -> SideEffectSession:
    session = SideEffectSession(workspace=workspace)
    session.commit_side_effect(action_id="action-1")
    session.disconnect_after_visible_commit()
    session.recover(replay_committed=False)
    return session


def run_illegal_replay(workspace: Path) -> SideEffectSession:
    session = SideEffectSession(workspace=workspace)
    session.commit_side_effect(action_id="action-1")
    session.disconnect_after_visible_commit()
    session.recover(replay_committed=True)
    return session


def run_pre_commit_control(workspace: Path) -> SideEffectSession:
    session = SideEffectSession(workspace=workspace)
    session.pre_commit_failure_then_retry(action_id="action-1")
    return session
