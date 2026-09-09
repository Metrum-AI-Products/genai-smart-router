# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Synthetic wiring tests for Wave-2 uncertainty, Thompson, drift, and cold start."""

from __future__ import annotations

import json
import random

import numpy as np
import pandas as pd
import pytest
from lrp.drift import (
    DriftThresholds,
    embedding_centroid,
    evaluate_drift,
    mean_cosine_distance_to_centroid,
    population_stability_index,
    shadow_decision_from_recommendation,
    training_centroid_record,
)
from lrp.features import FEATURE_NAMES, SCALAR_NAMES, SyntheticEmbedder, session_split
from lrp.policy import Decision, Prediction, safe_label
from lrp.schemas import GroupConfig, Target
from lrp.thompson import (
    COLD_START_ANCHOR_PROMPTS,
    cold_start_entry,
    cold_start_exploration_only,
    compare_exploration_strategies,
    decide_with_exploration,
    inject_cold_start_predictions,
    parse_cold_start_targets,
    sample_thompson_quality,
)
from lrp.train import train
from lrp.uncertainty import (
    UncertaintyEstimate,
    apply_abstention,
    compare_abstention_cost,
    ensemble_stats,
    should_abstain,
)


def _target(name: str, price: float = 1.0) -> Target:
    return Target(provider="synthetic", model=name, input_price=price, output_price=price)


def test_ensemble_stats_and_abstention_threshold():
    low = ensemble_stats([0.80, 0.81, 0.79, 0.805, 0.795])
    assert low.std < 0.15
    assert not should_abstain(low)
    high = ensemble_stats([0.2, 0.9, 0.4, 0.8, 0.1])
    assert high.std > 0.15
    assert should_abstain(high)
    assert not should_abstain(high, enabled=False)


def test_apply_abstention_routes_to_anchor_with_safe_label():
    targets = [_target("cheap", 1), _target("anchor", 10), _target("mid", 5)]
    decision = Decision(0, (1, 2), "lrp:caf:q0.90:c3")
    estimates = {
        targets[0].key: UncertaintyEstimate(0.85, 0.25, 5),
        targets[1].key: UncertaintyEstimate(0.95, 0.02, 5),
    }
    result = apply_abstention(
        decision,
        targets,
        estimates,
        anchor=targets[1].key,
        threshold=0.15,
        enabled=True,
    )
    assert result.primary == 1
    assert result.label == "lrp:uncertain"
    assert safe_label(result.label) == result.label
    unchanged = apply_abstention(
        decision, targets, estimates, anchor=targets[1].key, enabled=False
    )
    assert unchanged.primary == 0


def test_abstention_cost_comparison_scalars():
    report = compare_abstention_cost(
        enforce_cost=0.001,
        abstain_cost=0.01,
        enforce_quality=0.7,
        abstain_quality=0.95,
        enforce_latency_ms=40,
        abstain_latency_ms=55,
    )
    assert report["cost_delta_usd"] == pytest.approx(0.009)
    assert report["quality_delta"] == pytest.approx(0.25)
    assert report["label_abstain"] == "lrp:uncertain"


def test_thompson_vs_epsilon_greedy_seeded_comparison():
    targets = [_target("a", 1), _target("b", 2), _target("c", 3)]
    preds = {
        targets[0].key: Prediction(0.9, 100),
        targets[1].key: Prediction(0.85, 100),
        targets[2].key: Prediction(0.7, 100),
    }
    estimates = {
        targets[0].key: UncertaintyEstimate(0.9, 0.02, 5),
        targets[1].key: UncertaintyEstimate(0.85, 0.20, 5),
        targets[2].key: UncertaintyEstimate(0.7, 0.05, 5),
    }
    cfg = GroupConfig(explore_rate=1.0, exploration_projects=["calib"])
    report = compare_exploration_strategies(
        targets, preds, estimates, cfg, seed=7, draws=2000
    )
    assert report["draws"] == 2000
    assert abs(sum(report["epsilon_greedy_rates"]) - 1.0) < 1e-9
    assert abs(sum(report["thompson_rates"]) - 1.0) < 1e-9
    # Epsilon-greedy is near-uniform; Thompson concentrates on confident cheap targets.
    assert abs(report["epsilon_greedy_rates"][0] - report["epsilon_greedy_rates"][1]) < 0.08
    assert report["thompson_rates"][0] > report["thompson_rates"][2]
    assert max(
        abs(a - b)
        for a, b in zip(report["epsilon_greedy_rates"], report["thompson_rates"], strict=True)
    ) > 0.05


