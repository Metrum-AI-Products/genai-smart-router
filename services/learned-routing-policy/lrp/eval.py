# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Holdout-only policy replay, cost/quality baselines and explicit promotion gates."""

from __future__ import annotations

import html
import math
import random
from collections.abc import Callable
from pathlib import Path
from typing import Any

import numpy as np

from lrp.bundle import ModelBundle, canonical_json, load_bundle, target_key
from lrp.features import FEATURE_NAMES, as_dict, preflight_file, write_private
from lrp.policy import Prediction, decide, estimated_cost
from lrp.schemas import GroupConfig, Target, TargetKey
from lrp.train import feature_frame, index_rows, trusted_judgment


def _finite(value: Any) -> float | None:
    if value is None:
        return None
    number = float(value)
    return number if math.isfinite(number) and number >= 0 else None


def _response_cost(response: dict[str, Any], field: str) -> float | None:
    attempts = response.get("attempts")
    if not attempts:
        return _finite(response.get(field))
    values = [_finite(attempt.get(field)) for attempt in attempts]
    return (
        sum(value for value in values if value is not None)
        if all(value is not None for value in values)
        else None
    )


def _metrics(samples: list[dict[str, Any]], floor: float) -> dict[str, Any]:
    quality = [s["quality"] for s in samples if s["quality"] is not None]
    costs = [s["cost"] for s in samples if s["cost"] is not None]
    regrets = [
        s["cost"] - s["oracle_cost"]
        for s in samples
        if s["cost"] is not None and s["oracle_cost"] is not None
    ]
    durations = [s["duration_ms"] for s in samples if s.get("duration_ms") is not None]
    ttfbs = [s["ttfb_ms"] for s in samples if s.get("ttfb_ms") is not None]
    methods = sorted({s["method"] for s in samples if s["quality"] is not None})
    return {
        "n": len(samples),
        "quality_observed": len(quality),
        "cost_observed": len(costs),
        "quality_mean": float(np.mean(quality)) if quality else None,
        "floor_violation_rate": float(np.mean(np.asarray(quality) < floor))
        if quality
        else None,
        "cost_total_usd": float(sum(costs))
        if len(costs) == len(samples) and samples
        else None,
        "known_cost_total_usd": float(sum(costs)),
        "cost_per_1k": float(sum(costs) / len(samples) * 1000)
        if len(costs) == len(samples) and samples
        else None,
        "regret_vs_oracle": float(np.mean(regrets)) if regrets else None,
        "regret_observed": len(regrets),
        "duration_p95_ms": float(np.percentile(durations, 95)) if durations else None,
        "ttfb_p95_ms": float(np.percentile(ttfbs, 95)) if ttfbs else None,
        "billed_cost_total_usd": sum(
            s["billed_cost"] for s in samples if s.get("billed_cost") is not None
        ),
        "billed_cost_observed": sum(s.get("billed_cost") is not None for s in samples),
        "quality_by_method": {
            method: {
                "n": sum(
                    s["method"] == method and s["quality"] is not None for s in samples
                ),
                "quality_mean": float(
                    np.mean(
                        [
                            s["quality"]
                            for s in samples
                            if s["method"] == method and s["quality"] is not None
                        ]
                    )
                ),
            }
            for method in methods
        },
    }


def _gate_comparison(
    a: Any, b: Any, comparison: Callable[[float, float], bool]
) -> bool:
    return a is not None and b is not None and comparison(float(a), float(b))


