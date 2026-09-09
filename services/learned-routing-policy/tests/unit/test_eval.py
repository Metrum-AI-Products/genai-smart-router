# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

# ruff: noqa: F811 -- pytest fixtures are imported from the owned training test module.
import json

from lrp.eval import evaluate
from test_train import dataset, trained  # noqa: F401


def test_holdout_gates_baselines_sweep_and_report(dataset, trained, tmp_path):
    frame, judgments, responses = dataset
    report = evaluate(
        trained,
        frame,
        judgments,
        responses,
        {"groups": {"demo": {}}},
        out=tmp_path / "report.json",
    )
    assert not report["promotion_pass"] and report["synthetic"]
    group = report["groups"]["demo"]
    assert len(group["baselines"]) == 6 and len(group["floor_sweep"]) == 8
    assert group["gates"]["target_auc_and_coverage"]
    assert not group["gates"]["real_data_and_embedding"]
    anchor = next(t for t in report["targets"] if t["anchor"])
    assert anchor["auc"] is None and anchor["auc_reason"]
    assert report["n_test"] == int((frame.split == "test").sum())
    assert json.loads((tmp_path / "report.json").read_text()) == report
    assert "<svg" in (tmp_path / "report.svg").read_text()


def test_unknown_cost_and_uncertain_coverage_not_free(dataset, trained):
    frame, judgments, responses = dataset
    modified = [{**r, "cost_usd": None, "pricing": None} for r in responses]
    labels = [{**j, "detail": {"parse_failed": True}} for j in judgments]
    report = evaluate(trained, frame, labels, modified, {"groups": {"demo": {}}})
    result = report["groups"]["demo"]
    assert result["no_successful_oracle"] == report["n_test"]
    assert result["baselines"]["lrp"]["cost_total_usd"] is None
    assert result["baselines"]["lrp"]["quality_mean"] is None
    assert not result["gates"]["complete_holdout"]
    assert report["uncertain_judgments"] > 0


def test_evaluation_never_uses_training_outcomes(dataset, trained):
    frame, judgments, responses = dataset
    holdout = set(frame.loc[frame.split == "test", "request_id"])
    modified = [
        {**j, "quality": 0} if j["request_id"] not in holdout else j for j in judgments
    ]
    cfg = {"groups": {"demo": {}}}
    assert evaluate(trained, frame, judgments, responses, cfg) == evaluate(
        trained, frame, modified, responses, cfg
    )


def test_attempt_costs_and_missing_fanout_fail_coverage(dataset, trained):
    frame, judgments, responses = dataset
    cfg = {"groups": {"demo": {}}}
    attempts = [
        {
            **r,
            "attempts": [
                {
                    "sequence": 1,
                    "status": "upstream_error",
                    "duration_ms": 1,
                    "cost_usd": 0.1,
                },
                {
                    "sequence": 2,
                    "status": "ok",
                    "duration_ms": 1,
                    "cost_usd": r["cost_usd"],
                },
            ],
        }
        for r in responses
    ]
    base = evaluate(trained, frame, judgments, responses, cfg)
    retried = evaluate(trained, frame, judgments, attempts, cfg)
    baseline = base["groups"]["demo"]["baselines"]["always_anchor"]["cost_total_usd"]
    assert (
        abs(
            retried["groups"]["demo"]["baselines"]["always_anchor"]["cost_total_usd"]
            - baseline
            - 0.1 * base["n_test"]
        )
        < 1e-9
    )
    unknown = [
        {
            **r,
            "attempts": [
                {
                    "sequence": 1,
                    "status": "upstream_error",
                    "duration_ms": 1,
                    "cost_usd": None,
                },
                {
                    "sequence": 2,
                    "status": "ok",
                    "duration_ms": 1,
                    "cost_usd": r["cost_usd"],
                },
            ],
        }
        for r in responses
    ]
    result = evaluate(trained, frame, judgments, unknown, cfg)["groups"]["demo"]
    assert result["baselines"]["always_anchor"]["cost_total_usd"] is None
    assert not result["gates"]["complete_holdout"]
    missing = [r for r in responses if r["target"]["model"] != "cheap/model"]
    assert not evaluate(trained, frame, judgments, missing, cfg)["groups"]["demo"][
        "gates"
    ]["complete_holdout"]


