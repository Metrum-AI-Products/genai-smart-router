#!/usr/bin/env python3
"""Validate the enterprise license SKU catalog used by docs and operations."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "docs" / "enterprise-license-skus.json"
EXPECTED_SKUS = {
    "eval-72h",
    "pilot-30d",
    "enterprise-annual",
    "credit-pack-5m",
    "credit-pack-25m",
    "marketplace-seat",
}
DISALLOWED_GROUP_NAMES = {
    "default",
    "fast",
    "small",
    "medium",
    "high",
    "big-coder",
    "vision",
}


def fail(message: str) -> None:
    print(f"license SKU catalog check failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_catalog() -> dict[str, Any]:
    try:
        raw = CATALOG.read_text(encoding="utf-8")
    except OSError as exc:
        fail(f"cannot read {CATALOG}: {exc}")
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        fail(f"invalid JSON: {exc}")
    if not isinstance(data, dict):
        fail("catalog root must be an object")
    return data


def assert_no_disallowed_group_names(value: Any, path: str = "$") -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            assert_no_disallowed_group_names(nested, f"{path}.{key}")
    elif isinstance(value, list):
        for idx, nested in enumerate(value):
            assert_no_disallowed_group_names(nested, f"{path}[{idx}]")
    elif isinstance(value, str) and value in DISALLOWED_GROUP_NAMES:
        fail(f"{path} uses deployment-defined model group name {value!r}")


def main() -> int:
    data = load_catalog()
    if data.get("schema_version") != 1:
        fail("schema_version must be 1")
    if data.get("metadata", {}).get("runtime_dependency_issue") != 159:
        fail("runtime_dependency_issue must be 159")

    names = data.get("entitlement_names")
    if not isinstance(names, dict):
        fail("entitlement_names must be an object")
    features = set(names.get("features", []))
    limits = set(names.get("limits", []))
    if "routing" not in features:
        fail("routing feature must be declared")
    for required in ("max_total_tokens", "window_tokens", "max_instances", "allowed_skins"):
        if required not in limits:
            fail(f"{required} limit must be declared")

    skus = data.get("skus")
    if not isinstance(skus, list):
        fail("skus must be a list")
    seen = {sku.get("sku") for sku in skus if isinstance(sku, dict)}
    if seen != EXPECTED_SKUS:
        fail(f"SKU set mismatch: got {sorted(seen)!r}")

    for sku in skus:
        if not isinstance(sku, dict):
            fail("each SKU must be an object")
        name = sku.get("sku")
        template = sku.get("license_template")
        if not isinstance(name, str) or not name:
            fail("each SKU needs a non-empty sku")
        if template != name:
            fail(f"{name}: license_template must match sku")
        feature_list = sku.get("features")
        if not isinstance(feature_list, list) or "routing" not in feature_list:
            fail(f"{name}: features must include routing")
        unknown_features = set(feature_list) - features
        unknown_addons = set(sku.get("add_on_features", [])) - features
        if unknown_features or unknown_addons:
            fail(f"{name}: unknown features {sorted(unknown_features | unknown_addons)!r}")
        limit_obj = sku.get("limits")
        if not isinstance(limit_obj, dict) or not limit_obj:
            fail(f"{name}: limits must be a non-empty object")
        unknown_limits = set(limit_obj) - limits
        if unknown_limits:
            fail(f"{name}: unknown limits {sorted(unknown_limits)!r}")
        deployment = sku.get("deployment")
        if not isinstance(deployment, dict) or not deployment.get("mode"):
            fail(f"{name}: deployment.mode is required")
        if "term" not in sku or "duration" not in sku["term"]:
            fail(f"{name}: term.duration is required")

    assert_no_disallowed_group_names(data)
    print("license SKU catalog check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
