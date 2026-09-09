# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Pure cheapest-above-floor selection. No content, credentials or I/O."""

from __future__ import annotations

import math
import random
import re
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Literal

from lrp.schemas import GroupConfig, Target, TargetKey

# Router charset for classLabel: [A-Za-z0-9_.:-] and at most 64 characters.
_LABEL_RE = re.compile(r"[^A-Za-z0-9_.:-]")

# Coarse kind abbreviations kept short so optional q/c suffixes fit in 64 chars.
_KIND_ABBREV = {
    "cheapest-above-floor": "caf",
    "no-candidate-met-floor": "floor",
    "unknown-pricing": "noprice",
    "explore": "explore",
    "pinned": "pinned",
    "no-known-target": "noknown",
    "latency-constrained": "slow",
    "latency-unknown-excluded": "slowunk",
}


@dataclass(frozen=True)
class Prediction:
    quality: float
    out_tokens: float


@dataclass(frozen=True)
class CacheEstimate:
    """Trustworthy prompt-cache evidence for selection-time cost estimates.

    Unknown state never invents cache savings. Session pins alone are not
    evidence of a cache hit; callers must supply an explicit hit/miss state or
    partitioned cached/uncached token counts from upstream/router metadata.
    """

    state: Literal["unknown", "hit", "miss"] = "unknown"
    cached_input_tokens: float | None = None
    uncached_input_tokens: float | None = None


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
    return _LABEL_RE.sub("_", value)[:64] or "lrp:unknown"


def metric_label(value: str) -> str:
    """Coarse Prometheus-safe label: keep only ``lrp:<kind>`` (no q/c suffixes)."""
    cleaned = safe_label(value)
    parts = cleaned.split(":")
    if len(parts) >= 2 and parts[0] == "lrp":
        return safe_label(f"lrp:{parts[1]}")
    return cleaned


def cost_bucket(cost_usd: float) -> str:
    """Map a cost to a low-cardinality order-of-magnitude bucket."""
    if not math.isfinite(cost_usd) or cost_usd < 0:
        return "x"
    if cost_usd == 0:
        return "0"
    # Buckets: <1e-6 → 0, <1e-5 → 1, …, <1e-1 → 5, <1 → 6, else 7+
    exponent = math.floor(math.log10(cost_usd))
    # Shift so 1e-6 → bucket 0
    bucket = max(0, min(9, exponent + 6))
    return str(bucket)


def quality_bucket(quality: float) -> str:
    """Round quality to 0.05 steps for bounded telemetry."""
    if not math.isfinite(quality):
        return "x"
    stepped = round(max(0.0, min(1.0, quality)) * 20) / 20
    return f"{stepped:.2f}"


def format_decision_label(
    kind: str,
    *,
    quality: float | None = None,
    cost: float | None = None,
) -> str:
    """Build a bounded classLabel conveying top quality/cost reason.

    Exact floats and content never appear. Prometheus should use
    :func:`metric_label` so cardinality stays on the coarse kind.
    """
    abbrev = _KIND_ABBREV.get(kind, kind.replace("_", "-")[:16])
    parts = [f"lrp:{abbrev}"]
    if quality is not None:
        parts.append(f"q{quality_bucket(quality)}")
    if cost is not None:
        parts.append(f"c{cost_bucket(cost)}")
    return safe_label(":".join(parts))


def effective_quality_floor(cfg: GroupConfig, project: str = "") -> float:
    """Resolve per-project floor override with group fallback.

    Empty or unknown projects use the group floor. Project keys are
    deployment-defined; no production identities are hardcoded.
    """
    key = project.strip()
    if key and key in cfg.floors_by_project:
        return float(cfg.floors_by_project[key])
    return float(cfg.quality_floor)


def estimated_cost(
    target: Target,
    prediction: Prediction,
    input_tokens: float,
    cfg: GroupConfig,
    *,
    cache: CacheEstimate | None = None,
) -> float | None:
    if target.key in cfg.zero_price_targets:
        return 0.0
    if target.input_price is None or target.output_price is None:
        return None
    output_cost = max(0.0, prediction.out_tokens) * target.output_price
    input_cost = _input_cost_usd(target, max(0.0, input_tokens), cache)
    if input_cost is None:
        return None
    result = (input_cost + output_cost) / 1e6
    return result if math.isfinite(result) else None


def _input_cost_usd(
    target: Target, input_tokens: float, cache: CacheEstimate | None
) -> float | None:
    """Apply cached-input pricing only with trustworthy metadata and cache state."""
    assert target.input_price is not None
    if cache is None or cache.state == "unknown":
        return input_tokens * target.input_price
    if cache.state == "miss":
        return input_tokens * target.input_price
    # state == "hit": require an explicit cached-input catalog price.
    if target.cached_input_price is None or not math.isfinite(target.cached_input_price):
        return input_tokens * target.input_price
    cached = cache.cached_input_tokens
    uncached = cache.uncached_input_tokens
    if cached is not None and uncached is not None:
        if cached < 0 or uncached < 0 or not math.isfinite(cached) or not math.isfinite(uncached):
            return input_tokens * target.input_price
        return cached * target.cached_input_price + uncached * target.input_price
    # Hit without partitioned token counts: treat full input as cached only when
    # the operator/upstream marked an explicit hit (still never invented).
    return input_tokens * target.cached_input_price


