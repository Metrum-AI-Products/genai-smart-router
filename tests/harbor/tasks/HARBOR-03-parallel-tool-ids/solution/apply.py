# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


def _load_tools():
    path = Path(__file__).resolve().parents[1] / "tools" / "parallel_lookup.py"
    spec = importlib.util.spec_from_file_location("harbor03_tools", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules["harbor03_tools"] = module
    spec.loader.exec_module(module)
    return module


def apply(workspace: Path, mode: str) -> None:
    tools = _load_tools()
    target = workspace / "results.json"
    if mode == "solution":
        results = tools.execute_parallel(tools.default_requests(), seed=7)
        target.write_text(json.dumps(tools.map_by_call_id(results), indent=2, sort_keys=True) + "\n", encoding="utf-8")
    elif mode == "wrong":
        # Swapped by arrival/name confusion.
        target.write_text(
            json.dumps(
                {"call_a": "value-bravo", "call_b": "value-charlie", "call_c": "value-alpha"},
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
    else:
        raise ValueError(f"unsupported mode {mode!r}")
