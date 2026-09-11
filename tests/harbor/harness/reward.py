# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

from pathlib import Path


def write_reward(path: Path, reward: float) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(f"{reward}\n", encoding="utf-8")


def read_reward(path: Path) -> float | None:
    if not path.is_file():
        return None
    text = path.read_text(encoding="utf-8").strip()
    if not text:
        return None
    return float(text)
