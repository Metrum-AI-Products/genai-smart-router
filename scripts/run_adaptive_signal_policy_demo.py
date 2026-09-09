#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Run the adaptive external-policy wiring harness against a local policy process."""
from __future__ import annotations

import argparse
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


def wait_ready(url: str, timeout_s: float = 5.0) -> None:
    deadline = time.time() + timeout_s
    last_err = None
    while time.time() < deadline:
        try:
            urllib.request.urlopen(url + "/state", timeout=0.5).read()
            return
        except Exception as err:  # noqa: BLE001 - readiness probe
            last_err = err
            time.sleep(0.05)
    raise RuntimeError(f"policy service not ready at {url}: {last_err}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=18092)
    args = ap.parse_args()
    root = Path(__file__).resolve().parents[1]
    policy = root / "examples" / "external-routing-policy" / "adaptive_signal_policy.py"
    harness = root / "examples" / "external-routing-policy" / "adaptive_signal_harness.py"
    url = f"http://{args.host}:{args.port}"
    proc = subprocess.Popen(
        [sys.executable, str(policy), "--host", args.host, "--port", str(args.port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        wait_ready(url)
        result = subprocess.run(
            [sys.executable, str(harness), url],
            check=False,
        )
        return result.returncode
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except urllib.error.URLError as err:
        print(f"adaptive signal demo failed: {err}", file=sys.stderr)
        raise SystemExit(1)
