# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


def _load_tools():
    path = Path(__file__).resolve().parents[1] / "tools" / "side_effect.py"
    spec = importlib.util.spec_from_file_location("harbor06_tools", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules["harbor06_tools"] = module
    spec.loader.exec_module(module)
    return module


def apply(workspace: Path, mode: str) -> None:
    tools = _load_tools()
    # Reset ledger to starter state before applying.
    (workspace / "ledger.json").write_text(
        json.dumps({"counter": 0, "entries": []}, indent=2) + "\n", encoding="utf-8"
    )
    if mode == "solution":
        session = tools.run_reference_post_commit_disconnect(workspace)
        (workspace / "session.json").write_text(
            json.dumps(
                {
                    "automatic_replays": session.automatic_replays,
                    "disconnected_after_commit": session.disconnected_after_commit,
                    "events": session.events,
                    "mode": "post_commit_no_replay",
                },
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
    elif mode == "wrong":
        session = tools.run_illegal_replay(workspace)
        (workspace / "session.json").write_text(
            json.dumps(
                {
                    "automatic_replays": session.automatic_replays,
                    "disconnected_after_commit": session.disconnected_after_commit,
                    "events": session.events,
                    "mode": "illegal_replay",
                },
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
    else:
        raise ValueError(f"unsupported mode {mode!r}")
