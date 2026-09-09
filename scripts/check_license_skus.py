#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

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
    "hosted-pilot-30d",
    "enterprise-annual",
    "credit-pack-5m",
    "credit-pack-25m",
    "hosted-instance-monthly",
    "hosted-instance-annual",
    "hosted-instance-additional-monthly",
    "marketplace-seat",
    "oss-self-managed",
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
BILLING_KINDS = {
    "payment",
    "top_up",
    "subscription",
    "subscription_addon",
    "invoice_only",
}
STRIPE_MODES = {"payment", "subscription", "none"}
PRICING_STATUSES = {"placeholder_assumption"}


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


def validate_commercial_fields(sku: dict[str, Any], name: str) -> None:
    billing_kind = sku.get("billing_kind")
    if billing_kind not in BILLING_KINDS:
        fail(f"{name}: billing_kind must be one of {sorted(BILLING_KINDS)}")
    stripe_mode = sku.get("stripe_mode")
    if stripe_mode not in STRIPE_MODES:
        fail(f"{name}: stripe_mode must be one of {sorted(STRIPE_MODES)}")
    if not isinstance(sku.get("self_serve_stripe"), bool):
        fail(f"{name}: self_serve_stripe must be a boolean")
    if not isinstance(sku.get("auto_provision_instance"), bool):
        fail(f"{name}: auto_provision_instance must be a boolean")
    if sku.get("pricing_status") not in PRICING_STATUSES:
        fail(f"{name}: pricing_status must be placeholder_assumption")
    placeholder = sku.get("price_placeholder")
    if not isinstance(placeholder, dict):
        fail(f"{name}: price_placeholder must be an object")
    for field in ("currency", "unit", "interval", "assumption_notes"):
        if not isinstance(placeholder.get(field), str) or not placeholder.get(field):
            fail(f"{name}: price_placeholder.{field} must be a non-empty string")
    amount = placeholder.get("amount_usd")
    if not isinstance(amount, (int, float)) or amount < 0:
        fail(f"{name}: price_placeholder.amount_usd must be a non-negative number")

    self_serve = sku["self_serve_stripe"]
    stripe = sku.get("stripe")
    if not self_serve:
        if stripe not in (None, {}, "none"):
            # allow explicit null/omitted; reject product maps when not self-serve
            if isinstance(stripe, dict) and (stripe.get("product") or stripe.get("price")):
                fail(f"{name}: non-self-serve SKUs must not declare stripe.product/price")
        return

    if stripe_mode == "none":
        fail(f"{name}: self_serve_stripe true requires stripe_mode payment or subscription")
    if not isinstance(stripe, dict):
        fail(f"{name}: self_serve SKUs require a stripe object")
    product = stripe.get("product")
    price = stripe.get("price")
    if not isinstance(product, dict) or not product.get("name"):
        fail(f"{name}: stripe.product.name is required")
    if not isinstance(price, dict):
        fail(f"{name}: stripe.price is required")
    if price.get("lookup_key") != name:
        fail(f"{name}: stripe.price.lookup_key must equal sku")
    unit_amount = price.get("unit_amount")
    if not isinstance(unit_amount, int) or unit_amount < 0:
        fail(f"{name}: stripe.price.unit_amount must be a non-negative integer (cents)")
    expected_cents = int(round(float(amount) * 100))
    if unit_amount != expected_cents:
        fail(f"{name}: stripe.price.unit_amount {unit_amount} must match amount_usd*100={expected_cents}")
    if price.get("currency") != placeholder.get("currency"):
        fail(f"{name}: stripe.price.currency must match price_placeholder.currency")
    recurring = price.get("recurring")
    if stripe_mode == "subscription":
        if not isinstance(recurring, dict) or recurring.get("interval") not in {"month", "year"}:
            fail(f"{name}: subscription SKUs require stripe.price.recurring.interval month|year")
        if placeholder.get("interval") != recurring.get("interval"):
            fail(f"{name}: price_placeholder.interval must match recurring.interval")
    elif recurring not in (None, {}):
        fail(f"{name}: payment/top_up SKUs must not set stripe.price.recurring")


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

    canonical_templates = {
        sku.get("sku")
        for sku in skus
        if isinstance(sku, dict) and sku.get("sku") == sku.get("license_template")
    }

    for sku in skus:
        if not isinstance(sku, dict):
            fail("each SKU must be an object")
        name = sku.get("sku")
        template = sku.get("license_template")
        if not isinstance(name, str) or not name:
            fail("each SKU needs a non-empty sku")
        if not isinstance(template, str) or not template:
            fail(f"{name}: license_template is required")
        if template not in canonical_templates:
            fail(f"{name}: license_template {template!r} must resolve to a canonical template SKU")
        feature_list = sku.get("features")
        if not isinstance(feature_list, list) or "routing" not in feature_list:
            fail(f"{name}: features must include routing")
        unknown_features = set(feature_list) - features
        unknown_addons = set(sku.get("add_on_features", [])) - features
        if unknown_features or unknown_addons:
            fail(f"{name}: unknown features {sorted(unknown_features | unknown_addons)!r}")
        limit_obj = sku.get("limits")
        if name == "oss-self-managed":
            if limit_obj != {}:
                fail("oss-self-managed: limits must be empty")
        else:
            if not isinstance(limit_obj, dict) or not limit_obj:
                fail(f"{name}: limits must be a non-empty object")
            unknown_limits = set(limit_obj) - limits
            if unknown_limits:
                fail(f"{name}: unknown limits {sorted(unknown_limits)!r}")
            validate_commercial_fields(sku, name)
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
