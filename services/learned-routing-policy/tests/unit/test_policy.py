# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import random
import re

from hypothesis import given
from hypothesis import strategies as st
from lrp.policy import Prediction, decide, safe_label
from lrp.schemas import GroupConfig, Target


def target(name, price=1):
    return Target(provider="synthetic", model=name, input_price=price, output_price=price)


def test_cheapest_floor_ties_and_missing_prices():
    targets = [target("b", 2), target("a", 1)]
    preds = {t.key: Prediction(0.9, 100) for t in targets}
    cfg = GroupConfig()
    assert decide(targets, preds, cfg, random.Random(1)).primary == 1
    preds[targets[1].key] = Prediction(0.7, 100)
    assert decide(targets, preds, cfg, random.Random(1)).primary == 0
    cfg.quality_floor = 1
    assert decide(targets, preds, cfg, random.Random(1)).label == "lrp:no-candidate-met-floor"
    targets[0].input_price = None
    cfg.quality_floor = 0.8
    assert decide(targets, preds, cfg, random.Random(1)).label == "lrp:unknown-pricing"
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
    assert (
        decide(targets, preds, cfg, random.Random(1), exploration_allowed=False).label
        != "lrp:explore"
    )
    assert decide([targets[1]], preds, cfg, random.Random(1)).fallbacks == ()
    assert decide(targets, {}, cfg, random.Random(1)).label == "lrp:no-known-target"


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