def evaluate(
    bundle: ModelBundle | str | Path,
    features: Any,
    judgments: Any,
    responses: Any,
    config: Any,
    *,
    decide_fn: Callable[..., Any] | None = None,
    split: str = "test",
    seed: int = 42,
    out: str | Path | None = None,
) -> dict[str, Any]:
    from scipy.stats import spearmanr
    from sklearn.metrics import roc_auc_score

    if split != "test":
        raise ValueError("promotion evaluation requires the test holdout")
    if out is not None:
        destination = preflight_file(out)
        preflight_file(destination.with_suffix(".md"))
        preflight_file(destination.with_suffix(".svg"))
    model = load_bundle(bundle) if isinstance(bundle, (str, Path)) else bundle
    frame = feature_frame(features, int(model.manifest["seed"]))
    if set(frame.embedding_fingerprint) != {model.manifest["embedding_fingerprint"]}:
        raise ValueError("holdout embedding differs from bundle")
    holdout = frame.loc[frame.split == "test"]
    judgment_index, response_index = (
        index_rows(judgments, "judgment"),
        index_rows(responses, "response"),
    )
    response_keys: dict[str, list[TargetKey]] = {}
    for request_id, key in response_index:
        response_keys.setdefault(request_id, []).append(key)
    cfg_data = as_dict(config)
    group_configs = cfg_data.get("groups", {})
    policy = decide_fn or decide
    configured_anchor = cfg_data.get("anchor") or model.manifest.get("anchor")
    anchor = (
        target_key(configured_anchor)
        if isinstance(configured_anchor, dict)
        else (
            tuple(configured_anchor)
            if isinstance(configured_anchor, (tuple, list))
            else None
        )
    )
    catalog = {target_key(as_dict(t)): as_dict(t) for t in cfg_data.get("targets", [])}
    active_keys = set(model.models)
    per_target: dict[TargetKey, list[dict[str, Any]]] = {key: [] for key in active_keys}
    data_by_group: dict[str, list[dict[str, Any]]] = {}
    synthetic = (
        bool(model.manifest.get("synthetic"))
        or bool(frame.synthetic.any())
        or any(
            row.get("source") == "synthetic"
            for row in [*judgment_index.values(), *response_index.values()]
        )
    )
    uncertain = 0
    for _, row in holdout.iterrows():
        request_id, group = str(row.request_id), str(row.group)
        if group not in group_configs:
            raise ValueError("missing evaluation group config")
        vector = row[list(FEATURE_NAMES)].to_numpy(dtype=np.float32)
        predictions = model.predict(vector)
        targets, outcomes = [], {}
        keys = sorted(response_keys.get(request_id, []))
        for key in keys:
            response = response_index[(request_id, key)]
            if response.get("status") == "ineligible":
                continue
            judgment = judgment_index.get((request_id, key))
            valid = judgment is not None and trusted_judgment(judgment)
            if judgment is not None and not valid:
                uncertain += 1
            pricing = response.get("pricing") or {}
            # Missing runtime prices remain absent, never coerced to zero.
            target = Target.model_validate(
                {
                    **catalog.get(key, {}),
                    "provider": key[0],
                    "model": key[1],
                    "inputPricePerMillionUsd": pricing.get("input_per_m_usd"),
                    "outputPricePerMillionUsd": pricing.get("output_per_m_usd"),
                }
            )
            targets.append(target)
            quality = float(judgment["quality"]) if valid and judgment else None
            # A failed response cannot satisfy the oracle even if a stale judge says one.
            if response.get("status") != "ok" and valid:
                quality = 0.0
            outcome = {
                "quality": quality,
                "cost": _response_cost(response, "cost_usd"),
                "billed_cost": _response_cost(response, "billed_cost_usd"),
                "method": str(judgment.get("method", "unknown"))
                if judgment
                else "missing",
                "status": response.get("status"),
                "duration_ms": _finite(response.get("duration_ms")),
                "ttfb_ms": _finite(response.get("ttfb_ms")),
                "output_tokens": _finite(
                    (response.get("usage") or {}).get("output_tokens")
                ),
            }
            outcomes[key] = outcome
            if key in per_target and quality is not None:
                per_target[key].append(
                    {**outcome, "prediction": predictions[key], "group": group}
                )
        data_by_group.setdefault(group, []).append(
            {
                "targets": targets,
                "outcomes": outcomes,
                "predictions": predictions,
                "input_tokens": float(row.estimatedTokens),
                "missing_fanout": bool(active_keys - set(keys)),
            }
        )

    def target_metrics(key: TargetKey, samples: list[dict[str, Any]]) -> dict[str, Any]:
        qualities = np.asarray([s["quality"] for s in samples])
        predicted = np.asarray([s["prediction"].quality for s in samples])
        # AUC only has a defined binary cohort; fractional ties/rubric values
        # remain in Brier/Spearman, and do not get silently thresholded.
        binary = np.isin(qualities, [0, 1])
        applicable = len(np.unique(qualities[binary])) == 2
        auc = (
            float(roc_auc_score(qualities[binary], predicted[binary]))
            if applicable
            else None
        )
        rank_applicable = (
            len(samples) >= 2
            and len(np.unique(qualities)) > 1
            and len(np.unique(predicted)) > 1
        )
        spearman = (
            float(spearmanr(qualities, predicted).statistic)
            if rank_applicable
            else None
        )
        outputs = [
            s
            for s in samples
            if s["status"] == "ok"
            and s["output_tokens"] is not None
            and s["output_tokens"] > 0
        ]
        return {
            "provider": key[0],
            "model": key[1],
            "n_test": len(samples),
            "n_auc": int(binary.sum()),
            "auc": auc,
            "auc_reason": None if applicable else "single_class_or_no_binary_labels",
            "brier": float(np.mean((qualities - predicted) ** 2)) if samples else None,
            "spearman": spearman,
            "out_tokens_mape": float(
                np.mean(
                    [
                        abs(s["prediction"].out_tokens - s["output_tokens"])
                        / s["output_tokens"]
                        for s in outputs
                    ]
                )
            )
            if outputs
            else None,
            "out_tokens_n": len(outputs),
            "anchor": key == anchor,
            "gate_pass": len(samples) >= 100
            and (auc is None or (auc >= 0.70 and int(binary.sum()) >= 100)),
        }

    target_reports = [
        target_metrics(key, samples) for key, samples in sorted(per_target.items())
    ]

    def replay(
        rows: list[dict[str, Any]], cfg: GroupConfig, floor: float
    ) -> dict[str, Any]:
        floor_cfg = cfg.model_copy(update={"quality_floor": floor, "explore_rate": 0.0})
        samples: dict[str, list[dict[str, Any]]] = {
            name: []
            for name in (
                "lrp",
                "always_cheapest",
                "always_anchor",
                "weighted_random",
                "bt_only",
                "oracle",
            )
        }
        random_source = random.Random(seed)
        no_oracle, no_targets, incomplete = 0, 0, 0
        for row in rows:
            targets, outcomes, predictions = (
                row["targets"],
                row["outcomes"],
                row["predictions"],
            )
            if not targets:
                no_targets += 1
            oracle_keys = [
                key
                for key, o in outcomes.items()
                if o["status"] == "ok"
                and o["quality"] is not None
                and o["quality"] >= floor
                and o["cost"] is not None
            ]
            oracle_key = (
                min(oracle_keys, key=lambda key: (outcomes[key]["cost"], key))
                if oracle_keys
                else None
            )
            oracle_cost = outcomes[oracle_key]["cost"] if oracle_key else None
            no_oracle += oracle_key is None
            if (
                any(
                    o["quality"] is None or o["cost"] is None for o in outcomes.values()
                )
                or not outcomes
                or row["missing_fanout"]
            ):
                incomplete += 1
            selections: dict[str, TargetKey | None] = {name: None for name in samples}
            selections["oracle"] = oracle_key
            trained_predictions = {
                key: prediction
                for key, prediction in predictions.items()
                if key in model.models
                and model.models[key].n_train >= cfg.min_train_rows
            }
            bt_predictions = {
                key: prediction
                for key, prediction in model.bt_predictions().items()
                if key in trained_predictions
            }
            if any(t.key in trained_predictions for t in targets):
                selections["lrp"] = targets[
                    policy(
                        targets,
                        trained_predictions,
                        floor_cfg,
                        random_source,
                        input_tokens=row["input_tokens"],
                        exploration_allowed=False,
                    ).primary
                ].key
                selections["bt_only"] = targets[
                    policy(
                        targets,
                        bt_predictions,
                        floor_cfg,
                        random_source,
                        input_tokens=row["input_tokens"],
                        exploration_allowed=False,
                    ).primary
                ].key
            if targets:
                selections["always_anchor"] = anchor if anchor in outcomes else None
                priced = [
                    (
                        estimated_cost(
                            t,
                            predictions.get(t.key, Prediction(0, row["input_tokens"])),
                            row["input_tokens"],
                            floor_cfg,
                        ),
                        t.key,
                    )
                    for t in targets
                ]
                known = [(cost, key) for cost, key in priced if cost is not None]
                selections["always_cheapest"] = min(known)[1] if known else None
                weights = [t.weight for t in targets]
                selections["weighted_random"] = (
                    random_source.choices(targets, weights=weights, k=1)[0].key
                    if sum(weights) > 0
                    else None
                )
            for name, key in selections.items():
                missing = {"quality": None, "cost": None, "method": "unavailable"}
                samples[name].append(
                    {**outcomes.get(key, missing), "oracle_cost": oracle_cost}
                )
        return {
            "baselines": {
                name: _metrics(values, floor) for name, values in samples.items()
            },
            "no_successful_oracle": no_oracle,
            "no_eligible_targets": no_targets,
            "incomplete_requests": incomplete,
        }

    groups = {}
    for group, rows in sorted(data_by_group.items()):
        cfg = GroupConfig.model_validate(group_configs[group])
        result = replay(rows, cfg, cfg.quality_floor)
        group_target_reports = [
            target_metrics(key, [s for s in samples if s["group"] == group])
            for key, samples in sorted(per_target.items())
        ]
        result["targets"] = group_target_reports
        baselines = result["baselines"]
        lrp, anchor_metrics, bt_metrics = [
            baselines[name] for name in ("lrp", "always_anchor", "bt_only")
        ]
        gates = {
            "quality_vs_anchor": _gate_comparison(
                lrp["quality_mean"],
                anchor_metrics["quality_mean"],
                lambda a, b: a >= 0.97 * b,
            ),
            "cost_vs_anchor": _gate_comparison(
                lrp["cost_total_usd"],
                anchor_metrics["cost_total_usd"],
                lambda a, b: a <= 0.60 * b,
            ),
            "floor_violations": lrp["floor_violation_rate"] is not None
            and lrp["floor_violation_rate"] <= 0.10,
            "target_auc_and_coverage": bool(group_target_reports)
            and all(t["gate_pass"] for t in group_target_reports),
            "dominates_bt": (
                _gate_comparison(
                    lrp["quality_mean"], bt_metrics["quality_mean"], lambda a, b: a >= b
                )
                and _gate_comparison(
                    lrp["cost_total_usd"],
                    bt_metrics["cost_total_usd"],
                    lambda a, b: a <= b,
                )
                and (
                    lrp["quality_mean"] != bt_metrics["quality_mean"]
                    or lrp["cost_total_usd"] != bt_metrics["cost_total_usd"]
                )
            ),
            "complete_holdout": result["incomplete_requests"] == 0
            and anchor_metrics["quality_observed"] == len(rows),
            "real_data_and_embedding": not synthetic,
        }
        result.update(
            quality_floor=cfg.quality_floor,
            gates=gates,
            promotion_pass=all(gates.values()),
            floor_sweep=[
                {"quality_floor": floor, **replay(rows, cfg, floor)}
                for floor in [round(0.60 + i * 0.05, 2) for i in range(8)]
            ],
        )
        groups[group] = result
    candidate_models = {key[1] for key in active_keys}
    warnings = []
    if any(
        row.get("judge_model") in candidate_models for row in judgment_index.values()
    ):
        warnings.append("judge_is_candidate_self_preference_risk")
    if synthetic:
        warnings.append("synthetic_wiring_only_not_promotable")
    report = {
        "schema_version": "lrp.eval.v1",
        "bundle_version": model.version,
        "split": "test",
        "synthetic": synthetic,
        "n_test": len(holdout),
        "uncertain_judgments": uncertain,
        "targets": target_reports,
        "groups": groups,
        "warnings": warnings,
        "language_distribution": {
            str(k): int(v) for k, v in frame.language.value_counts().items()
        }
        if "language" in frame
        else {},
        "promotion_pass": bool(groups)
        and all(g["promotion_pass"] for g in groups.values()),
    }
    report["promotion_passed"] = report["promotion_pass"]
    if out is not None:
        write_private(destination, canonical_json(report))
        write_private(destination.with_suffix(".md"), render_markdown(report).encode())
        write_private(destination.with_suffix(".svg"), render_scatter(report).encode())
    return report


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# Learned routing holdout evaluation",
        "",
        f"Promotion passed: **{report['promotion_pass']}**. Synthetic: **{report['synthetic']}**.",
        "",
    ]
    for group, result in report["groups"].items():
        safe_group = str(group).replace("|", "_").replace("\n", " ")
        lines.extend(
            [
                f"## {safe_group}",
                "",
                "| Policy | Quality | Cost USD | Floor violations |",
                "|---|---:|---:|---:|",
            ]
        )
        for name, metrics in result["baselines"].items():
            lines.append(
                f"| {name} | {metrics['quality_mean']} | {metrics['cost_total_usd']} | {metrics['floor_violation_rate']} |"
            )
        lines.extend(
            [
                "",
                f"No successful oracle: {result['no_successful_oracle']}. Incomplete requests: {result['incomplete_requests']}.",
                "",
            ]
        )
        lines.extend(
            f"- {name}: {'PASS' if passed else 'FAIL'}"
            for name, passed in result["gates"].items()
        )
        lines.append("")
    lines.extend(
        [
            "Single-class AUC is N/A; inspect binary cohort counts and coverage in the JSON report.",
            "",
            *report["warnings"],
            "",
        ]
    )
    return "\n".join(lines)