def test_thompson_respects_exploration_allowlist():
    targets = [_target("a"), _target("b")]
    preds = {t.key: Prediction(0.9, 50) for t in targets}
    estimates = {t.key: UncertaintyEstimate(0.9, 0.2, 5) for t in targets}
    cfg = GroupConfig(explore_rate=1.0)
    decision = decide_with_exploration(
        targets,
        preds,
        cfg,
        random.Random(1),
        estimates=estimates,
        exploration_allowed=False,
        strategy="thompson",
    )
    assert decision.label != "lrp:explore-thompson"
    allowed = decide_with_exploration(
        targets,
        preds,
        cfg,
        random.Random(1),
        estimates=estimates,
        exploration_allowed=True,
        strategy="thompson",
    )
    assert allowed.label == "lrp:explore-thompson"


def test_thompson_sample_reproducible():
    rng_a, rng_b = random.Random(42), random.Random(42)
    assert sample_thompson_quality(0.5, 0.1, rng_a) == sample_thompson_quality(
        0.5, 0.1, rng_b
    )


def test_psi_and_embedding_drift_with_auto_shadow():
    rng = np.random.default_rng(0)
    reference = rng.normal(0, 1, size=(400, len(SCALAR_NAMES)))
    stable = rng.normal(0, 1, size=(400, len(SCALAR_NAMES)))
    shifted = rng.normal(3, 1, size=(400, len(SCALAR_NAMES)))
    assert population_stability_index(reference[:, 0], stable[:, 0]) < 0.25
    assert population_stability_index(reference[:, 0], shifted[:, 0]) > 0.25

    # Near-centroid cluster vs far-away cluster (unit rows).
    center = np.ones(32, dtype=np.float32)
    center /= np.linalg.norm(center)
    emb_ref = center + 0.01 * rng.normal(0, 1, size=(200, 32)).astype(np.float32)
    emb_ref /= np.linalg.norm(emb_ref, axis=1, keepdims=True)
    emb_obs = emb_ref.copy()
    far = -center + 0.01 * rng.normal(0, 1, size=(200, 32)).astype(np.float32)
    emb_far = far / np.linalg.norm(far, axis=1, keepdims=True)
    centroid = embedding_centroid(emb_ref)
    assert mean_cosine_distance_to_centroid(centroid, emb_obs) < 0.05
    report = evaluate_drift(
        reference_scalars=reference,
        observed_scalars=shifted,
        reference_embeddings=emb_ref,
        observed_embeddings=emb_far,
        thresholds=DriftThresholds(psi=0.25, embedding_distance=0.15),
        auto_shadow=True,
    )
    assert report.above_threshold
    assert report.recommend_shadow
    assert "psi_threshold" in report.reasons
    assert set(report.metrics()) >= {
        "psi_max",
        "embedding_centroid_distance",
        "recommend_shadow",
    }
    record = training_centroid_record(emb_ref)
    assert record["schema_version"] == "lrp.drift.centroid.v1"
    assert len(record["centroid"]) == 32


def test_shadow_recommendation_preserves_router_authority():
    decision = Decision(2, (0, 1), "lrp:caf:q0.90:c3")
    labeled = shadow_decision_from_recommendation(decision, recommend_shadow=True)
    assert labeled.primary == 2
    assert labeled.label == "lrp:shadow-drift"
    forced = shadow_decision_from_recommendation(
        decision, recommend_shadow=True, force_index_zero=True
    )
    assert forced.primary == 0
    assert forced.label == "lrp:shadow-drift"
    assert shadow_decision_from_recommendation(decision, recommend_shadow=False) is decision


def test_cold_start_seed_and_injection_never_overrides_eligibility():
    entry = cold_start_entry(
        provider="synthetic",
        model="new/model",
        bt_strength=0.62,
        n_anchor_prompts=COLD_START_ANCHOR_PROMPTS,
        mean_out_tokens=40,
    )
    assert entry is not None
    assert entry["cold_start"] is True
    assert entry["skipped"] == "cold_start_anchor_seed"
    assert cold_start_entry(
        provider="synthetic",
        model="new/model",
        bt_strength=0.62,
        n_anchor_prompts=199,
    ) is None

    cold = parse_cold_start_targets({"targets": [entry]})
    eligible = [_target("other"), _target("new/model")]
    preds, estimates = inject_cold_start_predictions(
        {},
        eligible,
        cold,
        min_train_rows=200,
        trained_keys=set(),
    )
    assert ("synthetic", "new/model") in preds
    assert ("synthetic", "new/model") in estimates
    # Ineligible (not in router target list) must not appear.
    preds2, _ = inject_cold_start_predictions(
        {},
        [_target("other")],
        cold,
        trained_keys=set(),
    )
    assert preds2 == {}


