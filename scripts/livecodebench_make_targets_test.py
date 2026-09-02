#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Hermetic regression coverage for LiveCodeBench Make target path handling."""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def need(ok: bool, message: str) -> None:
    if not ok:
        raise AssertionError(message)


def invoke(target: str, *, lcb_root: Path, command_file: Path | None, relative: bool, probe: Path, log: Path) -> dict[str, object]:
    root_value = str(lcb_root.relative_to(ROOT)) if relative else str(lcb_root)
    command_value = str(command_file.relative_to(ROOT)) if relative and command_file else str(command_file or "")
    environment = os.environ | {"LCB_TARGET_TEST_LOG": str(log)}
    command = [
        "make", "--no-print-directory", target,
        f"LCB_ROOT={root_value}",
        f"LCB_PYTHON={sys.executable} {probe}",
    ]
    if command_file:
        command.append(f"LCB_RUNNER_COMMAND_FILE={command_value}")
    subprocess.run(command, cwd=ROOT, env=environment, check=True, capture_output=True, text=True)
    return json.loads(log.read_text(encoding="utf-8"))


def assert_invocation(record: dict[str, object], *, command: str, lcb_root: Path, runner: Path | None) -> None:
    arguments = record["arguments"]
    need(record["cwd"] == str(lcb_root), f"{command} did not change into the absolute checkout")
    need(record["pythonpath"].split(":", 1)[0] == str(lcb_root), f"{command} did not put the absolute checkout first on PYTHONPATH")
    need(arguments[:3] == [str(ROOT / "scripts/livecodebench_eval.py"), command, "--lcb-root"], f"{command} evaluator invocation changed")
    need(arguments[3] == str(lcb_root), f"{command} passed a non-absolute LCB root")
    if runner is not None:
        need(arguments[4:] == ["--runner-command-file", str(runner)], "run passed a non-absolute runner command path")


def main() -> int:
    with tempfile.TemporaryDirectory(dir=ROOT, prefix=".livecodebench-make-test-") as temporary:
        directory = Path(temporary)
        lcb_root = directory / "official-checkout"
        lcb_root.mkdir()
        runner = directory / "protected-runner"
        runner.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        runner.chmod(0o600)
        probe = directory / "probe.py"
        probe.write_text(
            "import json, os, sys\n"
            "from pathlib import Path\n"
            "Path(os.environ['LCB_TARGET_TEST_LOG']).write_text(json.dumps({'cwd': os.getcwd(), 'arguments': sys.argv[1:], 'pythonpath': os.environ['PYTHONPATH']}), encoding='utf-8')\n",
            encoding="utf-8",
        )
        cases = (
            ("livecodebench-validate", None, True),
            ("livecodebench-validate", None, False),
            ("livecodebench-run", runner, True),
            ("livecodebench-run", runner, False),
        )
        for index, (target, command_file, relative) in enumerate(cases):
            log = directory / f"invocation-{index}.json"
            record = invoke(target, lcb_root=lcb_root, command_file=command_file, relative=relative, probe=probe, log=log)
            assert_invocation(record, command="run" if target.endswith("run") else "validate", lcb_root=lcb_root, runner=command_file)
    print("livecodebench Make target path tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
