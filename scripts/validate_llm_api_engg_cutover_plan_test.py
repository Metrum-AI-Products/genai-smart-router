#!/usr/bin/env python3
"""Regression tests for the hostname-specific cutover planning contract."""
from __future__ import annotations

import importlib.util
import json
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_llm_api_engg_cutover_plan.py"
PLAN = ROOT / "deploy" / "release" / "fixtures" / "llm-api-engg-cutover-plan.template.json"
EVIDENCE = ROOT / "deploy" / "release" / "fixtures" / "llm-api-engg-cutover-evidence.template.json"


def load_validator():
    spec = importlib.util.spec_from_file_location("validate_llm_api_engg_cutover_plan", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


VALIDATOR = load_validator()
PLAN_SCHEMA = VALIDATOR.load_json(VALIDATOR.PLAN_SCHEMA, "plan schema")
EVIDENCE_SCHEMA = VALIDATOR.load_json(VALIDATOR.EVIDENCE_SCHEMA, "evidence schema")


def clone(value: object) -> object:
    return json.loads(json.dumps(value))


def rejected(plan: object) -> None:
    try:
        VALIDATOR.validate_plan(plan, PLAN_SCHEMA)
    except ValueError:
        return
    raise AssertionError("expected planning manifest rejection")


def set_path(value: dict[str, object], path: tuple[str, ...], replacement: object) -> None:
    current = value
    for component in path[:-1]:
        child = current[component]
        assert isinstance(child, dict)
        current = child
    current[path[-1]] = replacement


def main() -> int:
    plan = json.loads(PLAN.read_text(encoding="utf-8"))
    evidence = json.loads(EVIDENCE.read_text(encoding="utf-8"))
    result = VALIDATOR.validate_plan(plan, PLAN_SCHEMA)
    assert result["outcome"] == "planning_contract_valid_no_apply"
    assert result["hostname"] == "llm-api-engg.metrum.ai"
    assert result["readiness"] == "awaiting_protected_authorization"
    VALIDATOR.validate_evidence(evidence, EVIDENCE_SCHEMA, plan["plan_id"])

    for path, replacement in (
        (("authority",), "apply"),
        (("source", "hostname"), "llm-api.apps.metrum.ai"),
        (("target", "hostname"), "llm-api.apps.metrum.ai"),
        (("target", "replicas"), 2),
        (("target", "deployment_strategy"), "RollingUpdate"),
        (("traffic", "no_dual_writers"), False),
        (("traffic", "max_canary_percent"), 10),
        (("migration", "reverse_migration_allowed"), True),
        (("migration", "reprice_historical_usage"), True),
        (("acceptance", "metrics_admin_isolation_required"), False),
        (("acceptance", "ordinary_metrics_status"), "200"),
        (("dns", "staging_hostname_forbidden"), False),
        (("rollback", "cleanup_separately_approved"), False),
        (("gate_record", "decision"), "go-review-required"),
        (("gate_record", "authority_after_decision"), "eks-fleet"),
    ):
        candidate = clone(plan)
        assert isinstance(candidate, dict)
        set_path(candidate, path, replacement)
        rejected(candidate)

    long_window = clone(plan)
    long_window["change_control"]["valid_until"] = "2029-01-01T01:00:01Z"
    rejected(long_window)
    same_rollback = clone(plan)
    same_rollback["rollback"]["known_good_image_digest"] = same_rollback["release"]["image_digest"]
    rejected(same_rollback)
    secret_reference = clone(plan)
    secret_reference["release"]["promotion_manifest_reference"] = "aws-secretsmanager:///private/runtime"
    rejected(secret_reference)
    extra = clone(plan)
    extra["execution"] = {"apply": True}
    rejected(extra)

    mismatched_evidence = clone(evidence)
    mismatched_evidence["plan_id"] = "another-plan"
    try:
        VALIDATOR.validate_evidence(mismatched_evidence, EVIDENCE_SCHEMA, plan["plan_id"])
    except ValueError:
        pass
    else:
        raise AssertionError("evidence for another plan was accepted")
    contradictory_evidence = clone(evidence)
    contradictory_evidence["outcome"] = "passed"
    try:
        VALIDATOR.validate_evidence(contradictory_evidence, EVIDENCE_SCHEMA, plan["plan_id"])
    except ValueError:
        pass
    else:
        raise AssertionError("contradictory passed evidence was accepted")

    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        manifest_path = temporary_root / "manifest.json"
        evidence_path = temporary_root / "evidence.json"
        manifest_path.write_text(json.dumps(plan), encoding="utf-8")
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        completed = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "--manifest",
                str(manifest_path),
                "--evidence-record",
                str(evidence_path),
            ],
            text=True,
            capture_output=True,
            check=False,
        )
        assert completed.returncode == 0, completed.stderr
        assert "planning_contract_valid_no_apply" in completed.stdout
        assert '"evidence_records_validated": 1' in completed.stdout

        duplicate_path = temporary_root / "duplicate.json"
        duplicate_path.write_text(
            '{"schema_version":1,"schema_version":2,"secret":"sk-' + ("x" * 24) + '"}',
            encoding="utf-8",
        )
        completed = subprocess.run(
            ["python3", str(SCRIPT), "--manifest", str(duplicate_path)],
            text=True,
            capture_output=True,
            check=False,
        )
        output = completed.stdout + completed.stderr
        assert completed.returncode == 2
        assert "sk-" not in output
        assert "artifact is invalid or unsafe" in output

    print("llm-api-engg cutover planning contract tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
