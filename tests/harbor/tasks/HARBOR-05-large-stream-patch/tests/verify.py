# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import hashlib
import importlib.util
import sys
from pathlib import Path


def _expected_sha(workspace: Path) -> str:
    path = workspace / "stream_patch.py"
    if not path.is_file():
        path = Path(__file__).resolve().parents[1] / "environment" / "workspace" / "stream_patch.py"
    spec = importlib.util.spec_from_file_location("harbor05_expected", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules["harbor05_expected"] = module
    spec.loader.exec_module(module)
    return module.EXPECTED_SHA256


def verify(workspace: Path) -> dict:
    target = workspace / "target.txt"
    if not target.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing target.txt"}
    body = target.read_text(encoding="utf-8")
    digest = hashlib.sha256(body.encode("utf-8")).hexdigest()
    expected = _expected_sha(workspace)
    if digest != expected:
        return {
            "passed": False,
            "reward": 0.0,
            "detail": f"sha256 mismatch got={digest} expected={expected}",
        }
    if "STARTER CONTENT" in body:
        return {"passed": False, "reward": 0.0, "detail": "starter content unchanged"}
    return {"passed": True, "reward": 1.0, "detail": "large streamed patch hash ok"}
