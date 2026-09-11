#!/usr/bin/env python3
# Copyright 2026 Metrum AI
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


def run_gate(
    workdir: Path,
    results: list[dict[str, object]],
    *,
    matrix_extra: dict[str, object] | None = None,
    promotion: bool = False,
) -> subprocess.CompletedProcess[str]:
    workdir.mkdir(parents=True, exist_ok=True)
    matrix = workdir / "matrix.json"
    result_path = workdir / "results.json"
    usage_path = workdir / "usage.json"
    out_json = workdir / "summary.json"
    out_md = workdir / "summary.md"
    matrix_body: dict[str, object] = {
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
        "run_matrix": {
            "task": "fixture-1",
            "fixed_model_controls": [],
            "attempts_per_cell": 1,
        },
    }
    if matrix_extra:
        gates = matrix_body.get("gates")
        run_matrix = matrix_body.get("run_matrix")
        if "gates" in matrix_extra and isinstance(gates, dict) and isinstance(matrix_extra["gates"], dict):
            gates.update(matrix_extra["gates"])
        if "run_matrix" in matrix_extra and isinstance(run_matrix, dict) and isinstance(matrix_extra["run_matrix"], dict):
            run_matrix.update(matrix_extra["run_matrix"])
        for key, value in matrix_extra.items():
            if key not in {"gates", "run_matrix"}:
                matrix_body[key] = value
    write_json(matrix, matrix_body)
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
    cmd = [
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
    ]
    if promotion:
        cmd.append("--promotion")
    return subprocess.run(
        cmd,
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
        require(summary.get("mode") == "smoke", summary)
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

        # EVAL-07: promotion with empty fixed_model_controls must block, not pass.
        promo_empty = run_gate(
            root / "promo-empty",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
            ],
            promotion=True,
        )
        require(promo_empty.returncode == 2, f"empty controls promotion should exit 2, got {promo_empty.returncode}")
        promo_empty_summary = json.loads((root / "promo-empty" / "summary.json").read_text(encoding="utf-8"))
        require(promo_empty_summary["status"] == "blocked", promo_empty_summary)
        require(any("fixed_model_controls" in item for item in promo_empty_summary["failures"]), promo_empty_summary["failures"])

        # Missing declared baseline blocks promotion even when dynamic cells pass.
        promo_missing_baseline = run_gate(
            root / "promo-missing-baseline",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "dynamic_router_group"},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "dynamic_router_group"},
            ],
            matrix_extra={
                "run_matrix": {
                    "fixed_model_controls": [{"model": "gpt-fixed-control", "role": "direct_provider"}],
                    "min_attempts_per_cell": 1,
                }
            },
            promotion=True,
        )
        require(promo_missing_baseline.returncode == 2, "missing baseline must block promotion")
        promo_missing_summary = json.loads((root / "promo-missing-baseline" / "summary.json").read_text(encoding="utf-8"))
        require(promo_missing_summary["status"] == "blocked", promo_missing_summary)
        require(any("baseline" in item for item in promo_missing_summary["failures"]), promo_missing_summary["failures"])

        # Sufficient controls + baseline present can pass promotion.
        promo_ok = run_gate(
            root / "promo-ok",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "dynamic_router_group"},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "dynamic_router_group"},
                {"client": "codex", "model_group": "gpt-fixed-control", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "direct_provider"},
                {"client": "claude-code", "model_group": "gpt-fixed-control", "task_id": "fixture-1", "status": "ok", "reward": 1, "control_mode": "direct_provider"},
            ],
            matrix_extra={
                "gates": {
                    "required_model_groups": ["small", "gpt-fixed-control"],
                },
                "run_matrix": {
                    "fixed_model_controls": [{"model": "gpt-fixed-control", "role": "direct_provider"}],
                    "min_attempts_per_cell": 1,
                    "task": "fixture-1",
                },
            },
            promotion=True,
        )
        require(promo_ok.returncode == 0, f"promotion with baseline should pass:\n{promo_ok.stdout}\n{promo_ok.stderr}")
        promo_ok_summary = json.loads((root / "promo-ok" / "summary.json").read_text(encoding="utf-8"))
        require(promo_ok_summary["status"] == "pass", promo_ok_summary)
        require(promo_ok_summary.get("mode") == "promotion", promo_ok_summary)

        # Insufficient sample size blocks promotion.
        promo_sample = run_gate(
            root / "promo-sample",
            [
                {"client": "codex", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
                {"client": "claude-code", "model_group": "small", "task_id": "fixture-1", "status": "ok", "reward": 1},
                {"client": "codex", "model_group": "gpt-fixed-control", "task_id": "fixture-1", "status": "ok", "reward": 1},
                {"client": "claude-code", "model_group": "gpt-fixed-control", "task_id": "fixture-1", "status": "ok", "reward": 1},
            ],
            matrix_extra={
                "gates": {"required_model_groups": ["small", "gpt-fixed-control"]},
                "run_matrix": {
                    "fixed_model_controls": ["gpt-fixed-control"],
                    "min_attempts_per_cell": 3,
                    "task": "fixture-1",
                },
            },
            promotion=True,
        )
        require(promo_sample.returncode == 2, "insufficient sample must block")
        promo_sample_summary = json.loads((root / "promo-sample" / "summary.json").read_text(encoding="utf-8"))
        require(promo_sample_summary["status"] == "blocked", promo_sample_summary)
        require(any("insufficient sample" in item for item in promo_sample_summary["failures"]), promo_sample_summary["failures"])

    print("workload gate evaluation self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
