#!/usr/bin/env python3
"""Regression tests for the offline production promotion manifest gate."""
from __future__ import annotations

import datetime as dt
import hashlib
import importlib.util
import json
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_production_promotion.py"
SCHEMA = ROOT / "deploy" / "release" / "production-release-manifest.schema.json"
FIXTURE = ROOT / "deploy" / "release" / "fixtures" / "production-release-manifest.valid.json"
EVIDENCE = FIXTURE.parent / "evidence"
SCHEMA_KEYWORDS = frozenset(
    {
        "$schema",
        "$defs",
        "$ref",
        "additionalProperties",
        "allOf",
        "const",
        "description",
        "enum",
        "format",
        "maximum",
        "minimum",
        "pattern",
        "properties",
        "required",
        "title",
        "type",
    }
)


def load_validator():
    spec = importlib.util.spec_from_file_location("validate_production_promotion", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


VALIDATOR = load_validator()
TEST_NOW = VALIDATOR.parse_time("2029-01-01T00:00:00Z", "test now")


def clone(value: object):
    return json.loads(json.dumps(value))


def failure(payload: dict[str, object], evidence_root: Path = EVIDENCE) -> str:
    try:
        VALIDATOR.validate(payload, TEST_NOW, evidence_root)
    except ValueError as exc:
        return str(exc)
    raise AssertionError("expected validation to fail")


def encoded_json(payload: dict[str, object]) -> bytes:
    return json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")


def timestamp(value: dt.datetime) -> str:
    return value.astimezone(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def supply_record(manifest: dict[str, object]) -> dict[str, object]:
    supply_chain = manifest["supply_chain"]
    assert isinstance(supply_chain, dict)
    record = supply_chain["validation_result"]
    assert isinstance(record, dict)
    return record


def rebind_evidence_payload(record: dict[str, object], evidence_root: Path, payload: dict[str, object]) -> None:
    """Replace one safe evidence result and update only its hash pin."""
    reference = record["reference"]
    assert isinstance(reference, str)
    rendered = encoded_json(payload)
    (evidence_root / reference).write_bytes(rendered)
    record["sha256"] = hashlib.sha256(rendered).hexdigest()


def refresh_evidence_timestamps(manifest: dict[str, object], evidence_root: Path, now: dt.datetime) -> None:
    """Make copied synthetic evidence current for the real-clock wrapper test."""
    migration = manifest["migration"]
    assert isinstance(migration, dict)
    records = (manifest["staging_evidence"], supply_record(manifest), migration["rehearsal_evidence"])
    for record in records:
        assert isinstance(record, dict)
        reference = record["reference"]
        assert isinstance(reference, str)
        payload = json.loads((evidence_root / reference).read_bytes())
        payload["timestamp"] = timestamp(now)
        rebind_evidence_payload(record, evidence_root, payload)


def promotion_make(manifest: str, evidence_root: str) -> subprocess.CompletedProcess[str]:
    """Invoke the documented wrapper with literal parent-environment inputs."""
    environment = os.environ.copy()
    environment.update(
        {
            "PRODUCTION_RELEASE_MANIFEST": manifest,
            "PRODUCTION_EVIDENCE_ROOT": evidence_root,
        }
    )
    return subprocess.run(
        ["make", "production-promotion-validate"],
        cwd=ROOT,
        env=environment,
        text=True,
        capture_output=True,
        check=False,
    )


def resolve_ref(schema: dict[str, object], reference: object) -> dict[str, object]:
    assert isinstance(reference, str) and reference.startswith("#/")
    current: object = schema
    for component in reference[2:].split("/"):
        assert isinstance(current, dict) and component in current, f"unresolved schema reference: {reference}"
        current = current[component]
    assert isinstance(current, dict), f"schema reference is not an object: {reference}"
    return current


def assert_known_schema_keywords(node: object, root: dict[str, object]) -> None:
    """Reject unimplemented schema keywords before trusting the fixture test."""
    assert isinstance(node, dict), "schema node must be an object"
    unknown = set(node) - SCHEMA_KEYWORDS
    assert not unknown, f"unknown schema keyword: {sorted(unknown)!r}"
    if "$ref" in node:
        resolve_ref(root, node["$ref"])
    properties = node.get("properties", {})
    assert isinstance(properties, dict)
    for child in properties.values():
        assert_known_schema_keywords(child, root)
    definitions = node.get("$defs", {})
    assert isinstance(definitions, dict)
    for child in definitions.values():
        assert_known_schema_keywords(child, root)
    all_of = node.get("allOf", [])
    assert isinstance(all_of, list)
    for child in all_of:
        assert_known_schema_keywords(child, root)


def validate_schema_instance(value: object, node: dict[str, object], root: dict[str, object], path: str) -> None:
    """Validate the checked-in fixture with the schema's actual stdlib-only subset."""
    if "$ref" in node:
        validate_schema_instance(value, resolve_ref(root, node["$ref"]), root, path)
    for child in node.get("allOf", []):
        assert isinstance(child, dict)
        validate_schema_instance(value, child, root, path)
    if "const" in node:
        assert value == node["const"], f"{path} does not match const"
    if "enum" in node:
        assert value in node["enum"], f"{path} is not an approved enum value"
    expected_type = node.get("type")
    if expected_type == "object":
        assert isinstance(value, dict), f"{path} must be an object"
        required = node.get("required", [])
        assert isinstance(required, list)
        assert all(isinstance(field, str) and field in value for field in required), f"{path} is missing required fields"
        properties = node.get("properties", {})
        assert isinstance(properties, dict)
        if node.get("additionalProperties") is False:
            assert set(value).issubset(properties), f"{path} has an additional property"
        for field, child in properties.items():
            if field in value:
                assert isinstance(child, dict)
                validate_schema_instance(value[field], child, root, f"{path}.{field}")
    elif expected_type == "string":
        assert isinstance(value, str), f"{path} must be a string"
    elif expected_type == "integer":
        assert type(value) is int, f"{path} must be an integer"
    elif expected_type is not None:
        raise AssertionError(f"unsupported checked-in schema type: {expected_type!r}")
    if "pattern" in node:
        assert isinstance(value, str)
        assert re.search(str(node["pattern"]), value), f"{path} does not match pattern"
    if "format" in node:
        assert node["format"] == "date-time", f"unsupported checked-in schema format: {node['format']!r}"
        VALIDATOR.parse_time(value, path)
    if "minimum" in node:
        assert type(value) is int and value >= node["minimum"], f"{path} is below minimum"
    if "maximum" in node:
        assert type(value) is int and value <= node["maximum"], f"{path} is above maximum"


def assert_schema_fixture_contract(valid: dict[str, object]) -> None:
    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    assert_known_schema_keywords(schema, schema)
    validate_schema_instance(valid, schema, schema, "fixture")

    def assert_schema_rejected(candidate: dict[str, object], expected: str) -> None:
        try:
            validate_schema_instance(candidate, schema, schema, "fixture")
        except AssertionError as exc:
            assert expected in str(exc), str(exc)
        else:
            raise AssertionError(f"schema accepted invalid fixture: {expected}")

    missing_approval = clone(valid)
    del missing_approval["approval"]
    assert_schema_rejected(missing_approval, "missing required fields")
    failed_staging = clone(valid)
    failed_staging["staging_evidence"]["result"] = "failed"
    assert_schema_rejected(failed_staging, "does not match const")
    wrong_staging_reference = clone(valid)
    wrong_staging_reference["staging_evidence"]["reference"] = "evidence-promotion-plan.json"
    assert_schema_rejected(wrong_staging_reference, "does not match const")
    boolean_schema_version = clone(valid)
    boolean_schema_version["schema_version"] = True
    assert_schema_rejected(boolean_schema_version, "must be an integer")
    invalid_release_id = clone(valid)
    invalid_release_id["release_id"] = "invalid release id"
    assert_schema_rejected(invalid_release_id, "does not match pattern")
    oversized_canary = clone(valid)
    oversized_canary["canary"]["max_traffic_percent"] = 11
    assert_schema_rejected(oversized_canary, "above maximum")
    extra_field = clone(valid)
    extra_field["deployment"] = {"architecture": "linux/amd64"}
    try:
        validate_schema_instance(extra_field, schema, schema, "fixture")
    except AssertionError as exc:
        assert "additional property" in str(exc)
    else:
        raise AssertionError("schema accepted removed deployment/caller-architecture input")
    unknown_keyword = clone(schema)
    definitions = unknown_keyword["$defs"]
    assert isinstance(definitions, dict)
    supply_chain = definitions["supplyChain"]
    assert isinstance(supply_chain, dict)
    supply_chain["unknownKeyword"] = True
    try:
        assert_known_schema_keywords(unknown_keyword, unknown_keyword)
    except AssertionError as exc:
        assert "unknown schema keyword" in str(exc)
    else:
        raise AssertionError("schema keyword audit accepted an unknown keyword")


def alter_supply_result(
    valid: dict[str, object], transform, expected: str
) -> None:
    with tempfile.TemporaryDirectory() as temporary:
        evidence_root = Path(temporary) / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        candidate = clone(valid)
        record = supply_record(candidate)
        reference = record["reference"]
        assert isinstance(reference, str)
        payload = json.loads((evidence_root / reference).read_bytes())
        transform(payload)
        rebind_evidence_payload(record, evidence_root, payload)
        assert expected in failure(candidate, evidence_root.resolve())


def main() -> int:
    valid = json.loads(FIXTURE.read_text(encoding="utf-8"))
    assert_schema_fixture_contract(valid)
    assert VALIDATOR.validate(valid, TEST_NOW, EVIDENCE)["outcome"] == "review_required_no_production_apply"
    boolean_schema_version = clone(valid)
    boolean_schema_version["schema_version"] = True
    assert "schema_version 1" in failure(boolean_schema_version)
    for mutation, expected in (
        (("approval", "expires_at", "2020-01-01T00:00:00Z"), "expired"),
        (("canary", "max_traffic_percent", 11), "1 through 10"),
        (("artifact", "image_digest", "tag:latest"), "invalid safe format"),
        (("staging_evidence", "result", "failed"), "must be passed"),
        (("approval", "token", "never"), "exactly"),
    ):
        candidate = clone(valid)
        parent, key, value = mutation
        candidate[parent][key] = value
        assert expected in failure(candidate)
    direct_apply = clone(valid)
    direct_apply["apply"] = "production"
    assert "exactly" in failure(direct_apply)
    caller_architecture = clone(valid)
    caller_architecture["deployment"] = {"architecture": "linux/arm64"}
    assert "exactly" in failure(caller_architecture)
    duplicate_raw_supply_artifacts = clone(valid)
    duplicate_raw_supply_artifacts["supply_chain"]["sbom"] = {"reference": "not-allowed", "sha256": "a" * 64}
    assert "exactly" in failure(duplicate_raw_supply_artifacts)
    changed_evidence = clone(valid)
    supply_record(changed_evidence)["sha256"] = "0" * 64
    assert "does not match resolved evidence" in failure(changed_evidence)
    raw_staging_reference = clone(valid)
    raw_staging_reference["staging_evidence"]["reference"] = "evidence-promotion-plan.json"
    assert "fixed safe promotion-plan evidence" in failure(raw_staging_reference)

    # Production accepts only exact safe projections, never broad raw EKS or
    # rehearsal JSON. Extra fields are rejected structurally without echoing
    # their untrusted names or values.
    for evidence_kind, extra_fields in (
        (
            "staging_evidence",
            {
                "events": [{"name": "promotion_plan", "result": "review_required_no_production_apply"}],
                "aws_account_id": "123456789012",
                "authorization": "opaque-redacted-test-value",
            },
        ),
        (
            "migration.rehearsal_evidence",
            {
                "command_apply": "production",
                "authorization": "opaque-redacted-test-value",
            },
        ),
    ):
        with tempfile.TemporaryDirectory() as temporary:
            evidence_root = Path(temporary) / "evidence"
            shutil.copytree(EVIDENCE, evidence_root)
            candidate = clone(valid)
            if evidence_kind == "staging_evidence":
                record = candidate["staging_evidence"]
            else:
                migration = candidate["migration"]
                assert isinstance(migration, dict)
                record = migration["rehearsal_evidence"]
            assert isinstance(record, dict)
            reference = record["reference"]
            assert isinstance(reference, str)
            payload = json.loads((evidence_root / reference).read_bytes())
            payload.update(extra_fields)
            rebind_evidence_payload(record, evidence_root, payload)
            message = failure(candidate, evidence_root.resolve())
            assert "must contain exactly" in message
            for field_name in extra_fields:
                assert field_name not in message

    for evidence_kind in ("staging_evidence", "migration.rehearsal_evidence"):
        with tempfile.TemporaryDirectory() as temporary:
            evidence_root = Path(temporary) / "evidence"
            shutil.copytree(EVIDENCE, evidence_root)
            candidate = clone(valid)
            if evidence_kind == "staging_evidence":
                record = candidate["staging_evidence"]
            else:
                migration = candidate["migration"]
                assert isinstance(migration, dict)
                record = migration["rehearsal_evidence"]
            assert isinstance(record, dict)
            reference = record["reference"]
            assert isinstance(reference, str)
            payload = json.loads((evidence_root / reference).read_bytes())
            payload["schema_version"] = True
            rebind_evidence_payload(record, evidence_root, payload)
            assert "schema_version must be integer 1" in failure(candidate, evidence_root.resolve())

    for field_path in (
        ("release_id",),
        ("approval", "change_reference"),
        ("approval", "release_approved_by"),
        ("approval", "operations_approved_by"),
    ):
        candidate = clone(valid)
        target = candidate
        for field in field_path[:-1]:
            target = target[field]
        credential = "sk-" + "a" * 20
        target[field_path[-1]] = credential
        message = failure(candidate)
        assert "credential-shaped" in message
        assert credential not in message

    # Even if every protected projection is internally rebound to the same
    # syntactically valid digest, a credential-shaped artifact repository is
    # rejected before it can reach successful validator or CLI output.
    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        evidence_root = temporary_root / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        candidate = clone(valid)
        credential_digest = "registry.example/sk_test_" + "a" * 20 + "@sha256:" + "c" * 64
        candidate["artifact"]["image_digest"] = credential_digest
        records = [candidate["staging_evidence"], supply_record(candidate)]
        migration = candidate["migration"]
        assert isinstance(migration, dict)
        records.append(migration["rehearsal_evidence"])
        for record in records:
            assert isinstance(record, dict)
            reference = record["reference"]
            assert isinstance(reference, str)
            payload = json.loads((evidence_root / reference).read_bytes())
            payload["image_digest"] = credential_digest
            if "supply_chain" in payload:
                summary = payload["supply_chain"]
                assert isinstance(summary, dict)
                summary["image_digest"] = credential_digest
            rebind_evidence_payload(record, evidence_root, payload)
        message = failure(candidate, evidence_root.resolve())
        assert "credential-shaped" in message
        assert credential_digest not in message
        now = dt.datetime.now(dt.timezone.utc)
        refresh_evidence_timestamps(candidate, evidence_root, now)
        candidate["approval"]["issued_at"] = timestamp(now - dt.timedelta(minutes=1))
        candidate["approval"]["expires_at"] = timestamp(now + dt.timedelta(minutes=29))
        manifest_path = temporary_root / "manifest.json"
        manifest_path.write_text(json.dumps(candidate))
        cli = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "--manifest",
                str(manifest_path),
                "--evidence-root",
                str(evidence_root),
            ],
            text=True,
            capture_output=True,
        )
    output = cli.stdout + cli.stderr
    assert cli.returncode == 2
    assert credential_digest not in output
    assert "credential-shaped" in output

    approved_architecture = VALIDATOR.approved_target_architecture()
    mismatched_architecture = "linux/arm64" if approved_architecture != "linux/arm64" else "linux/amd64"
    alter_supply_result(
        valid,
        lambda payload: payload.update({"image_digest": "registry.example/router@sha256:" + "b" * 64}),
        "must bind the promoted artifact",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"image_digest": "registry.example/router@sha256:" + "b" * 64}),
        "summary must bind the promoted artifact",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"architecture": mismatched_architecture}),
        "policy-derived deployment architecture",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"signature_verified": False}),
        "signature and scan must both pass",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"scan_verdict": "fail"}),
        "signature and scan must both pass",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"scan_sha256": "A" * 64}),
        "invalid safe format",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].update({"caller_architecture": "linux/arm64"}),
        "must contain exactly",
    )

    with tempfile.TemporaryDirectory() as temporary:
        evidence_root = Path(temporary) / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        stale_supply = clone(valid)
        record = supply_record(stale_supply)
        reference = record["reference"]
        assert isinstance(reference, str)
        payload = json.loads((evidence_root / reference).read_bytes())
        payload["timestamp"] = "2028-12-30T23:59:59Z"
        rebind_evidence_payload(record, evidence_root, payload)
        assert "no more than 24 hours old" in failure(stale_supply, evidence_root.resolve())
    alter_supply_result(
        valid,
        lambda payload: payload.update({"timestamp": "2029-01-01T00:00:01Z"}),
        "must not be in the future",
    )
    alter_supply_result(
        valid,
        lambda payload: payload.pop("timestamp"),
        "must contain exactly",
    )
    alter_supply_result(
        valid,
        lambda payload: payload["supply_chain"].pop("scan_sha256"),
        "must contain exactly",
    )
    with tempfile.TemporaryDirectory() as temporary:
        evidence_root = Path(temporary) / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        duplicate_evidence = clone(valid)
        record = supply_record(duplicate_evidence)
        reference = record["reference"]
        assert isinstance(reference, str)
        duplicate_raw = b'{"outcome":"passed","run_id":"token=redacted-test-material","run_id":"safe"}\n'
        (evidence_root / reference).write_bytes(duplicate_raw)
        record["sha256"] = hashlib.sha256(duplicate_raw).hexdigest()
        message = failure(duplicate_evidence, evidence_root.resolve())
        assert "duplicate JSON members" in message
        assert "redacted-test-material" not in message

    rehearsal_image_mismatch = clone(valid)
    with tempfile.TemporaryDirectory() as temporary:
        evidence_root = Path(temporary) / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        migration = rehearsal_image_mismatch["migration"]
        assert isinstance(migration, dict)
        record = migration["rehearsal_evidence"]
        assert isinstance(record, dict)
        reference = record["reference"]
        assert isinstance(reference, str)
        payload = json.loads((evidence_root / reference).read_bytes())
        payload["image_digest"] = "registry.example/smart-llmrouter@sha256:" + "b" * 64
        rebind_evidence_payload(record, evidence_root, payload)
        assert "must bind the promoted artifact" in failure(rehearsal_image_mismatch, evidence_root.resolve())
    same_rollback = clone(valid)
    same_rollback["rollback"]["known_good_image_digest"] = valid["artifact"]["image_digest"]
    assert "must differ" in failure(same_rollback)
    rollback_alias = clone(valid)
    rollback_alias["rollback"]["known_good_image_digest"] = "mirror.example/router@sha256:" + "a" * 64
    assert "must differ" in failure(rollback_alias)

    with tempfile.TemporaryDirectory() as temporary:
        path = Path(temporary) / "manifest.json"
        path.write_text(json.dumps(valid))
        result = subprocess.run(
            ["python3", str(SCRIPT), "--manifest", str(path), "--evidence-root", str(EVIDENCE), "--now", "2029-01-01T00:00:00Z"],
            text=True,
            capture_output=True,
        )
    assert result.returncode != 0
    assert "unrecognized arguments: --now" in result.stderr
    with tempfile.TemporaryDirectory() as temporary:
        secret_shaped_path = Path(temporary) / "token=redacted-test-material.json"
        result = subprocess.run(
            ["python3", str(SCRIPT), "--manifest", str(secret_shaped_path), "--evidence-root", str(EVIDENCE)],
            text=True,
            capture_output=True,
        )
    assert result.returncode == 2
    assert "unable to access protected input" in result.stderr
    assert str(secret_shaped_path) not in result.stderr
    assert "redacted-test-material" not in result.stderr

    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        secret_shaped_evidence_root = temporary_root / "evidence-token=redacted-test-material"
        secret_shaped_evidence_root.mkdir()
        (secret_shaped_evidence_root / "evidence-promotion-plan.safe.json").symlink_to(
            "evidence-promotion-plan.safe.json"
        )
        manifest_path = temporary_root / "manifest.json"
        loop_manifest = clone(valid)
        loop_now = dt.datetime.now(dt.timezone.utc)
        loop_manifest["approval"]["issued_at"] = timestamp(loop_now - dt.timedelta(minutes=1))
        loop_manifest["approval"]["expires_at"] = timestamp(loop_now + dt.timedelta(minutes=29))
        manifest_path.write_text(json.dumps(loop_manifest))
        result = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "--manifest",
                str(manifest_path),
                "--evidence-root",
                str(secret_shaped_evidence_root),
            ],
            text=True,
            capture_output=True,
        )
    output = result.stdout + result.stderr
    assert result.returncode == 2
    assert "Traceback" not in output
    assert "unable to access protected input" in output
    assert str(secret_shaped_evidence_root) not in output
    assert "redacted-test-material" not in output

    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        live_manifest = clone(valid)
        now = dt.datetime.now(dt.timezone.utc)
        evidence_root = temporary_root / "evidence"
        shutil.copytree(EVIDENCE, evidence_root)
        refresh_evidence_timestamps(live_manifest, evidence_root, now)
        live_manifest["approval"]["issued_at"] = timestamp(now - dt.timedelta(minutes=1))
        live_manifest["approval"]["expires_at"] = timestamp(now + dt.timedelta(minutes=29))
        live_manifest_path = temporary_root / "manifest.json"
        live_manifest_path.write_text(json.dumps(live_manifest))
        result = promotion_make(str(live_manifest_path), str(evidence_root))
    assert result.returncode == 0, result.stderr
    assert '"outcome": "review_required_no_production_apply"' in result.stdout
    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        marker = temporary_root / "shell-injection-marker"
        malicious_manifest = f'"; touch {marker}; #'
        result = promotion_make(malicious_manifest, str(EVIDENCE))
        output = result.stdout + result.stderr
        assert result.returncode == 2
        assert not marker.exists(), "promotion path was interpreted by a shell"
        assert malicious_manifest not in output
        assert "unable to access protected input" in output
    with tempfile.TemporaryDirectory() as temporary:
        temporary_root = Path(temporary)
        marker = temporary_root / "make-expansion-marker"
        make_expression_manifest = f"$(shell touch {marker})"
        result = promotion_make(make_expression_manifest, str(EVIDENCE))
        output = result.stdout + result.stderr
        assert result.returncode == 2
        assert not marker.exists(), "promotion path was expanded by Make"
        assert make_expression_manifest not in output
        assert "unable to access protected input" in output
    print("production promotion manifest tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
