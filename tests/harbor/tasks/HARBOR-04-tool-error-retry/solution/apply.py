# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


def _load_tools():
    path = Path(__file__).resolve().parents[1] / "tools" / "fetch_record.py"
    spec = importlib.util.spec_from_file_location("harbor04_tools", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules["harbor04_tools"] = module
    spec.loader.exec_module(module)
    return module


def apply(workspace: Path, mode: str) -> None:
    tools = _load_tools()
    if mode == "solution":
        session = tools.run_reference(workspace)
        (workspace / "attempts.json").write_text(
            json.dumps(session.attempts, indent=2) + "\n", encoding="utf-8"
        )
    elif mode == "wrong":
        (workspace / "record.json").write_text(
            json.dumps({"id": "record-00", "value": "still-wrong"}, indent=2) + "\n",
            encoding="utf-8",
        )
        (workspace / "attempts.json").write_text("[]\n", encoding="utf-8")
    else:
        raise ValueError(f"unsupported mode {mode!r}")
