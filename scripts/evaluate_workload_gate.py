#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Evaluate outcome gates for Harbor or workload-regression runs."""

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
    if status in {"failed", "fail", "error", "errored"}:
        return False
    reward = as_float(row.get("reward"), 0.0)
    errors = as_int(row.get("errors"), 0)
    return reward >= 1.0 and errors == 0


def normalize_result(row: dict[str, Any]) -> dict[str, Any]:
    safe = safe_row(row)
    elapsed_ms = as_float(safe.get("elapsed_seconds"), 0.0) * 1000
    request_ids = safe.get("request_ids", safe.get("requestId", safe.get("request_id", [])))
    if isinstance(request_ids, str):
        request_ids = [part.strip() for part in request_ids.split(",") if part.strip()]
    if not isinstance(request_ids, list):
        request_ids = []
    return {
        "task_id": str(safe.get("task_id", safe.get("task", "unknown"))),
        "client": str(safe.get("client", safe.get("agent", "unknown"))),
        "model_group": str(safe.get("model_group", safe.get("resolved_group", safe.get("model", "unknown")))),
        "seed": str(safe.get("seed", "")),
        "attempt": str(safe.get("attempt", "")),
        "passed": bool_passed(safe),
        "reward": as_float(safe.get("reward"), 1.0 if bool_passed(safe) else 0.0),
        "latency_ms": as_float(safe.get("latency_ms"), elapsed_ms),
        "cost_usd": as_float(safe.get("cost_usd"), as_float(safe.get("total_cost_usd"), 0.0)),
        "fallback_count": as_int(safe.get("fallback_count"), as_int(safe.get("fallbacks"), 0)),
        "error_type": str(safe.get("error_type", safe.get("error", ""))),
        "selected_provider": str(safe.get("selected_provider", safe.get("provider", ""))),
        "selected_model": str(safe.get("selected_model", safe.get("model_id", ""))),
        "request_ids": request_ids,
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
        fallback_count = sum(row["fallback_count"] for row in rows) + sum(row["fallbacks"] for row in usage)
        error_count = sum(1 for row in rows if row["error_type"]) + sum(1 for row in usage if row["status"] >= 400)
        provider_models = Counter(row["provider_model"] for row in usage if row["provider_model"])
        if not provider_models:
            provider_models = Counter(
                "/".join(part for part in (row["selected_provider"], row["selected_model"]) if part)
                for row in rows
                if row["selected_provider"] or row["selected_model"]
            )
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
                "cost_usd": cost,
                "cost_per_success_usd": cost / successes if successes else math.inf,
                "error_rate": error_count / total if total else 0.0,
                "fallback_rate": fallback_count / total if total else 0.0,
                "selected_upstreams": dict(provider_models),
                "request_ids": sorted({rid for row in rows for rid in row["request_ids"]} | {row["request_id"] for row in usage if row["request_id"]}),
            }
        )
    return summaries


def evaluate_gates(matrix: dict[str, Any], summaries: list[dict[str, Any]]) -> tuple[str, list[str]]:
    gates = matrix.get("gates") or {}
    failures: list[str] = []
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
    return ("pass" if not failures else "fail", failures)


def markdown_report(matrix: dict[str, Any], status: str, failures: list[str], summaries: list[dict[str, Any]]) -> str:
    lines = [
        f"# Workload Gate: {matrix.get('name', 'unnamed')}",
        "",
        f"Status: **{status.upper()}**",
        "",
    ]
    if failures:
        lines.extend(["## Failures", ""])
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
            "Use request IDs, client, model group, caller/project labels, and the run time window to join this gate with router usage reports. Raw tokens, token hashes, provider keys, prompts, images, and tool outputs are intentionally not included.",
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


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--matrix", required=True, type=Path, help="Gate matrix JSON file")
    parser.add_argument("--results", required=True, type=Path, help="Harbor results TSV or workload results JSON")
    parser.add_argument("--usage-json", type=Path, help="Optional safe usage report JSON rows")
    parser.add_argument("--out-json", type=Path, help="Write machine-readable summary JSON")
    parser.add_argument("--out-md", type=Path, help="Write Markdown summary")
    parser.add_argument("--no-fail", action="store_true", help="Return zero even when gates fail")
    args = parser.parse_args(argv)

    matrix = load_json(args.matrix)
    results = load_results(args.results)
    usage_rows = load_usage(args.usage_json)
    summaries = aggregate(results, usage_rows)
    status, failures = evaluate_gates(matrix, summaries)
    output = {
        "name": matrix.get("name", "unnamed"),
        "status": status,
        "failures": failures,
        "summaries": summaries,
    }

    if args.out_json:
        args.out_json.parent.mkdir(parents=True, exist_ok=True)
        args.out_json.write_text(json.dumps(json_safe(output), indent=2, sort_keys=True, allow_nan=False) + "\n", encoding="utf-8")
    if args.out_md:
        args.out_md.parent.mkdir(parents=True, exist_ok=True)
        args.out_md.write_text(markdown_report(matrix, status, failures, summaries), encoding="utf-8")
    if not args.out_json and not args.out_md:
        print(json.dumps(json_safe(output), indent=2, sort_keys=True, allow_nan=False))
    return 0 if status == "pass" or args.no_fail else 1


if __name__ == "__main__":
    raise SystemExit(main())
