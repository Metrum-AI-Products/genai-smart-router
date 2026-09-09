# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Allowlisted plugin bodies executed only inside the isolated verifier worker.

This module is concatenated into the bubblewrap Python -c payload. It must stay
stdlib-only and must not import the host lrp package.
"""

from __future__ import annotations

import json
import math
from collections.abc import Callable
from typing import Any


def _contains_v1(params: dict[str, Any], content: str) -> bool:
    needle = params.get("needle")
    if not isinstance(needle, str) or not needle:
        raise TypeError("invalid_params")
    return needle in content


def _json_equals_v1(params: dict[str, Any], content: str) -> bool:
    if "expected" not in params:
        raise ValueError("invalid_params")
    return bool(json.loads(content) == params["expected"])


def _numeric_equals_v1(params: dict[str, Any], content: str) -> bool:
    raw_expected = params.get("expected")
    if isinstance(raw_expected, bool) or not isinstance(raw_expected, (int, float)):
        raise TypeError("invalid_params")
    expected = float(raw_expected)
    raw_tol = params.get("abs_tol", 0.0)
    if isinstance(raw_tol, bool) or not isinstance(raw_tol, (int, float)) or raw_tol < 0:
        raise TypeError("invalid_params")
    abs_tol = float(raw_tol)
    actual = float(content.strip())
    return math.isclose(actual, expected, rel_tol=0.0, abs_tol=abs_tol)


ALLOWED_PLUGINS: dict[str, Callable[[dict[str, Any], str], bool]] = {
    "contains_v1": _contains_v1,
    "json_equals_v1": _json_equals_v1,
    "numeric_equals_v1": _numeric_equals_v1,
}


def run_plugin(plugin_id: str, params: dict[str, Any], content: str) -> bool:
    runner = ALLOWED_PLUGINS.get(plugin_id)
    if runner is None:
        raise ValueError("unknown_plugin")
    if not isinstance(params, dict):
        raise TypeError("invalid_params")
    return bool(runner(params, content))
