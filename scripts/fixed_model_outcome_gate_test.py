#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Self-test the strict fixed-model outcome gate."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path


SCRIPT = Path(__file__).resolve().with_name("fixed_model_outcome_gate.py")


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value), encoding="utf-8")


def run_gate(root: Path, preregistration: dict[str, object], results: dict[str, object]) -> subprocess.CompletedProcess[str]:
    preregistration_path = root / "preregistration.json"
    results_path = root / "results.json"
    output_path = root / "output" / "gate.json"
    write_json(preregistration_path, preregistration)
    write_json(results_path, results)
    return subprocess.run(
        [
            sys.executable,
            str(SCRIPT),
            "--preregistration",
            str(preregistration_path),
            "--results",
            str(results_path),
            "--out",
            str(output_path),
        ],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def fixture() -> tuple[dict[str, object], dict[str, object]]:
    preregistration: dict[str, object] = {
        "schema_version": "fixed-model-outcome-gate/v1",
        "preregistration_id": "synthetic-ocr-v1",
        "workload_style": "ocr",
        "case_count": 10,
        "fixed_model_id": "fixed-model-control",
        "candidate_id": "router-candidate",
        "max_pass_rate_regression": 0.05,
        "max_error_rate_increase": 0.0,
        "max_latency_ratio": 1.25,
        "max_cost_ratio": 1.0,
    }
    results: dict[str, object] = {
        "schema_version": "fixed-model-outcome-gate/v1",
        "preregistration_id": "synthetic-ocr-v1",
        "arms": [
            {
                "role": "fixed_model",
                "model_id": "fixed-model-control",
                "total_cases": 10,
                "passed_cases": 9,
                "error_count": 0,
                "p95_latency_ms": 1000,
                "total_cost_usd": 0.1,
            },
            {
                "role": "candidate",
                "model_id": "router-candidate",
                "total_cases": 10,
                "passed_cases": 9,
                "error_count": 0,
                "p95_latency_ms": 900,
                "total_cost_usd": 0.08,
            },
        ],
    }
    return preregistration, results


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        preregistration, results = fixture()
        passing_root = root / "pass"
        passing_root.mkdir()
        passing = run_gate(passing_root, preregistration, results)
        require(passing.returncode == 0, f"passing gate failed: {passing.stderr}")
        output = json.loads((passing_root / "output" / "gate.json").read_text(encoding="utf-8"))
        require(output["status"] == "pass", f"unexpected pass output: {output}")
        scalar_types = (str, int, float, bool, type(None))
        require(
            all(isinstance(value, scalar_types) or (isinstance(value, list) and all(isinstance(item, str) for item in value)) for value in output.values()),
            f"output is not scalar-only: {output}",
        )

        regression_root = root / "regression"
        regression_root.mkdir()
        regression_results = json.loads(json.dumps(results))
        regression_results["arms"][1]["passed_cases"] = 7
        regression = run_gate(regression_root, preregistration, regression_results)
        require(regression.returncode == 1, "WE-1: quality regression returned zero")
        regression_output = json.loads((regression_root / "output" / "gate.json").read_text(encoding="utf-8"))
        require("pass_rate_regression" in regression_output["failure_metrics"], regression_output)

        strict_root = root / "strict"
        strict_root.mkdir()
        unsafe_results = json.loads(json.dumps(results))
        unsafe_results["prompt"] = "must be rejected"
        strict = run_gate(strict_root, preregistration, unsafe_results)
        require(strict.returncode == 2, "WE-2: prompt field was accepted by strict schema")
        require(not (strict_root / "output" / "gate.json").exists(), "invalid input produced an artifact")

    print("fixed-model outcome gate self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
