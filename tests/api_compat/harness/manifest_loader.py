# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Load and validate per-catalog compatibility manifests."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml

from .constants import DISPOSITIONS

MANIFEST_DIR = Path(__file__).resolve().parents[1] / "manifest"
SCHEMA_PATH = MANIFEST_DIR / "schema.json"

REQUIRED_FIELDS = (
    "id",
    "priority",
    "owner",
    "component",
    "disposition",
    "assertions",
)

OPTIONAL_FIELDS = frozenset(
    {
        "source_url",
        "reviewed_date",
        "sdk_client_version",
        "inbound_route",
        "outbound_dialect",
        "capability_profile",
        "transport",
        "evidence_paths",
        "test_id",
        "rationale",
    }
)


class ManifestError(ValueError):
    """Raised when a manifest row fails validation."""


def load_schema(path: Path | None = None) -> dict[str, Any]:
    return json.loads((path or SCHEMA_PATH).read_text())


def _validate_case(case: dict[str, Any], source: Path, index: int) -> None:
    where = f"{source.name}[{index}]"
    if not isinstance(case, dict):
        raise ManifestError(f"{where}: case must be an object")
    missing = [key for key in REQUIRED_FIELDS if key not in case]
    if missing:
        raise ManifestError(f"{where}: missing required fields: {missing}")
    for key in case:
        if key not in REQUIRED_FIELDS and key not in OPTIONAL_FIELDS and not key.startswith("_"):
            raise ManifestError(f"{where}: unknown field {key!r}")
    disposition = case["disposition"]
    if disposition not in DISPOSITIONS:
        raise ManifestError(
            f"{where}: disposition {disposition!r} must be one of {sorted(DISPOSITIONS)}"
        )
    if not isinstance(case["assertions"], list) or not case["assertions"]:
        raise ManifestError(f"{where}: assertions must be a non-empty list")
    # A skip is never a pass: blocked/unsupported need evidence or rationale.
    if disposition in {"blocked", "unsupported"}:
        if not case.get("evidence_paths") and not case.get("rationale"):
            raise ManifestError(
                f"{where}: disposition {disposition!r} requires evidence_paths or rationale"
            )


def load_manifest_file(path: Path) -> list[dict[str, Any]]:
    raw = yaml.safe_load(path.read_text())
    if raw is None:
        return []
    if not isinstance(raw, dict) or "cases" not in raw:
        raise ManifestError(f"{path.name}: top-level object must contain 'cases'")
    cases = raw["cases"]
    if not isinstance(cases, list):
        raise ManifestError(f"{path.name}: cases must be a list")
    validated: list[dict[str, Any]] = []
    for index, case in enumerate(cases):
        _validate_case(case, path, index)
        validated.append(case)
    return validated


def load_all_manifests(directory: Path | None = None) -> list[dict[str, Any]]:
    root = directory or MANIFEST_DIR
    if not root.is_dir():
        raise ManifestError(f"manifest directory missing: {root}")
    load_schema(root / "schema.json" if (root / "schema.json").is_file() else None)
    cases: list[dict[str, Any]] = []
    seen: set[str] = set()
    for path in sorted(root.glob("*.yaml")):
        for case in load_manifest_file(path):
            case_id = case["id"]
            if case_id in seen:
                raise ManifestError(f"duplicate case id {case_id!r} in {path.name}")
            seen.add(case_id)
            cases.append(case)
    return cases


def load_manifests(directory: Path | None = None) -> list[dict[str, Any]]:
    """Alias used by tests."""
    return load_all_manifests(directory)


def cases_by_id(directory: Path | None = None) -> dict[str, dict[str, Any]]:
    return {case["id"]: case for case in load_all_manifests(directory)}
