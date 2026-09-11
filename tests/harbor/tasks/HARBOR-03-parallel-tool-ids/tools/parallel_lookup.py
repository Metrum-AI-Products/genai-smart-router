# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Parallel same-name lookup simulation for HARBOR-03."""

from __future__ import annotations

import random
from dataclasses import dataclass


@dataclass(frozen=True)
class LookupRequest:
    call_id: str
    key: str


@dataclass(frozen=True)
class LookupResult:
    call_id: str
    name: str
    value: str


VALUES = {
    "alpha": "value-alpha",
    "bravo": "value-bravo",
    "charlie": "value-charlie",
}


def default_requests() -> list[LookupRequest]:
    return [
        LookupRequest("call_a", "alpha"),
        LookupRequest("call_b", "bravo"),
        LookupRequest("call_c", "charlie"),
    ]


def execute_parallel(requests: list[LookupRequest], *, seed: int = 42) -> list[LookupResult]:
    """Return results in randomized completion order; IDs must still match."""
    results = [
        LookupResult(call_id=req.call_id, name="lookup", value=VALUES[req.key])
        for req in requests
    ]
    rng = random.Random(seed)
    shuffled = list(results)
    rng.shuffle(shuffled)
    return shuffled


def map_by_call_id(results: list[LookupResult]) -> dict[str, str]:
    return {item.call_id: item.value for item in results}
