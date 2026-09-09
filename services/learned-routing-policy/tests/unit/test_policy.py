# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import random
import re

import pytest
from hypothesis import given
from hypothesis import strategies as st
from lrp.policy import (
    CacheEstimate,
    Prediction,
    cost_bucket,
    decide,
    effective_quality_floor,
    estimated_cost,
    floor_sweep_values,
    format_decision_label,
    metric_label,
    quality_bucket,
    resolve_cache_estimate,
    safe_label,
)
from lrp.schemas import GroupConfig, Target
from pydantic import ValidationError


def target(name, price=1, *, cached=None):
    return Target(
        provider="synthetic",
        model=name,
        input_price=price,
        output_price=price,
        cached_input_price=cached,
    )


def test_cheapest_floor_ties_and_missing_prices():
    targets = [target("b", 2), target("a", 1)]
    preds = {t.key: Prediction(0.9, 100) for t in targets}
    cfg = GroupConfig()
    assert decide(targets, preds, cfg, random.Random(1)).primary == 1
    preds[targets[1].key] = Prediction(0.7, 100)
    assert decide(targets, preds, cfg, random.Random(1)).primary == 0
    cfg.quality_floor = 1
    assert decide(targets, preds, cfg, random.Random(1)).label.startswith("lrp:floor")
    targets[0].input_price = None
    cfg.quality_floor = 0.8
    assert decide(targets, preds, cfg, random.Random(1)).label.startswith("lrp:noprice")
    targets = [target("b"), target("a")]
    preds = {t.key: Prediction(0.9, 100) for t in targets}
    assert decide(targets, preds, cfg, random.Random(1)).primary == 1


def test_pin_exploration_single_and_unknown_indexes():
    targets = [target("unknown"), target("known"), target("other")]
    preds = {t.key: Prediction(0.9, 100) for t in targets[1:]}
    cfg = GroupConfig(explore_rate=1)
    assert decide(targets, preds, cfg, random.Random(1), pin=targets[2].key).primary == 2
    draws = [decide(targets, preds, cfg, random.Random(i)).primary for i in range(10000)]
    assert abs(draws.count(1) - 5000) < 200
    assert set(draws) == {1, 2}
    assert not decide(
        targets, preds, cfg, random.Random(1), exploration_allowed=False
    ).label.startswith("lrp:explore")
    assert decide([targets[1]], preds, cfg, random.Random(1)).fallbacks == ()
    assert decide(targets, {}, cfg, random.Random(1)).label.startswith("lrp:noknown")


@given(
    st.lists(
        st.tuples(
            st.floats(min_value=0, max_value=100, allow_nan=False),
            st.floats(min_value=0, max_value=1, allow_nan=False),
        ),
        min_size=1,
        max_size=30,
    )
)
def test_fallback_permutation(values):
    targets = [target(str(i), price) for i, (price, _) in enumerate(values)]
    preds = {t.key: Prediction(values[i][1], 100) for i, t in enumerate(targets)}
    result = decide(targets, preds, GroupConfig(), random.Random(42))
    assert sorted([result.primary, *result.fallbacks]) == list(range(len(targets)))
    assert re.fullmatch(r"[A-Za-z0-9_.:-]{1,64}", result.label)


def test_label_exact_router_charset():
    assert re.fullmatch(r"[A-Za-z0-9_.:-]{1,64}", safe_label("lrp:/" + "x" * 100))
    detailed = format_decision_label("cheapest-above-floor", quality=0.91, cost=0.0004)
    assert re.fullmatch(r"[A-Za-z0-9_.:-]{1,64}", detailed)
    assert detailed.startswith("lrp:caf:q")
    assert metric_label(detailed) == "lrp:caf"
    assert quality_bucket(0.91) == "0.90"
    assert cost_bucket(0.0004) == "2"