def test_no_trained_prediction_abstains(dataset, trained):
    frame, judgments, responses = dataset
    result = evaluate(
        trained,
        frame,
        judgments,
        responses,
        {"groups": {"demo": {"min_train_rows": 10000}}},
    )
    for baseline in ("lrp", "bt_only"):
        assert result["groups"]["demo"]["baselines"][baseline]["quality_observed"] == 0
        assert result["groups"]["demo"]["baselines"][baseline]["cost_observed"] == 0
    assert not result["promotion_passed"]


def test_eval_preflights_every_report_path(dataset, trained, tmp_path):
    import pytest
    from lrp.collect import DataError

    frame, judgments, responses = dataset
    destination = tmp_path / "report.json"
    unsafe = destination.with_suffix(".svg")
    unsafe.write_text("unchanged")
    unsafe.chmod(0o644)
    with pytest.raises(DataError):
        evaluate(
            trained,
            frame,
            judgments,
            responses,
            {"groups": {"demo": {}}},
            out=destination,
        )
    assert not destination.exists() and not destination.with_suffix(".md").exists()
    assert unsafe.read_text() == "unchanged"


def test_serving_provider_variance_preserves_model_and_missing(dataset, trained):
    from lrp.eval import render_markdown, serving_provider_variance

    frame, judgments, responses = dataset
    holdout = set(frame.loc[frame.split == "test", "request_id"])
    annotated = []
    cheap_index = 0
    for row in responses:
        if row["request_id"] not in holdout:
            annotated.append(row)
            continue
        updated = dict(row)
        if row["target"]["model"] == "cheap/model":
            if cheap_index % 2 == 0:
                updated["serving_provider"] = "synthetic-backend-a"
                updated["duration_ms"] = 40.0
                updated["ttfb_ms"] = 4.0
            else:
                updated["serving_provider"] = "synthetic-backend-b"
                updated["duration_ms"] = 120.0
                updated["ttfb_ms"] = 12.0
            cheap_index += 1
        # anchor/model intentionally omits serving_provider → missing evidence
        annotated.append(updated)
    report = evaluate(trained, frame, judgments, annotated, {"groups": {"demo": {}}})
    variance = report["serving_provider_variance"]
    assert variance["schema_version"] == "lrp.serving_provider_variance.v1"
    assert variance["serving_provider_missing"] > 0
    assert variance["serving_provider_observed"] > 0
    assert "serving_provider_missing_evidence" in report["warnings"]
    by_model = {(t["provider"], t["model"]): t for t in variance["targets"]}
    cheap = by_model[("synthetic", "cheap/model")]
    anchor = by_model[("synthetic", "anchor/model")]
    assert cheap["model"] == "cheap/model"
    assert cheap["distinct_serving_providers"] == 2
    assert "synthetic-backend-a" in cheap["by_serving_provider"]
    assert "synthetic-backend-b" in cheap["by_serving_provider"]
    assert cheap["variance_across_serving_providers"]["duration_p50_ms_variance"] is not None
    assert anchor["serving_provider_missing"] == anchor["n_responses"]
    assert "missing" in anchor["by_serving_provider"]
    assert "Serving-provider variance" in render_markdown(report)
    markdown_samples = serving_provider_variance(
        [
            {
                "provider": "openrouter",
                "model": "example/model-a",
                "quality": 1.0,
                "cost": 0.01,
                "duration_ms": 10,
                "ttfb_ms": 1,
                "serving_provider": "Together",
                "serving_provider_available": True,
                "status": "ok",
            },
            {
                "provider": "openrouter",
                "model": "example/model-a",
                "quality": 0.0,
                "cost": 0.02,
                "duration_ms": 50,
                "ttfb_ms": 2,
                "serving_provider": "Fireworks",
                "serving_provider_available": True,
                "status": "ok",
            },
            {
                "provider": "openrouter",
                "model": "example/model-b",
                "quality": 1.0,
                "cost": 0.03,
                "duration_ms": 20,
                "ttfb_ms": None,
                "serving_provider": None,
                "serving_provider_available": False,
                "status": "ok",
            },
        ]
    )
    assert markdown_samples["targets"][0]["model"] == "example/model-a"
    assert markdown_samples["targets"][1]["model"] == "example/model-b"
    assert markdown_samples["targets"][1]["by_serving_provider"]["missing"]["n"] == 1
