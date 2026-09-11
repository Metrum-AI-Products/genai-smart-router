# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path


def _load_tools():
    path = Path(__file__).resolve().parents[1] / "tools" / "nonce_tools.py"
    spec = importlib.util.spec_from_file_location("harbor02_tools", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules["harbor02_tools"] = module
    spec.loader.exec_module(module)
    return module


def apply(workspace: Path, mode: str) -> None:
    tools = _load_tools()
    artifact = workspace / "artifact.json"
    if mode == "solution":
        if artifact.is_file():
            artifact.unlink()
        tools.run_reference_chain(workspace)
    elif mode == "wrong":
        artifact.write_text(
            '{\n  "nonce": "WRONG_NONCE",\n  "digest": "WRONG_DIGEST"\n}\n',
            encoding="utf-8",
        )
    else:
        raise ValueError(f"unsupported mode {mode!r}")