def test_cold_start_thompson_exploration_label():
    targets = [_target("trained", 1), _target("new/model", 2)]
    # Serve-path cold-start mode only injects BT predictions for untrained keys.
    preds = {
        targets[1].key: Prediction(0.6, 50),
    }
    estimates = {
        targets[1].key: UncertaintyEstimate(0.6, 0.3, 3),
    }
    cfg = GroupConfig(explore_rate=1.0, cold_start_exploration=True)
    hits = 0
    for seed in range(20):
        decision = cold_start_exploration_only(
            targets,
            preds,
            estimates,
            {targets[1].key},
            cfg,
            random.Random(seed),
            exploration_allowed=True,
        )
        if decision.label == "lrp:explore-cold-start":
            hits += 1
            assert targets[decision.primary].key == targets[1].key
    assert hits == 20


def test_train_writes_ensemble_and_cold_start(tmp_path):
    rows, judgments, responses = [], [], []
    for i in range(600):
        x = (i % 20) / 20
        values = np.zeros(len(FEATURE_NAMES), dtype=np.float32)
        values[0], values[FEATURE_NAMES.index("estimatedTokens")] = x, 100
        row = dict(zip(FEATURE_NAMES, values))
        row.update(
            request_id=f"synthetic-{i:04d}",
            session_key=f"session-{i // 2}",
            split=session_split(f"session-{i // 2}"),
            group="demo",
            source="synthetic",
            synthetic=True,
            embedding_kind="synthetic",
            embedding_fingerprint=SyntheticEmbedder.fingerprint,
            language="en",
        )
        rows.append(row)
        for name, quality, price in [
            ("cheap/model", float(x < 0.6), 1),
            ("anchor/model", 1.0, 10),
            ("new/model", float(x < 0.5), 2),
        ]:
            # Omit validation-split rows for new/model so it cannot fully train
            # but still accumulates a 200+ train-row anchor seed (#30).
            if name == "new/model" and row["split"] == "valid":
                continue
            target = {"provider": "synthetic", "model": name}
            judgments.append(
                {
                    "request_id": row["request_id"],
                    "target": target,
                    "source": "synthetic",
                    "judged_at": "2026-09-09T00:00:00Z",
                    "method": "pairwise_vs_anchor:v1",
                    "quality": quality,
                    "detail": {},
                }
            )
            responses.append(
                {
                    "request_id": row["request_id"],
                    "target": target,
                    "source": "synthetic",
                    "started_at": "2026-09-09T00:00:00Z",
                    "status": "ok",
                    "usage": {"input_tokens": 100, "output_tokens": 20 + i % 20},
                    "pricing": {
                        "input_per_m_usd": price,
                        "output_per_m_usd": price,
                        "source": "https://example.test/pricing",
                        "fetched_at": "2026-09-09T00:00:00Z",
                    },
                    "cost_usd": price * (120 + i % 20) / 1e6,
                    "duration_ms": 50,
                    "ttfb_ms": 5,
                }
            )
    frame = pd.DataFrame(rows)
    path = train(
        frame,
        judgments,
        responses,
        tmp_path,
        embedding={"kind": "synthetic"},
        anchor=("synthetic", "anchor/model"),
        num_boost_round=20,
        ensemble_size=3,
    )
    manifest = json.loads((path / "manifest.json").read_text())
    assert manifest["ensemble_size"] == 3
    by_model = {t["model"]: t for t in manifest["targets"]}
    cheap = by_model["cheap/model"]
    assert cheap.get("ensemble_size", 0) >= 2
    assert len(cheap["ensemble_quality_files"]) == cheap["ensemble_size"]
    for rel in cheap["ensemble_quality_files"]:
        assert (path / rel).is_file()
    new = by_model["new/model"]
    assert new.get("cold_start") is True
    assert new.get("skipped") == "cold_start_anchor_seed"
    assert new["n_train"] >= COLD_START_ANCHOR_PROMPTS
    assert new["n_valid"] < 2
