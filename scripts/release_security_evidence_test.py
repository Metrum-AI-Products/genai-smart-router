#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for release security evidence fail-closed behavior."""

from __future__ import annotations

import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def main() -> int:
    with tempfile.TemporaryDirectory() as temporary:
        result = subprocess.run(
            [
                sys.executable,
                str(ROOT / "scripts/release_security_evidence.py"),
                "--dist-dir",
                temporary,
                "--version",
                "test-rc",
                "--syft",
                "/must-not-run/syft",
                "--grype",
                "/must-not-run/grype",
            ],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    if result.returncode != 2 or "missing artifacts" not in result.stderr:
        raise AssertionError(f"incomplete release matrix did not fail closed: {result.returncode} {result.stderr}")
    print("release security evidence self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
