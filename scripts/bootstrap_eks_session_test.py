#!/usr/bin/env python3
"""Regression tests for local EKS session file safety helpers."""

from __future__ import annotations

import configparser
import importlib.util
import stat
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("bootstrap_eks_session", ROOT / "scripts/bootstrap_eks_session.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load bootstrap session helpers")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def mode(path: Path) -> int:
    return stat.S_IMODE(path.stat().st_mode)


def main() -> int:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        config = configparser.RawConfigParser()
        config["session"] = {"aws_access_key_id": "test-id"}
        credentials = root / "credentials"
        MODULE.atomic_write_config(credentials, config)
        if mode(credentials) != 0o600:
            raise AssertionError("credential profile must be atomically created with mode 0600")
        if ".credentials." in "".join(path.name for path in root.iterdir()):
            raise AssertionError("atomic credential temporary file was not removed")

        cleanup = root / "cleanup" / "source-key.json"
        MODULE.write_cleanup_record(cleanup, "smartrouter", "AKIAEXAMPLEKEYID")
        if mode(cleanup) != 0o600:
            raise AssertionError("cleanup record must be atomically created with mode 0600")
        payload = cleanup.read_text(encoding="utf-8")
        if "AKIAEXAMPLEKEYID" not in payload or "SecretAccessKey" in payload:
            raise AssertionError("cleanup record must contain only the actionable key identifier")
        try:
            MODULE.require_no_unresolved_cleanup_record(cleanup)
        except RuntimeError:
            pass
        else:
            raise AssertionError("an unresolved cleanup record must block a retry before IAM mutation")
        try:
            MODULE.write_cleanup_record(cleanup, "smartrouter", "AKIASECONDKEYID")
        except RuntimeError:
            pass
        else:
            raise AssertionError("cleanup record writes must not overwrite an unresolved key record")
        if cleanup.read_text(encoding="utf-8") != payload:
            raise AssertionError("an unresolved cleanup record was overwritten")
        MODULE.remove_cleanup_record(cleanup, "smartrouter", "AKIAEXAMPLEKEYID")
        if cleanup.exists():
            raise AssertionError("confirmed source-key deletion must remove only its own cleanup record")
    print("EKS session bootstrap safety tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
