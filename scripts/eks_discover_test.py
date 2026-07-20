#!/usr/bin/env python3
"""Regression tests for safe EKS discovery diagnostics."""

from __future__ import annotations

import importlib.util
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("eks_discover", ROOT / "scripts" / "eks_discover.py")
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def test_command_failure_never_copies_stderr() -> None:
    original = MODULE.subprocess.check_output
    MODULE.subprocess.check_output = lambda *args, **kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        subprocess.CalledProcessError(1, args[0], stderr="private-endpoint.example Authorization: Bearer test-value")
    )
    try:
        try:
            MODULE.run(["aws"])
        except MODULE.DiscoveryError as exc:
            assert str(exc) == "aws command failed"
            assert "private-endpoint" not in str(exc)
            assert "Bearer" not in str(exc)
        else:
            raise AssertionError("expected a sanitized discovery error")
    finally:
        MODULE.subprocess.check_output = original  # type: ignore[method-assign]


if __name__ == "__main__":
    test_command_failure_never_copies_stderr()
    print("EKS discovery diagnostic safeguards passed")
