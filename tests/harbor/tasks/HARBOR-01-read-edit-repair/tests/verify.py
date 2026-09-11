# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Independent verifier for HARBOR-01.

Pass requires:
  1. Hidden cases (from cases.json) all match.
  2. Implementation differs from the starter (catches no-edit/read-loop).
"""

from __future__ import annotations

import hashlib
import importlib.util
import json
import sys
from pathlib import Path

STARTER_HASH = None  # filled at module load from sibling starter bytes fingerprint


def _starter_fingerprint() -> str:
    # Canonical starter body fingerprint (must match environment/workspace/normalize.py).
    starter = Path(__file__).resolve().parents[1] / "environment" / "workspace" / "normalize.py"
    return hashlib.sha256(starter.read_bytes()).hexdigest()


def _load_normalize(workspace: Path):
    path = workspace / "normalize.py"
    spec = importlib.util.spec_from_file_location("harbor01_normalize", path)
    if spec is None or spec.loader is None:
        raise ImportError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules["harbor01_normalize"] = module
    spec.loader.exec_module(module)
    return module


def verify(workspace: Path) -> dict:
    cases_path = workspace / "cases.json"
    cases = json.loads(cases_path.read_text(encoding="utf-8"))["cases"]
    artifact = workspace / "normalize.py"
    if not artifact.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing normalize.py"}

    current_hash = hashlib.sha256(artifact.read_bytes()).hexdigest()
    if current_hash == _starter_fingerprint():
        return {
            "passed": False,
            "reward": 0.0,
            "detail": "no-edit/read-loop: artifact identical to starter",
        }

    try:
        mod = _load_normalize(workspace)
    except Exception as exc:  # noqa: BLE001 — verifier must classify any load failure
        return {"passed": False, "reward": 0.0, "detail": f"import failed: {exc}"}

    failures = []
    for case in cases:
        got = mod.normalize(case["input"])
        if got != case["expected"]:
            failures.append({"input": case["input"], "expected": case["expected"], "got": got})
    if failures:
        return {"passed": False, "reward": 0.0, "detail": f"case failures: {failures}"}
    return {"passed": True, "reward": 1.0, "detail": "all cases passed; differs from starter"}
