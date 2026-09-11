# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Simulate large streamed write/patch argument fragments for HARBOR-05."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

# Includes escapes, unicode, and enough bulk to force multi-chunk streaming.
TARGET_BODY = (
    "line-1: hello\\nworld\n"
    "line-2: quotes \"and\" backslash\\\\\n"
    "line-3: unicode café 日本語 🚀\n"
    + ("block:" + ("x" * 64) + "\n") * 40
)

EXPECTED_SHA256 = hashlib.sha256(TARGET_BODY.encode("utf-8")).hexdigest()


def fragment_argument(payload: str, *, chunk_size: int = 37) -> list[str]:
    """Split JSON string contents into chunks that cross escape/unicode boundaries."""
    encoded = json.dumps(payload)[1:-1]  # JSON string content without surrounding quotes
    return [encoded[i : i + chunk_size] for i in range(0, len(encoded), chunk_size)]


def reassemble_argument(fragments: list[str]) -> str:
    joined = "".join(fragments)
    return json.loads(f'"{joined}"')


def apply_patch(workspace: Path, body: str) -> str:
    path = workspace / "target.txt"
    path.write_text(body, encoding="utf-8")
    return hashlib.sha256(body.encode("utf-8")).hexdigest()


def simulate_stream_write(workspace: Path) -> dict:
    fragments = fragment_argument(TARGET_BODY)
    rebuilt = reassemble_argument(fragments)
    digest = apply_patch(workspace, rebuilt)
    return {
        "fragments": len(fragments),
        "sha256": digest,
        "expected_sha256": EXPECTED_SHA256,
    }
