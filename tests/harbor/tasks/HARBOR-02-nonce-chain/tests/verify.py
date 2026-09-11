# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Verifier for HARBOR-02: artifact digest must match sha256(harbor02:{nonce})."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path


def verify(workspace: Path) -> dict:
    path = workspace / "artifact.json"
    if not path.is_file():
        return {"passed": False, "reward": 0.0, "detail": "missing artifact.json"}
    data = json.loads(path.read_text(encoding="utf-8"))
    nonce = data.get("nonce")
    digest = data.get("digest")
    if not isinstance(nonce, str) or not isinstance(digest, str):
        return {"passed": False, "reward": 0.0, "detail": "malformed artifact"}
    if nonce in {"STATIC_CANNED_NONCE", "WRONG_NONCE"}:
        return {"passed": False, "reward": 0.0, "detail": "static/canned nonce rejected"}
    expected = hashlib.sha256(f"harbor02:{nonce}".encode()).hexdigest()
    if digest != expected:
        return {"passed": False, "reward": 0.0, "detail": "digest mismatch for nonce"}
    return {"passed": True, "reward": 1.0, "detail": "nonce chain artifact valid"}
