#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Compare scalar workload outcomes with a preregistered fixed-model baseline."""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "fixed-model-outcome-gate/v1"
WORKLOAD_STYLES = {"unit", "tool", "ocr"}
PREREGISTRATION_KEYS = {
    "schema_version",
    "preregistration_id",
    "workload_style",
    "case_count",
    "fixed_model_id",
    "candidate_id",
    "max_pass_rate_regression",
    "max_error_rate_increase",
    "max_latency_ratio",
    "max_cost_ratio",
}
RESULT_KEYS = {"schema_version", "preregistration_id", "arms"}
ARM_KEYS = {
    "role",
    "model_id",
    "total_cases",
    "passed_cases",
    "error_count",
    "p95_latency_ms",
    "total_cost_usd",
}


def load_object(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return value


def require_exact_keys(value: dict[str, Any], allowed: set[str], required: set[str], label: str) -> None:
    unknown = sorted(set(value) - allowed)
    missing = sorted(required - set(value))
    if unknown:
        raise ValueError(f"{label} contains unsupported fields: {', '.join(unknown)}")
    if missing:
        raise ValueError(f"{label} is missing required fields: {', '.join(missing)}")


def require_string(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must be a non-empty string")
    return value.strip()


def require_int(value: Any, label: str, minimum: int = 0) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum:
        raise ValueError(f"{label} must be an integer >= {minimum}")
    return value


def require_number(value: Any, label: str, minimum: float = 0.0) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{label} must be a number")
    number = float(value)
    if not math.isfinite(number) or number < minimum:
        raise ValueError(f"{label} must be finite and >= {minimum}")
    return number


def validate_preregistration(value: dict[str, Any]) -> dict[str, Any]:
    required = PREREGISTRATION_KEYS - {"max_latency_ratio", "max_cost_ratio"}
    require_exact_keys(value, PREREGISTRATION_KEYS, required, "preregistration")
    if value["schema_version"] != SCHEMA_VERSION:
        raise ValueError(f"unsupported preregistration schema_version {value['schema_version']!r}")
    require_string(value["preregistration_id"], "preregistration_id")
    if value["workload_style"] not in WORKLOAD_STYLES:
        raise ValueError("workload_style must be unit, tool, or ocr")
    require_int(value["case_count"], "case_count", 1)
    require_string(value["fixed_model_id"], "fixed_model_id")
    require_string(value["candidate_id"], "candidate_id")
    require_number(value["max_pass_rate_regression"], "max_pass_rate_regression")
    require_number(value["max_error_rate_increase"], "max_error_rate_increase")
    for key in ("max_latency_ratio", "max_cost_ratio"):
        if key in value:
            require_number(value[key], key)
    return value


def validate_arm(value: Any, index: int) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ValueError(f"arms[{index}] must be an object")
    require_exact_keys(value, ARM_KEYS, ARM_KEYS, f"arms[{index}]")
    if value["role"] not in {"fixed_model", "candidate"}:
        raise ValueError(f"arms[{index}].role must be fixed_model or candidate")
    require_string(value["model_id"], f"arms[{index}].model_id")
    total = require_int(value["total_cases"], f"arms[{index}].total_cases", 1)
    passed = require_int(value["passed_cases"], f"arms[{index}].passed_cases")
    errors = require_int(value["error_count"], f"arms[{index}].error_count")
    if passed > total or errors > total:
        raise ValueError(f"arms[{index}] counts cannot exceed total_cases")
    require_number(value["p95_latency_ms"], f"arms[{index}].p95_latency_ms")
    require_number(value["total_cost_usd"], f"arms[{index}].total_cost_usd")
    return value


def validate_results(value: dict[str, Any], preregistration: dict[str, Any]) -> dict[str, dict[str, Any]]:
    require_exact_keys(value, RESULT_KEYS, RESULT_KEYS, "results")
    if value["schema_version"] != SCHEMA_VERSION:
        raise ValueError(f"unsupported results schema_version {value['schema_version']!r}")
    if value["preregistration_id"] != preregistration["preregistration_id"]:
        raise ValueError("results preregistration_id does not match")
    if not isinstance(value["arms"], list) or len(value["arms"]) != 2:
        raise ValueError("results arms must contain exactly two entries")
    arms = [validate_arm(arm, index) for index, arm in enumerate(value["arms"])]
    by_role = {arm["role"]: arm for arm in arms}
    if set(by_role) != {"fixed_model", "candidate"}:
        raise ValueError("results must contain one fixed_model arm and one candidate arm")
    expected_ids = {
        "fixed_model": preregistration["fixed_model_id"],
        "candidate": preregistration["candidate_id"],
    }
    for role, expected_id in expected_ids.items():
        arm = by_role[role]
        if arm["model_id"] != expected_id:
            raise ValueError(f"{role} model_id does not match preregistration")
        if arm["total_cases"] != preregistration["case_count"]:
            raise ValueError(f"{role} total_cases does not match preregistration")
    return by_role


def ratio(candidate: float, baseline: float) -> float | None:
    if baseline == 0:
        return 1.0 if candidate == 0 else None
    return candidate / baseline


def evaluate(preregistration: dict[str, Any], arms: dict[str, dict[str, Any]]) -> dict[str, Any]:
    baseline = arms["fixed_model"]
    candidate = arms["candidate"]
    baseline_pass_rate = baseline["passed_cases"] / baseline["total_cases"]
    candidate_pass_rate = candidate["passed_cases"] / candidate["total_cases"]
    baseline_error_rate = baseline["error_count"] / baseline["total_cases"]
    candidate_error_rate = candidate["error_count"] / candidate["total_cases"]
    pass_rate_regression = baseline_pass_rate - candidate_pass_rate
    error_rate_increase = candidate_error_rate - baseline_error_rate
    latency_ratio = ratio(float(candidate["p95_latency_ms"]), float(baseline["p95_latency_ms"]))
    cost_ratio = ratio(float(candidate["total_cost_usd"]), float(baseline["total_cost_usd"]))

    failures: list[str] = []
    if pass_rate_regression > preregistration["max_pass_rate_regression"]:
        failures.append("pass_rate_regression")
    if error_rate_increase > preregistration["max_error_rate_increase"]:
        failures.append("error_rate_increase")
    if "max_latency_ratio" in preregistration and (
        latency_ratio is None or latency_ratio > preregistration["max_latency_ratio"]
    ):
        failures.append("latency_ratio")
    if "max_cost_ratio" in preregistration and (
        cost_ratio is None or cost_ratio > preregistration["max_cost_ratio"]
    ):
        failures.append("cost_ratio")

    return {
        "schema_version": SCHEMA_VERSION,
        "preregistration_id": preregistration["preregistration_id"],
        "workload_style": preregistration["workload_style"],
        "fixed_model_id": preregistration["fixed_model_id"],
        "candidate_id": preregistration["candidate_id"],
        "case_count": preregistration["case_count"],
        "baseline_pass_rate": baseline_pass_rate,
        "candidate_pass_rate": candidate_pass_rate,
        "pass_rate_regression": pass_rate_regression,
        "baseline_error_rate": baseline_error_rate,
        "candidate_error_rate": candidate_error_rate,
        "error_rate_increase": error_rate_increase,
        "latency_ratio": latency_ratio,
        "cost_ratio": cost_ratio,
        "status": "pass" if not failures else "fail",
        "failure_metrics": failures,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--preregistration", required=True, type=Path)
    parser.add_argument("--results", required=True, type=Path)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args(argv)
    try:
        preregistration = validate_preregistration(load_object(args.preregistration))
        arms = validate_results(load_object(args.results), preregistration)
        report = evaluate(preregistration, arms)
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        print(f"fixed-model outcome gate: {exc}", file=sys.stderr)
        return 2
    rendered = json.dumps(report, indent=2, sort_keys=True, allow_nan=False) + "\n"
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(rendered, encoding="utf-8")
    else:
        sys.stdout.write(rendered)
    return 0 if report["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
