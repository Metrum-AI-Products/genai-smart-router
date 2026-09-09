"""Pure cheapest-above-floor selection. No content, credentials or I/O."""

from __future__ import annotations

import math
import random
import re
from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from lrp.schemas import GroupConfig, Target, TargetKey


@dataclass(frozen=True)
class Prediction:
    quality: float
    out_tokens: float


@dataclass(frozen=True)
class Decision:
    primary: int
    fallbacks: tuple[int, ...]
    label: str

    def response(self) -> dict[str, object]:
        return {
            "targetIndex": self.primary,
            "fallbackIndexes": list(self.fallbacks),
            "classLabel": safe_label(self.label),
        }


def safe_label(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.:-]", "_", value)[:64] or "lrp:unknown"


def estimated_cost(
    target: Target, prediction: Prediction, input_tokens: float, cfg: GroupConfig
) -> float | None:
    if target.key in cfg.zero_price_targets:
        return 0.0
    if target.input_price is None or target.output_price is None:
        return None
    result = (
        max(0, input_tokens) * target.input_price
        + max(0, prediction.out_tokens) * target.output_price
    ) / 1e6
    return result if math.isfinite(result) else None


def decide(
    targets: Sequence[Target],
    predictions: Mapping[TargetKey, Prediction],
    cfg: GroupConfig,
    rng: random.Random,
    *,
    input_tokens: float = 0,
    pin: TargetKey | None = None,
    exploration_allowed: bool = True,
) -> Decision:
    if not targets:
        raise ValueError("no eligible targets")
    available = [
        i
        for i, target in enumerate(targets)
        if target.key in predictions
        and math.isfinite(predictions[target.key].quality)
        and 0 <= predictions[target.key].quality <= 1
        and math.isfinite(predictions[target.key].out_tokens)
        and predictions[target.key].out_tokens >= 0
    ]
    if not available:
        return Decision(0, tuple(range(1, len(targets))), "lrp:no-known-target")

    def rank(i: int) -> tuple[float, TargetKey, int]:
        return -predictions[targets[i].key].quality, targets[i].key, i

    ranked = sorted(available, key=rank)
    pinned = next((i for i in available if targets[i].key == pin), None)
    if pinned is not None:
        return Decision(pinned, tuple(i for i in ranked if i != pinned), "lrp:pinned")
    costs = {
        i: estimated_cost(targets[i], predictions[targets[i].key], input_tokens, cfg)
        for i in available
    }
    above = [
        i
        for i in available
        if predictions[targets[i].key].quality >= cfg.quality_floor and costs[i] is not None
    ]
    if above:
        primary = min(above, key=lambda i: (float(costs[i] or 0), rank(i)))
        label = "lrp:cheapest-above-floor"
    else:
        primary = ranked[0]
        label = (
            "lrp:unknown-pricing"
            if any(predictions[targets[i].key].quality >= cfg.quality_floor for i in available)
            else "lrp:no-candidate-met-floor"
        )
    if exploration_allowed and cfg.explore_rate > 0 and rng.random() < cfg.explore_rate:
        primary, label = rng.choice(available), "lrp:explore"
    return Decision(primary, tuple(i for i in ranked if i != primary), label)
