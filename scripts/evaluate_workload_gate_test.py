#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Self-test workload gate evaluation with mock fixtures."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path


SCRIPT = Path(__file__).resolve().with_name("evaluate_workload_gate.py")


def write_json(path: Path, data: object) -> None:
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def run_gate(workdir: Path, results: list[dict[str, object]]) -> subprocess.CompletedProcess[str]:
    workdir.mkdir(parents=True, exist_ok=True)
    matrix = workdir / "matrix.json"
    result_path = workdir / "results.json"
    usage_path = workdir / "usage.json"
    out_json = workdir / "summary.json"
    out_md = workdir / "summary.md"
    write_json(
        matrix,
        {
            "name": "mock-codex-claude-gate",
            "gates": {
                "required_clients": ["codex", "claude-code"],
                "required_model_groups": ["small"],
                "min_pass_rate": 0.75,
                "min_reward": 1.0,
                "max_p95_latency_ms": 5000,
                "max_cost_per_success_usd": 0.02,
                "max_error_rate": 0.0,
                "max_fallback_rate": 1.0,
            },
        },
    )
    write_json(result_path, {"runs": results})
    write_json(
        usage_path,
        {
            "rows": [
                {
                    "client": "codex",
                    "resolvedGroup": "small",
                    "status": 200,
                    "totalCostUsd": 0.01,
                    "latencyMs": 1500,
                    "fallback": False,
                    "targetProvider": "mock",
                    "targetModel": "fast-ok",
                    "requestId": "req_codex_1",
                    "token_hash": "must-not-appear",
                },
                {
                    "client": "claude-code",
                    "resolvedGroup": "small",
                    "status": 200,
                    "totalCostUsd": 0.012,
                    "latencyMs": 1800,
                    "fallback": True,
                    "targetProvider": "mock",
                    "targetModel": "fallback-ok",
                    "requestId": "req_claude_1",
                    "authorization": "Bearer secret",
                },
            ]
        },
    )
    return subprocess.run(
        [
            sys.executable,
            str(SCRIPT),
            "--matrix",
            str(matrix),
            "--results",
            str(result_path),
            "--usage-json",
            str(usage_path),
            "--out-json",
            str(out_json),
            "--out-md",
            str(out_md),
        ],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        passing = run_gate(
            root / "pass",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "elapsed_seconds": 1.4},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "elapsed_seconds": 1.8},
            ],
        )
        require(passing.returncode == 0, f"passing fixture failed:\n{passing.stdout}\n{passing.stderr}")
        summary = json.loads((root / "pass" / "summary.json").read_text(encoding="utf-8"))
        require(summary["status"] == "pass", f"unexpected passing summary: {summary}")
        rendered = (root / "pass" / "summary.md").read_text(encoding="utf-8")
        require("must-not-appear" not in rendered and "Bearer secret" not in rendered, "secret-like fixture data leaked into markdown")
        require("mock/fallback-ok=1" in rendered, f"upstream distribution missing from markdown:\n{rendered}")

        failing = run_gate(
            root / "fail",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "failed", "reward": 0, "elapsed_seconds": 7.0, "error": "verifier-failed"},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "elapsed_seconds": 1.8},
            ],
        )
        require(failing.returncode == 1, "failing fixture returned success")
        failed_summary = json.loads((root / "fail" / "summary.json").read_text(encoding="utf-8"))
        require(failed_summary["status"] == "fail", f"unexpected failing summary: {failed_summary}")
        require(any("codex/small failed" in item for item in failed_summary["failures"]), failed_summary["failures"])
        missing_cell = run_gate(
            root / "missing-cell",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
                {"client": "claude-code", "model_group": "big", "task_id": "fixture-1", "status": "ok", "reward": 1},
            ],
        )
        require(missing_cell.returncode == 1, "missing required client/group cell returned success")
        missing_cell_summary = json.loads((root / "missing-cell" / "summary.json").read_text(encoding="utf-8"))
        require(any("claude-code/small" in item for item in missing_cell_summary["failures"]), missing_cell_summary["failures"])

        string_false = run_gate(
            root / "string-false",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "passed": "false", "reward": 1},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
            ],
        )
        require(string_false.returncode == 1, "string passed=false returned success")
        string_false_summary = json.loads((root / "string-false" / "summary.json").read_text(encoding="utf-8"))
        require(any("codex/small failed" in item for item in string_false_summary["failures"]), string_false_summary["failures"])

    print("workload gate evaluation self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