def test_project_quality_floors_and_sweep():
    cfg = GroupConfig(
        quality_floor=0.8,
        floors_by_project={"demo-strict": 0.9, "demo-loose": 0.7},
    )
    assert effective_quality_floor(cfg, "") == 0.8
    assert effective_quality_floor(cfg, "unknown-project") == 0.8
    assert effective_quality_floor(cfg, "demo-strict") == 0.9
    assert effective_quality_floor(cfg, "  demo-loose ") == 0.7
    targets = [target("cheap", 1), target("strong", 10)]
    preds = {
        targets[0].key: Prediction(0.85, 100),
        targets[1].key: Prediction(0.95, 100),
    }
    # Group/default floor: cheap meets 0.8.
    assert decide(targets, preds, cfg, random.Random(1), project="").primary == 0
    # Strict project: cheap 0.85 < 0.9 → strong wins as only above-floor priced target.
    assert (
        decide(targets, preds, cfg, random.Random(1), project="demo-strict").primary == 1
    )
    sweep = floor_sweep_values(cfg)
    assert ("*", 0.8) in sweep
    assert ("demo-strict", 0.9) in sweep
    with pytest.raises(ValidationError):
        GroupConfig(floors_by_project={"demo": 1.5})
    with pytest.raises(ValidationError):
        GroupConfig(floors_by_project={"": 0.5})


def test_latency_constraint_cold_start_and_exclusion():
    targets = [target("fast", 2), target("slow", 1)]
    preds = {t.key: Prediction(0.9, 100) for t in targets}
    evidence = {targets[0].key: 100.0, targets[1].key: 5000.0}
    cfg = GroupConfig(latency_p95_ms_max=1000.0, unknown_latency="allow")
    # Slow target excluded; expensive-but-fast wins above floor.
    decision = decide(
        targets, preds, cfg, random.Random(1), latency_evidence=evidence
    )
    assert decision.primary == 0
    assert decision.label.startswith("lrp:caf")

    # Cold-start / unknown latency remains eligible by default.
    cold = decide(
        targets,
        preds,
        cfg,
        random.Random(1),
        latency_evidence={targets[1].key: 5000.0},
    )
    assert cold.primary == 0  # fast unknown → allowed; slow excluded

    cfg_exclude = GroupConfig(latency_p95_ms_max=1000.0, unknown_latency="exclude")
    unknown_only = decide(
        targets, preds, cfg_exclude, random.Random(1), latency_evidence={}
    )
    assert unknown_only.label.startswith("lrp:slowunk")

    all_slow = decide(
        targets,
        preds,
        GroupConfig(latency_p95_ms_max=50.0),
        random.Random(1),
        latency_evidence=evidence,
    )
    assert all_slow.label.startswith("lrp:slow")


def test_cache_estimate_requires_trustworthy_metadata():
    priced = target("cached", 2.0, cached=0.2)
    pred = Prediction(0.9, 100)
    cfg = GroupConfig()
    full = estimated_cost(priced, pred, 1000, cfg)
    assert full == pytest.approx((1000 * 2.0 + 100 * 2.0) / 1e6)

    # Unknown cache state: no invented savings even with catalog cached price.
    unknown = estimated_cost(
        priced, pred, 1000, cfg, cache=CacheEstimate(state="unknown")
    )
    assert unknown == full

    # Session pin alone must not create savings.
    pin_context = resolve_cache_estimate({"systemChars": 50_000}, pinned=True)
    assert pin_context.state == "unknown"
    assert estimated_cost(priced, pred, 1000, cfg, cache=pin_context) == full

    hit = estimated_cost(
        priced, pred, 1000, cfg, cache=CacheEstimate(state="hit")
    )
    assert hit == pytest.approx((1000 * 0.2 + 100 * 2.0) / 1e6)

    partitioned = estimated_cost(
        priced,
        pred,
        1000,
        cfg,
        cache=CacheEstimate(state="hit", cached_input_tokens=800, uncached_input_tokens=200),
    )
    assert partitioned == pytest.approx((800 * 0.2 + 200 * 2.0 + 100 * 2.0) / 1e6)

    # Hit without catalog cached price: preserve full input price.
    no_meta = target("plain", 2.0)
    assert estimated_cost(
        no_meta, pred, 1000, cfg, cache=CacheEstimate(state="hit")
    ) == estimated_cost(no_meta, pred, 1000, cfg)

    # Partitioned counts alone can mark hit when positive cached tokens exist.
    from_tokens = resolve_cache_estimate(
        {"cachedInputTokens": 10, "uncachedInputTokens": 5}
    )
    assert from_tokens.state == "hit"
