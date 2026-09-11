# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path


def _load_sim(workspace: Path | None = None):
    # Prefer staged workspace copy; fall back to task template.
    candidates = []
    if workspace is not None:
        candidates.append(workspace / "stream_patch.py")
    candidates.append(Path(__file__).resolve().parents[1] / "environment" / "workspace" / "stream_patch.py")
    for path in candidates:
        if path.is_file():
            spec = importlib.util.spec_from_file_location("harbor05_stream", path)
            module = importlib.util.module_from_spec(spec)
            assert spec and spec.loader
            sys.modules["harbor05_stream"] = module
            spec.loader.exec_module(module)
            return module
    raise FileNotFoundError("stream_patch.py not found")


def apply(workspace: Path, mode: str) -> None:
    sim = _load_sim(workspace)
    if mode == "solution":
        sim.simulate_stream_write(workspace)
    elif mode == "wrong":
        # Truncated / corrupted body.
        (workspace / "target.txt").write_text(sim.TARGET_BODY[:50] + "CORRUPT", encoding="utf-8")
    else:
        raise ValueError(f"unsupported mode {mode!r}")
