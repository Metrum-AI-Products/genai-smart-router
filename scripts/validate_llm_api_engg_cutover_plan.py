#!/usr/bin/env python3
"""Validate secret-free llm-api-engg.metrum.ai cutover planning artifacts.

This offline command only validates planning metadata. It cannot deploy, read
cloud state, migrate data, change DNS, access secrets, or authorize a cutover.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
PLAN_SCHEMA = ROOT / "deploy" / "release" / "llm-api-engg-cutover-plan.schema.json"
EVIDENCE_SCHEMA = ROOT / "deploy" / "release" / "llm-api-engg-cutover-evidence.schema.json"
SECRET_SHAPED = re.compile(
    r"""(?ix)
    (
      \b(?:sk-[A-Za-z0-9_-]{20,}|xai-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|AIza[0-9A-Za-z_-]{20,})\b
      |-----BEGIN[ ](?:RSA[ ]|EC[ ]|OPENSSH[ ])?PRIVATE[ ]KEY-----
      |(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?):\/\/
      |aws-(?:ssm|secretsmanager):\/\/
      |\b(?:authorization|password|api[_-]?key|bearer|token[_-]?hash)\s*[:=]
    )
    """
)


class DuplicateJSONMemberError(ValueError):
    """Raised when JSON input contains an ambiguous duplicate member."""


def reject_duplicate_members(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateJSONMemberError("duplicate JSON member")
        result[key] = value
    return result


def load_json(path: Path, label: str) -> object:
    try:
        return json.loads(path.read_bytes(), object_pairs_hook=reject_duplicate_members)
    except DuplicateJSONMemberError as exc:
        raise ValueError(f"{label} contains duplicate JSON members") from exc
    except (OSError, json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise ValueError(f"{label} is unavailable or not valid JSON") from exc


def resolve_ref(root: dict[str, object], reference: object) -> dict[str, object]:
    if not isinstance(reference, str) or not reference.startswith("#/"):
        raise ValueError("schema contains an unsupported reference")
    current: object = root
    for component in reference[2:].split("/"):
        if not isinstance(current, dict) or component not in current:
            raise ValueError("schema contains an unresolved reference")
        current = current[component]
    if not isinstance(current, dict):
        raise ValueError("schema reference does not resolve to an object")
    return current


def parse_timestamp(value: object, label: str) -> dt.datetime:
    if not isinstance(value, str):
        raise ValueError(f"{label} must be an RFC3339 timestamp")
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError(f"{label} must be an RFC3339 timestamp") from exc
    if parsed.tzinfo is None or parsed.utcoffset() is None:
        raise ValueError(f"{label} must include a UTC offset")
    return parsed.astimezone(dt.timezone.utc)


def validate_schema(value: object, node: dict[str, object], root: dict[str, object], path: str) -> None:
    if "$ref" in node:
        validate_schema(value, resolve_ref(root, node["$ref"]), root, path)
    if "const" in node and value != node["const"]:
        raise ValueError(f"{path} does not match its required value")
    if "enum" in node:
        allowed = node["enum"]
        if not isinstance(allowed, list) or value not in allowed:
            raise ValueError(f"{path} is not an approved value")

    expected = node.get("type")
    if expected == "object":
        if not isinstance(value, dict):
            raise ValueError(f"{path} must be an object")
        required = node.get("required", [])
        properties = node.get("properties", {})
        if not isinstance(required, list) or not isinstance(properties, dict):
            raise ValueError("schema object contract is invalid")
        missing = [name for name in required if name not in value]
        if missing:
            raise ValueError(f"{path} is missing required fields")
        if node.get("additionalProperties") is False and not set(value).issubset(properties):
            raise ValueError(f"{path} contains an unapproved field")
        for name, child in properties.items():
            if name in value:
                if not isinstance(child, dict):
                    raise ValueError("schema property contract is invalid")
                validate_schema(value[name], child, root, f"{path}.{name}")
    elif expected == "array":
        if not isinstance(value, list):
            raise ValueError(f"{path} must be an array")
    elif expected == "string":
        if not isinstance(value, str):
            raise ValueError(f"{path} must be a string")
    elif expected == "integer":
        if type(value) is not int:
            raise ValueError(f"{path} must be an integer")
    elif expected == "boolean":
        if type(value) is not bool:
            raise ValueError(f"{path} must be a boolean")
    elif expected is not None:
        raise ValueError("schema contains an unsupported type")

    pattern = node.get("pattern")
    if pattern is not None:
        if not isinstance(value, str) or re.fullmatch(str(pattern), value) is None:
            raise ValueError(f"{path} has an invalid safe format")
    if node.get("format") == "date-time":
        parse_timestamp(value, path)
    if "minimum" in node and (type(value) is not int or value < int(node["minimum"])):
        raise ValueError(f"{path} is below its minimum")
    if "maximum" in node and (type(value) is not int or value > int(node["maximum"])):
        raise ValueError(f"{path} is above its maximum")


def scan_secret_shaped(value: object) -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            if SECRET_SHAPED.search(key):
                raise ValueError("artifact contains a secret-shaped field or value")
            scan_secret_shaped(child)
    elif isinstance(value, list):
        for child in value:
            scan_secret_shaped(child)
    elif isinstance(value, str) and SECRET_SHAPED.search(value):
        raise ValueError("artifact contains a secret-shaped field or value")


def validate_plan(plan: object, schema: dict[str, object]) -> dict[str, object]:
    validate_schema(plan, schema, schema, "plan")
    scan_secret_shaped(plan)
    if not isinstance(plan, dict):
        raise ValueError("plan must be an object")

    change_control = plan["change_control"]
    rollback = plan["rollback"]
    release = plan["release"]
    target = plan["target"]
    gate_record = plan["gate_record"]
    assert isinstance(change_control, dict)
    assert isinstance(rollback, dict)
    assert isinstance(release, dict)
    assert isinstance(target, dict)
    assert isinstance(gate_record, dict)

    evaluated_at = parse_timestamp(change_control["evaluated_at"], "plan.change_control.evaluated_at")
    valid_until = parse_timestamp(change_control["valid_until"], "plan.change_control.valid_until")
    if valid_until <= evaluated_at or valid_until - evaluated_at > dt.timedelta(hours=1):
        raise ValueError("change-control validity window must be greater than zero and no more than one hour")
    deadline = parse_timestamp(rollback["decision_deadline"], "plan.rollback.decision_deadline")
    if deadline < valid_until:
        raise ValueError("rollback decision deadline must not precede change-control validity expiry")
    if release["image_digest"].rsplit("@", 1)[1] == rollback["known_good_image_digest"].rsplit("@", 1)[1]:
        raise ValueError("known-good rollback image must have a distinct content digest")
    if target["production_profile_authorized"] is False and (
        gate_record["decision"] != "no-go"
        or gate_record["authority_after_decision"] != "ec2-compose"
    ):
        raise ValueError("an unauthorized production profile must remain no-go with Compose authority")
    if gate_record["decision"] == "rollback" and gate_record["rollback_window_decision"] != "rollback-invoked":
        raise ValueError("a rollback decision must record rollback-invoked")

    readiness = (
        "awaiting_protected_authorization"
        if target["production_profile_authorized"] is False
        else "authorization_recorded_requires_protected_execution_review"
    )
    return {
        "outcome": "planning_contract_valid_no_apply",
        "hostname": "llm-api-engg.metrum.ai",
        "plan_id": plan["plan_id"],
        "readiness": readiness,
    }


def validate_evidence(record: object, schema: dict[str, object], plan_id: object) -> None:
    validate_schema(record, schema, schema, "evidence")
    scan_secret_shaped(record)
    if not isinstance(record, dict) or record["plan_id"] != plan_id:
        raise ValueError("evidence.plan_id must match the planning manifest")
    if record.get("outcome") == "passed" and record.get("status_class") in {"not-run", "mismatch"}:
        raise ValueError("passed evidence cannot have a not-run or mismatch status")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--evidence-record", action="append", default=[])
    args = parser.parse_args()
    try:
        plan_schema = load_json(PLAN_SCHEMA, "plan schema")
        evidence_schema = load_json(EVIDENCE_SCHEMA, "evidence schema")
        if not isinstance(plan_schema, dict) or not isinstance(evidence_schema, dict):
            raise ValueError("planning schemas must be JSON objects")
        plan = load_json(Path(args.manifest), "planning manifest")
        result = validate_plan(plan, plan_schema)
        assert isinstance(plan, dict)
        for path in args.evidence_record:
            record = load_json(Path(path), "evidence record")
            validate_evidence(record, evidence_schema, plan["plan_id"])
        result["evidence_records_validated"] = len(args.evidence_record)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (AssertionError, ValueError):
        print("cutover planning validation failed: artifact is invalid or unsafe", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