def resolve_cache_estimate(
    context: Mapping[str, object] | None,
    *,
    pinned: bool = False,
) -> CacheEstimate:
    """Derive cache estimate from router/context scalars.

    Session pins never imply a cache hit. Absent or unrecognized state is
    unknown and preserves full-price estimates.
    """
    del pinned  # Explicitly unused: pins are not cache evidence.
    if not context:
        return CacheEstimate()
    raw_state = context.get("promptCacheState") or context.get("prompt_cache_state")
    state: Literal["unknown", "hit", "miss"] = "unknown"
    if isinstance(raw_state, str):
        normalized = raw_state.strip().lower()
        if normalized in {"hit", "warm", "cached"}:
            state = "hit"
        elif normalized in {"miss", "cold", "uncached"}:
            state = "miss"
    cached = _optional_nonneg_float(
        context.get("cachedInputTokens") or context.get("cached_input_tokens")
    )
    uncached = _optional_nonneg_float(
        context.get("uncachedInputTokens") or context.get("uncached_input_tokens")
    )
    # Partitioned counts alone can establish a partial-hit estimate when both
    # sides are present; still require catalog cached_input_price at cost time.
    if state == "unknown" and cached is not None and uncached is not None and cached > 0:
        state = "hit"
    return CacheEstimate(state=state, cached_input_tokens=cached, uncached_input_tokens=uncached)


def _optional_nonneg_float(value: object) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    number = float(value)
    if not math.isfinite(number) or number < 0:
        return None
    return number


def latency_passes(
    target_key: TargetKey,
    evidence: Mapping[TargetKey, float] | None,
    cfg: GroupConfig,
) -> bool:
    """Return whether a target clears the optional upstream latency gate."""
    if cfg.latency_p95_ms_max is None:
        return True
    if evidence is None or target_key not in evidence:
        return cfg.unknown_latency == "allow"
    observed = evidence[target_key]
    if not math.isfinite(observed) or observed < 0:
        return cfg.unknown_latency == "allow"
    return observed <= cfg.latency_p95_ms_max


def decide(
    targets: Sequence[Target],
    predictions: Mapping[TargetKey, Prediction],
    cfg: GroupConfig,
    rng: random.Random,
    *,
    input_tokens: float = 0,
    pin: TargetKey | None = None,
    exploration_allowed: bool = True,
    project: str = "",
    latency_evidence: Mapping[TargetKey, float] | None = None,
    cache: CacheEstimate | None = None,
) -> Decision:
    if not targets:
        raise ValueError("no eligible targets")
    floor = effective_quality_floor(cfg, project)
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
        return Decision(0, tuple(range(1, len(targets))), format_decision_label("no-known-target"))

    def rank(i: int) -> tuple[float, TargetKey, int]:
        return -predictions[targets[i].key].quality, targets[i].key, i

    ranked = sorted(available, key=rank)
    pinned = next((i for i in available if targets[i].key == pin), None)
    if pinned is not None:
        pred = predictions[targets[pinned].key]
        return Decision(
            pinned,
            tuple(i for i in ranked if i != pinned),
            format_decision_label("pinned", quality=pred.quality),
        )

    latency_ok = [i for i in available if latency_passes(targets[i].key, latency_evidence, cfg)]
    if not latency_ok:
        # Every known target failed the latency gate (or unknowns were excluded).
        primary = ranked[0]
        pred = predictions[targets[primary].key]
        kind = (
            "latency-unknown-excluded"
            if cfg.unknown_latency == "exclude"
            and (latency_evidence is None or all(targets[i].key not in latency_evidence for i in available))
            else "latency-constrained"
        )
        return Decision(
            primary,
            tuple(i for i in ranked if i != primary),
            format_decision_label(kind, quality=pred.quality),
        )

    costs = {
        i: estimated_cost(
            targets[i], predictions[targets[i].key], input_tokens, cfg, cache=cache
        )
        for i in latency_ok
    }
    above = [
        i
        for i in latency_ok
        if predictions[targets[i].key].quality >= floor and costs[i] is not None
    ]
    if above:
        primary = min(above, key=lambda i: (float(costs[i] or 0), rank(i)))
        pred = predictions[targets[primary].key]
        label = format_decision_label(
            "cheapest-above-floor",
            quality=pred.quality,
            cost=float(costs[primary] or 0),
        )
    else:
        # Prefer highest quality among latency-eligible; fall back to global ranked.
        pool = latency_ok or available
        primary = min(pool, key=rank)
        pred = predictions[targets[primary].key]
        met_floor_unknown_price = any(
            predictions[targets[i].key].quality >= floor for i in latency_ok
        )
        if met_floor_unknown_price:
            label = format_decision_label("unknown-pricing", quality=pred.quality)
        elif cfg.latency_p95_ms_max is not None and len(latency_ok) < len(available):
            # Floor miss after latency filtering — surface latency when it removed
            # the only floor-meeting candidates.
            floor_met_before = any(
                predictions[targets[i].key].quality >= floor for i in available
            )
            kind = "latency-constrained" if floor_met_before else "no-candidate-met-floor"
            label = format_decision_label(kind, quality=pred.quality)
        else:
            label = format_decision_label("no-candidate-met-floor", quality=pred.quality)

    if exploration_allowed and cfg.explore_rate > 0 and rng.random() < cfg.explore_rate:
        primary = rng.choice(latency_ok)
        # Keep exact ``lrp:explore`` for usage-import / CLI filters (no q/c suffix).
        label = "lrp:explore"
    return Decision(primary, tuple(i for i in ranked if i != primary), label)


def floor_sweep_values(
    cfg: GroupConfig, *, projects: Sequence[str] | None = None
) -> list[tuple[str, float]]:
    """Return (scope, floor) pairs for operator floor sweeps.

    Includes the group default and each configured project override. Synthetic
    project names only; never invents production identities.
    """
    rows: list[tuple[str, float]] = [("*", float(cfg.quality_floor))]
    names = list(projects) if projects is not None else sorted(cfg.floors_by_project)
    for name in names:
        key = name.strip()
        if not key:
            continue
        rows.append((key, effective_quality_floor(cfg, key)))
    return rows