def render_scatter(report: dict[str, Any]) -> str:
    points = [
        (group, name, metrics["cost_per_1k"], metrics["quality_mean"])
        for group, result in report["groups"].items()
        for name, metrics in result["baselines"].items()
        if metrics["cost_per_1k"] is not None and metrics["quality_mean"] is not None
    ]
    maximum = max([p[2] for p in points] or [1]) or 1
    elements = [
        '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="560" viewBox="0 0 900 560">',
        '<rect width="900" height="560" fill="white"/><path d="M70 30V480H860" fill="none" stroke="black"/>',
        '<text x="300" y="535">Stored cost USD per 1000 requests</text><text x="5" y="20">Quality</text>',
    ]
    colors = ["#1565c0", "#ad1457", "#2e7d32", "#6a1b9a", "#ef6c00", "#37474f"]
    for index, (group, name, cost, quality) in enumerate(points):
        x, y = 70 + 710 * cost / maximum, 480 - 430 * quality
        label = html.escape(f"{group}: {name}")
        elements.append(
            f'<circle cx="{x:.2f}" cy="{y:.2f}" r="5" fill="{colors[index % len(colors)]}"><title>{label}; cost={cost:.6g}; quality={quality:.4f}</title></circle><text x="{x + 7:.2f}" y="{y - 6:.2f}" font-size="11">{label}</text>'
        )
    return "\n".join([*elements, "</svg>"])
