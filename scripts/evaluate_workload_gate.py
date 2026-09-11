#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Evaluate outcome gates for Harbor or workload-regression runs.

EVAL modes (issue #94):
- smoke (default): threshold gates only; empty fixed_model_controls allowed for examples.
- promotion: requires nonempty fixed_model_controls and present baseline cells;
  missing baseline, missing task, or insufficient sample size yields status blocked
  (not pass). Exit code 2 for blocked, 1 for fail, 0 for pass.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import statistics
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


SECRET_FIELD_HINTS = ("token", "api_key", "apikey", "authorization", "secret", "password", "hash")

OUTCOME_DIMENSIONS = (
    "protocol_pass",
    "sdk_parse_pass",
    "tool_execution_pass",
    "verifier_reward",
)

FAILURE_CLASSES = (
    "provider_model_error",
    "router_compatibility_failure",
    "agent_failure",
    "verifier_failure",
    "environment_failure",
    "skipped",
    "blocked",
)

CONTROL_MODES = (
    "direct_provider",
    "fixed_router",
    "dynamic_router_group",
)


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def load_results(path: Path) -> list[dict[str, Any]]:
    if path.suffix.lower() == ".tsv":
        with path.open("r", encoding="utf-8", newline="") as f:
            return [dict(row) for row in csv.DictReader(f, delimiter="\t")]
    raw = load_json(path)
    if isinstance(raw, dict):
        raw = raw.get("runs", raw.get("results", []))
    if not isinstance(raw, list):
        raise ValueError(f"{path} must contain a list or an object with runs/results")
    return [dict(row) for row in raw]


def load_usage(path: Path | None) -> list[dict[str, Any]]:
    if path is None:
        return []
    raw = load_json(path)
    if isinstance(raw, dict):
        raw = raw.get("rows", raw.get("requests", []))
    if not isinstance(raw, list):
        raise ValueError(f"{path} must contain a list or an object with rows/requests")
    return [dict(row) for row in raw]


def safe_row(row: dict[str, Any]) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for key, value in row.items():
        lowered = key.lower()
        if any(hint in lowered for hint in SECRET_FIELD_HINTS):
            continue
        out[key] = value
    return out


def as_float(value: Any, default: float = 0.0) -> float:
    if value in (None, ""):
        return default
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def as_int(value: Any, default: int = 0) -> int:
    if value in (None, ""):
        return default
    try:
        return int(float(value))
    except (TypeError, ValueError):
        return default


def parse_bool(value: Any, default: bool = False) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    if isinstance(value, str):
        normalized = value.strip().lower()
        if normalized in {"true", "t", "yes", "y", "1", "pass", "passed", "success", "succeeded", "ok"}:
            return True
        if normalized in {"false", "f", "no", "n", "0", "fail", "failed", "error", "errored", ""}:
            return False
    return default


def bool_passed(row: dict[str, Any]) -> bool:
    if "passed" in row:
        return parse_bool(row["passed"], False)
    status = str(row.get("status", "")).strip().lower()
    if status in {"ok", "pass", "passed", "success", "succeeded"}:
        return True
    if status in {"failed", "fail", "error", "errored", "blocked", "skipped"}:
        return False
    reward = as_float(row.get("reward"), 0.0)
    errors = as_int(row.get("errors"), 0)
    return reward >= 1.0 and errors == 0


def classify_failure(row: dict[str, Any]) -> str:
    explicit = str(row.get("failure_class", row.get("outcome_class", ""))).strip().lower()
    if explicit in FAILURE_CLASSES:
        return explicit
    status = str(row.get("status", "")).strip().lower()
    if status in {"blocked"}:
        return "blocked"
    if status in {"skipped", "skip"}:
        return "skipped"
    if bool_passed(row):
        return ""
    error_type = str(row.get("error_type", row.get("error", ""))).strip().lower()
    if "compat" in error_type or "router" in error_type:
        return "router_compatibility_failure"
    if "provider" in error_type or "upstream" in error_type or "model" in error_type:
        return "provider_model_error"
    if "verifier" in error_type or "reward" in error_type:
        return "verifier_failure"
    if "env" in error_type or "credential" in error_type or "timeout" in error_type:
        return "environment_failure"
    if error_type:
        return "agent_failure"
    return "verifier_failure" if as_float(row.get("reward"), 1.0) < 1.0 else "agent_failure"


def dimension_value(row: dict[str, Any], name: str) -> bool | None:
    if name in row:
        return parse_bool(row[name], False)
    if name == "verifier_reward":
        return bool_passed(row)
    return None


def normalize_result(row: dict[str, Any]) -> dict[str, Any]:
    safe = safe_row(row)
    elapsed_ms = as_float(safe.get("elapsed_seconds"), 0.0) * 1000
    request_ids = safe.get("request_ids", safe.get("requestId", safe.get("request_id", [])))
    if isinstance(request_ids, str):
        request_ids = [part.strip() for part in request_ids.split(",") if part.strip()]
    if not isinstance(request_ids, list):
        request_ids = []
    control_mode = str(safe.get("control_mode", safe.get("mode", ""))).strip().lower()
    dimensions = {name: dimension_value(safe, name) for name in OUTCOME_DIMENSIONS}
    return {
        "task_id": str(safe.get("task_id", safe.get("task", "unknown"))),
        "client": str(safe.get("client", safe.get("agent", "unknown"))),
        "model_group": str(safe.get("model_group", safe.get("resolved_group", safe.get("model", "unknown")))),
        "seed": str(safe.get("seed", "")),
        "attempt": str(safe.get("attempt", "")),
        "control_mode": control_mode,
        "passed": bool_passed(safe),
        "reward": as_float(safe.get("reward"), 1.0 if bool_passed(safe) else 0.0),
        "latency_ms": as_float(safe.get("latency_ms"), elapsed_ms),
        "ttft_ms": as_float(safe.get("ttft_ms"), as_float(safe.get("time_to_first_event_ms"), 0.0)),
        "cost_usd": as_float(safe.get("cost_usd"), as_float(safe.get("total_cost_usd"), 0.0)),
        "fallback_count": as_int(safe.get("fallback_count"), as_int(safe.get("fallbacks"), 0)),
        "error_type": str(safe.get("error_type", safe.get("error", ""))),
        "failure_class": classify_failure(safe),
        "selected_provider": str(safe.get("selected_provider", safe.get("provider", ""))),
        "selected_model": str(safe.get("selected_model", safe.get("model_id", ""))),
        "router_sha": str(safe.get("router_sha", safe.get("router_build", ""))),
        "config_fingerprint": str(safe.get("config_fingerprint", "")),
        "run_id": str(safe.get("run_id", safe.get("case_id", ""))),
        "trial_id": str(safe.get("trial_id", "")),
        "request_ids": request_ids,
        "dimensions": dimensions,
    }


def normalize_usage(row: dict[str, Any]) -> dict[str, Any]:
    safe = safe_row(row)
    provider = str(safe.get("targetProvider", safe.get("target_provider", safe.get("provider", ""))))
    model = str(safe.get("targetModel", safe.get("target_model", safe.get("model", ""))))
    return {
        "client": str(safe.get("client", "unknown")),
        "model_group": str(safe.get("resolvedGroup", safe.get("resolved_group", safe.get("modelGroup", safe.get("model_group", "unknown"))))),
        "status": as_int(safe.get("status"), 0),
        "cost_usd": as_float(safe.get("totalCostUsd"), as_float(safe.get("total_cost_usd"), as_float(safe.get("costUsd"), 0.0))),
        "latency_ms": as_float(safe.get("latencyMs"), as_float(safe.get("latency_ms"), 0.0)),
        "fallbacks": as_int(safe.get("fallbacks"), 1 if safe.get("fallback") is True else 0),
        "provider_model": "/".join(part for part in (provider, model) if part),
        "request_id": str(safe.get("requestId", safe.get("request_id", ""))),
    }


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    rank = math.ceil((pct / 100.0) * len(ordered)) - 1
    return ordered[max(0, min(rank, len(ordered) - 1))]


def wilson_interval(successes: int, total: int, z: float = 1.96) -> tuple[float, float]:
    if total == 0:
        return (0.0, 0.0)
    phat = successes / total
    denom = 1 + z * z / total
    center = (phat + z * z / (2 * total)) / denom
    spread = z * math.sqrt((phat * (1 - phat) + z * z / (4 * total)) / total) / denom
    return (max(0.0, center - spread), min(1.0, center + spread))


def aggregate(results: list[dict[str, Any]], usage_rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    normalized_results = [normalize_result(row) for row in results]
    normalized_usage = [normalize_usage(row) for row in usage_rows]
    usage_by_key: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for row in normalized_usage:
        usage_by_key[(row["client"], row["model_group"])].append(row)

    groups: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for row in normalized_results:
        groups[(row["client"], row["model_group"])].append(row)

    summaries: list[dict[str, Any]] = []
    for (client, model_group), rows in sorted(groups.items()):
        usage = usage_by_key.get((client, model_group), [])
        successes = sum(1 for row in rows if row["passed"])
        total = len(rows)
        result_cost = sum(row["cost_usd"] for row in rows)
        usage_cost = sum(row["cost_usd"] for row in usage)
        cost = usage_cost if usage else result_cost
        result_latencies = [row["latency_ms"] for row in rows if row["latency_ms"] > 0]
        usage_latencies = [row["latency_ms"] for row in usage if row["latency_ms"] > 0]
        latencies = usage_latencies or result_latencies
        ttfts = [row["ttft_ms"] for row in rows if row["ttft_ms"] > 0]
        fallback_count = sum(row["fallback_count"] for row in rows) + sum(row["fallbacks"] for row in usage)
        error_count = sum(1 for row in rows if row["error_type"]) + sum(1 for row in usage if row["status"] >= 400)
        failure_classes = Counter(row["failure_class"] for row in rows if row["failure_class"])
        provider_models = Counter(row["provider_model"] for row in usage if row["provider_model"])
        if not provider_models:
            provider_models = Counter(
                "/".join(part for part in (row["selected_provider"], row["selected_model"]) if part)
                for row in rows
                if row["selected_provider"] or row["selected_model"]
            )
        dim_rates: dict[str, float | None] = {}
        for name in OUTCOME_DIMENSIONS:
            values = [row["dimensions"][name] for row in rows if row["dimensions"][name] is not None]
            dim_rates[name] = (sum(1 for value in values if value) / len(values)) if values else None
        control_modes = sorted({row["control_mode"] for row in rows if row["control_mode"]})
        ci_low, ci_high = wilson_interval(successes, total)
        summaries.append(
            {
                "client": client,
                "model_group": model_group,
                "runs": total,
                "successes": successes,
                "pass_rate": successes / total if total else 0.0,
                "pass_rate_ci95": [ci_low, ci_high],
                "mean_reward": statistics.fmean(row["reward"] for row in rows) if rows else 0.0,
                "min_reward": min((row["reward"] for row in rows), default=0.0),
                "p95_latency_ms": percentile(latencies, 95),
                "p95_ttft_ms": percentile(ttfts, 95),
                "cost_usd": cost,
                "cost_per_success_usd": cost / successes if successes else math.inf,
                "error_rate": error_count / total if total else 0.0,
                "fallback_rate": fallback_count / total if total else 0.0,
                "failure_classes": dict(failure_classes),
                "dimension_pass_rates": dim_rates,
                "control_modes": control_modes,
                "selected_upstreams": dict(provider_models),
                "correlation": {
                    "request_ids": sorted(
                        {rid for row in rows for rid in row["request_ids"]}
                        | {row["request_id"] for row in usage if row["request_id"]}
                    ),
                    "run_ids": sorted({row["run_id"] for row in rows if row["run_id"]}),
                    "trial_ids": sorted({row["trial_id"] for row in rows if row["trial_id"]}),
                    "router_shas": sorted({row["router_sha"] for row in rows if row["router_sha"]}),
                    "config_fingerprints": sorted(
                        {row["config_fingerprint"] for row in rows if row["config_fingerprint"]}
                    ),
                },
                "request_ids": sorted(
                    {rid for row in rows for rid in row["request_ids"]}
                    | {row["request_id"] for row in usage if row["request_id"]}
                ),
            }
        )
    return summaries


def run_matrix(matrix: dict[str, Any]) -> dict[str, Any]:
    value = matrix.get("run_matrix")
    return value if isinstance(value, dict) else {}


def fixed_model_controls(matrix: dict[str, Any]) -> list[Any]:
    controls = run_matrix(matrix).get("fixed_model_controls", matrix.get("fixed_model_controls", []))
    if controls is None:
        return []
    if not isinstance(controls, list):
        raise ValueError("fixed_model_controls must be a list")
    return controls


def nonempty_controls(controls: list[Any]) -> list[Any]:
    out: list[Any] = []
    for item in controls:
        if item in (None, "", {}, []):
            continue
        if isinstance(item, str) and not item.strip():
            continue
        if isinstance(item, dict):
            model = str(item.get("model", item.get("model_id", item.get("id", "")))).strip()
            if not model:
                continue
        out.append(item)
    return out


def control_labels(controls: list[Any]) -> list[str]:
    labels: list[str] = []
    for item in controls:
        if isinstance(item, str):
            labels.append(item.strip())
        elif isinstance(item, dict):
            labels.append(str(item.get("model", item.get("model_id", item.get("id", "")))).strip())
        else:
            labels.append(str(item).strip())
    return [label for label in labels if label]


def evaluate_promotion_requirements(
    matrix: dict[str, Any],
    summaries: list[dict[str, Any]],
    results: list[dict[str, Any]],
) -> list[str]:
    """Return block reasons for promotion mode. Empty means promotion checks cleared."""
    blocks: list[str] = []
    matrix_run = run_matrix(matrix)
    controls = nonempty_controls(fixed_model_controls(matrix))
    if not controls:
        blocks.append(
            "promotion mode requires nonempty fixed_model_controls "
            "(example placeholders with an empty list cannot produce a promotion pass)"
        )
        return blocks

    present_groups = {row["model_group"] for row in summaries}
    present_cells = {(row["client"], row["model_group"]) for row in summaries}
    for label in control_labels(controls):
        if label not in present_groups:
            blocks.append(f"missing direct/fixed-model baseline for promotion: {label}")

    required_tasks = matrix_run.get("tasks") or matrix_run.get("required_tasks")
    if not required_tasks and matrix_run.get("task"):
        required_tasks = [matrix_run.get("task")]
    if required_tasks:
        normalized = [normalize_result(row) for row in results]
        present_tasks = {row["task_id"] for row in normalized}
        for task in required_tasks:
            task_id = str(task)
            if task_id not in present_tasks:
                blocks.append(f"missing required task for promotion: {task_id}")

    min_attempts = as_int(
        matrix_run.get("min_attempts_per_cell", matrix.get("gates", {}).get("min_attempts_per_cell")),
        0,
    )
    if min_attempts > 0:
        for row in summaries:
            if row["runs"] < min_attempts:
                blocks.append(
                    f"insufficient sample size for promotion: {row['client']}/{row['model_group']} "
                    f"runs={row['runs']} min_attempts_per_cell={min_attempts}"
                )

    # Require at least one dynamic/group cell distinct from fixed controls when declared.
    required_modes = matrix_run.get("control_modes") or []
    if required_modes:
        observed = {mode for row in summaries for mode in row.get("control_modes") or []}
        # Also accept control_mode on raw results when summaries lack them.
        if not observed:
            observed = {
                str(normalize_result(row).get("control_mode") or "")
                for row in results
                if normalize_result(row).get("control_mode")
            }
        for mode in required_modes:
            mode_s = str(mode).strip().lower()
            if mode_s and mode_s not in observed and mode_s in CONTROL_MODES:
                # Soft signal: only block when results declare control_mode at all.
                if any(normalize_result(row).get("control_mode") for row in results):
                    blocks.append(f"missing required control_mode for promotion: {mode_s}")

    if not present_cells:
        blocks.append("promotion mode has no result cells")
    return blocks


def evaluate_gates(
    matrix: dict[str, Any],
    summaries: list[dict[str, Any]],
    *,
    promotion: bool = False,
    results: list[dict[str, Any]] | None = None,
) -> tuple[str, list[str]]:
    gates = matrix.get("gates") or {}
    failures: list[str] = []
    blocks: list[str] = []
    required_clients = set(gates.get("required_clients") or [])
    required_groups = set(gates.get("required_model_groups") or [])
    required_cells = {tuple(cell) for cell in gates.get("required_cells", []) if isinstance(cell, list) and len(cell) == 2}
    present_cells = {(row["client"], row["model_group"]) for row in summaries}
    if required_clients and required_groups:
        required_cells.update((client, group) for client in required_clients for group in required_groups)
    elif required_clients:
        present_clients = {row["client"] for row in summaries}
        missing_clients = sorted(required_clients - present_clients)
        if missing_clients:
            failures.append(f"missing required clients: {', '.join(missing_clients)}")
    elif required_groups:
        present_groups = {row["model_group"] for row in summaries}
        missing_groups = sorted(required_groups - present_groups)
        if missing_groups:
            failures.append(f"missing required model groups: {', '.join(missing_groups)}")
    missing_cells = sorted(required_cells - present_cells)
    for client, group in missing_cells:
        failures.append(f"missing required client/model group cell: {client}/{group}")

    thresholds = [
        ("min_pass_rate", lambda row, value: row["pass_rate"] >= float(value), "pass_rate"),
        ("min_reward", lambda row, value: row["min_reward"] >= float(value), "min_reward"),
        ("max_p95_latency_ms", lambda row, value: row["p95_latency_ms"] <= float(value), "p95_latency_ms"),
        ("max_p95_ttft_ms", lambda row, value: row["p95_ttft_ms"] <= float(value), "p95_ttft_ms"),
        ("max_cost_per_success_usd", lambda row, value: row["cost_per_success_usd"] <= float(value), "cost_per_success_usd"),
        ("max_error_rate", lambda row, value: row["error_rate"] <= float(value), "error_rate"),
        ("max_fallback_rate", lambda row, value: row["fallback_rate"] <= float(value), "fallback_rate"),
    ]
    for row in summaries:
        label = f"{row['client']}/{row['model_group']}"
        for gate_name, predicate, metric_name in thresholds:
            if gate_name not in gates:
                continue
            if not predicate(row, gates[gate_name]):
                failures.append(f"{label} failed {gate_name}: {metric_name}={row[metric_name]:.6g}, gate={gates[gate_name]}")

    if promotion:
        blocks.extend(evaluate_promotion_requirements(matrix, summaries, results or []))

    if blocks:
        # Promotion incompleteness is blocked, not a silent pass, even if thresholds look green.
        return ("blocked", blocks + failures)
    return ("pass" if not failures else "fail", failures)


def markdown_report(matrix: dict[str, Any], status: str, failures: list[str], summaries: list[dict[str, Any]]) -> str:
    lines = [
        f"# Workload Gate: {matrix.get('name', 'unnamed')}",
        "",
        f"Status: **{status.upper()}**",
        "",
    ]
    if failures:
        heading = "Blocks" if status == "blocked" else "Failures"
        lines.extend([f"## {heading}", ""])
        lines.extend(f"- {failure}" for failure in failures)
        lines.append("")
    lines.extend(
        [
            "## Summary",
            "",
            "| Client | Model group | Runs | Pass rate | CI95 | Min reward | P95 latency ms | Cost/success USD | Error rate | Fallback rate | Upstreams |",
            "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|",
        ]
    )
    for row in summaries:
        ci = row["pass_rate_ci95"]
        upstreams = ", ".join(f"{name}={count}" for name, count in sorted(row["selected_upstreams"].items())) or "n/a"
        cost_per_success = row["cost_per_success_usd"]
        cost_text = "inf" if math.isinf(cost_per_success) else f"{cost_per_success:.6f}"
        lines.append(
            "| {client} | {group} | {runs} | {pass_rate:.3f} | {ci0:.3f}-{ci1:.3f} | {reward:.3f} | {latency:.0f} | {cost} | {error:.3f} | {fallback:.3f} | {upstreams} |".format(
                client=row["client"],
                group=row["model_group"],
                runs=row["runs"],
                pass_rate=row["pass_rate"],
                ci0=ci[0],
                ci1=ci[1],
                reward=row["min_reward"],
                latency=row["p95_latency_ms"],
                cost=cost_text,
                error=row["error_rate"],
                fallback=row["fallback_rate"],
                upstreams=upstreams,
            )
        )
    lines.extend(
        [
            "",
            "## Correlation",
            "",
            "Use run → trial → request IDs, client, model group, caller/project labels, router SHA/config fingerprint, and the run time window to join this gate with router usage reports. Raw tokens, token hashes, provider keys, prompts, images, and tool outputs are intentionally not included.",
            "",
            "## Outcome dimensions",
            "",
            "Track protocol_pass, sdk_parse_pass, tool_execution_pass, and verifier_reward separately. "
            "Harbor exceptions alone are not task success. Failure classes: "
            + ", ".join(FAILURE_CLASSES)
            + ".",
            "",
        ]
    )
    return "\n".join(lines)


def json_safe(value: Any) -> Any:
    if isinstance(value, float) and not math.isfinite(value):
        return None
    if isinstance(value, dict):
        return {key: json_safe(item) for key, item in value.items()}
    if isinstance(value, list):
        return [json_safe(item) for item in value]
    return value


def resolve_promotion(matrix: dict[str, Any], flag: bool) -> bool:
    if flag:
        return True
    mode = str(matrix.get("mode", run_matrix(matrix).get("mode", "smoke"))).strip().lower()
    return mode == "promotion"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--matrix", required=True, type=Path, help="Gate matrix JSON file")
    parser.add_argument("--results", required=True, type=Path, help="Harbor results TSV or workload results JSON")
    parser.add_argument("--usage-json", type=Path, help="Optional safe usage report JSON rows")
    parser.add_argument("--out-json", type=Path, help="Write machine-readable summary JSON")
    parser.add_argument("--out-md", type=Path, help="Write Markdown summary")
    parser.add_argument(
        "--promotion",
        action="store_true",
        help="Promotion mode: nonempty fixed_model_controls required; missing baseline blocks",
    )
    parser.add_argument("--no-fail", action="store_true", help="Return zero even when gates fail or block")
    args = parser.parse_args(argv)

    matrix = load_json(args.matrix)
    results = load_results(args.results)
    usage_rows = load_usage(args.usage_json)
    summaries = aggregate(results, usage_rows)
    promotion = resolve_promotion(matrix, args.promotion)
    status, failures = evaluate_gates(matrix, summaries, promotion=promotion, results=results)
    output = {
        "name": matrix.get("name", "unnamed"),
        "mode": "promotion" if promotion else "smoke",
        "status": status,
        "failures": failures,
        "summaries": summaries,
        "fixed_model_controls": fixed_model_controls(matrix),
        "evidence_date": matrix.get("evidence_date", run_matrix(matrix).get("evidence_date")),
    }

    if args.out_json:
        args.out_json.parent.mkdir(parents=True, exist_ok=True)
        args.out_json.write_text(json.dumps(json_safe(output), indent=2, sort_keys=True, allow_nan=False) + "\n", encoding="utf-8")
    if args.out_md:
        args.out_md.parent.mkdir(parents=True, exist_ok=True)
        args.out_md.write_text(markdown_report(matrix, status, failures, summaries), encoding="utf-8")
    if not args.out_json and not args.out_md:
        print(json.dumps(json_safe(output), indent=2, sort_keys=True, allow_nan=False))
    if args.no_fail or status == "pass":
        return 0
    if status == "blocked":
        return 2
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
