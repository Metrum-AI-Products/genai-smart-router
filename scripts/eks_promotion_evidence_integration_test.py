#!/usr/bin/env python3
"""Prove #557's safe result can satisfy the production review gate offline.

The test runs the EKS promotion-plan producer with checked-in fake cloud tools,
then runs the real staging release-binding validator over a standard evidence
bundle. Only the validator's safe stdout JSON is persisted in the production
evidence root. No cloud account, kubeconfig, credentials, image registry, or
production operation is used.
"""
from __future__ import annotations

import datetime as dt
import hashlib
import importlib.util
import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
EKS_CONTRACT_SCRIPT = ROOT / "scripts" / "eks_delivery_contract_test.py"
STAGING_SUPPLY_CHAIN_SCRIPT = ROOT / "scripts" / "validate_staging_supply_chain.py"
VALIDATOR_SCRIPT = ROOT / "scripts" / "validate_production_promotion.py"
TARGET_POLICY = json.loads(
    (ROOT / "deploy" / "aws" / "genai-smart-router-eks-staging-target.json").read_text(
        encoding="utf-8"
    )
)
SHA256 = re.compile(r"^[0-9a-f]{64}$")
RELEASE_BINDING_PREDICATE_TYPE = "https://metrum.ai/attestations/eks-release-binding/v1"
BINDING_FILENAME = "release-binding.intoto.json"


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


EKS_CONTRACT = load_module("eks_delivery_contract_for_promotion", EKS_CONTRACT_SCRIPT)
VALIDATOR = load_module("production_promotion_validator_for_integration", VALIDATOR_SCRIPT)


def encoded_json(payload: dict[str, object]) -> bytes:
    return (json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")


def write_evidence(root: Path, reference: str, payload: dict[str, object]) -> dict[str, str]:
    encoded = encoded_json(payload)
    (root / reference).write_bytes(encoded)
    return {
        "result": "passed",
        "reference": reference,
        "sha256": hashlib.sha256(encoded).hexdigest(),
    }


def write_safe_result(root: Path, reference: str, stdout: str) -> dict[str, str]:
    """Persist the exact safe result emitted by the #557 validator."""
    encoded = stdout.encode("utf-8")
    (root / reference).write_bytes(encoded)
    return {
        "result": "passed",
        "reference": reference,
        "sha256": hashlib.sha256(encoded).hexdigest(),
    }


def write_json(path: Path, payload: dict[str, object]) -> bytes:
    encoded = encoded_json(payload)
    path.write_bytes(encoded)
    return encoded


def timestamp(value: dt.datetime) -> str:
    return value.astimezone(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def spdx_sbom() -> dict[str, object]:
    return {
        "spdxVersion": "SPDX-2.3",
        "dataLicense": "CC0-1.0",
        "SPDXID": "SPDXRef-DOCUMENT",
        "name": "smart-llmrouter-e2e",
        "documentNamespace": "https://example.invalid/spdx/smart-llmrouter-e2e",
        "creationInfo": {
            "created": "2026-07-20T00:00:00Z",
            "creators": ["Tool: promotion-integration-test"],
        },
        "packages": [{"SPDXID": "SPDXRef-Package-router", "name": "smart-llmrouter"}],
    }


def slsa_provenance(image_digest: str) -> dict[str, object]:
    repository, separator, sha256 = image_digest.partition("@sha256:")
    assert separator and repository and SHA256.fullmatch(sha256)
    return {
        "_type": "https://in-toto.io/Statement/v1",
        "subject": [{"name": repository, "digest": {"sha256": sha256}}],
        "predicateType": "https://slsa.dev/provenance/v1",
        "predicate": {
            "buildDefinition": {
                "buildType": "https://example.invalid/build/smart-llmrouter-e2e",
                "externalParameters": {},
                "internalParameters": {},
                "resolvedDependencies": [],
            },
            "runDetails": {
                "builder": {"id": "https://example.invalid/builders/promotion-integration-test"},
                "metadata": {},
            },
        },
    }


def release_binding(image_digest: str, artifact_bytes: dict[str, bytes]) -> dict[str, object]:
    repository, separator, sha256 = image_digest.partition("@sha256:")
    assert separator and repository and SHA256.fullmatch(sha256)
    return {
        "_type": "https://in-toto.io/Statement/v1",
        "subject": [{"name": repository, "digest": {"sha256": sha256}}],
        "predicateType": RELEASE_BINDING_PREDICATE_TYPE,
        "predicate": {
            "architecture": TARGET_POLICY["image_architecture"],
            "artifacts": {
                filename: {"sha256": hashlib.sha256(raw).hexdigest()}
                for filename, raw in artifact_bytes.items()
            },
        },
    }


def safe_staging_supply_chain_result(root: Path, image_digest: str) -> tuple[dict[str, object], str]:
    """Build a standard #557 bundle and obtain the real validator stdout."""
    bundle = root / "staging-supply-chain-input"
    bundle.mkdir()
    artifacts = {
        "sbom.json": write_json(bundle / "sbom.json", spdx_sbom()),
        "provenance.json": write_json(bundle / "provenance.json", slsa_provenance(image_digest)),
        "scan.json": write_json(bundle / "scan.json", {"verdict": "pass"}),
    }
    binding_bytes = write_json(bundle / BINDING_FILENAME, release_binding(image_digest, artifacts))
    write_json(
        bundle / "signature-verification.json",
        {"verified": True, "binding_sha256": hashlib.sha256(binding_bytes).hexdigest()},
    )
    completed = subprocess.run(
        [
            "python3",
            str(STAGING_SUPPLY_CHAIN_SCRIPT),
            "--image-digest",
            image_digest,
            "--evidence-dir",
            str(bundle),
        ],
        cwd=ROOT,
        text=True,
        capture_output=True,
    )
    assert completed.returncode == 0, completed.stderr
    result = json.loads(completed.stdout)
    assert set(result) == {"outcome", "timestamp", "image_digest", "supply_chain"}
    assert result["outcome"] == "passed" and result["image_digest"] == image_digest
    summary = result["supply_chain"]
    assert isinstance(summary, dict)
    assert summary == {
        "image_digest": image_digest,
        "architecture": TARGET_POLICY["image_architecture"],
        "release_binding_sha256": hashlib.sha256(binding_bytes).hexdigest(),
        "sbom_sha256": hashlib.sha256(artifacts["sbom.json"]).hexdigest(),
        "provenance_sha256": hashlib.sha256(artifacts["provenance.json"]).hexdigest(),
        "scan_sha256": hashlib.sha256(artifacts["scan.json"]).hexdigest(),
        "signature_verified": True,
        "scan_verdict": "pass",
    }
    return result, completed.stdout


def generated_staging_promotion_evidence() -> tuple[dict[str, object], dict[str, object]]:
    """Run the raw staging plan and consume only its isolated safe projection."""
    with tempfile.TemporaryDirectory(prefix="smartrouter-promotion-producer-") as temporary:
        root = Path(temporary)
        fake_bin = root / "fake-bin"
        fake_bin.mkdir()
        EKS_CONTRACT.fake_tools(fake_bin)
        applied = EKS_CONTRACT.run("apply", root, ["--confirm", "STAGING_APPLY"])
        assert applied.returncode == 0, applied.stderr
        smoke_script = EKS_CONTRACT.protected_smoke_script(root, "passing-smoke.sh", "exit 0\n")
        smoked = EKS_CONTRACT.run("smoke", root, ["--smoke-command-file", str(smoke_script)])
        assert smoked.returncode == 0, smoked.stderr
        planned = EKS_CONTRACT.run("promotion-plan", root)
        assert planned.returncode == 0, planned.stderr
        raw_evidence_path = root / "tmp" / "evidence" / "evidence-promotion-plan.json"
        safe_evidence_path = (
            root
            / "tmp"
            / "promotion-evidence"
            / EKS_CONTRACT.EKS_DELIVERY.PROMOTION_PLAN_SAFE_EVIDENCE_FILE
        )
        raw_evidence = json.loads(raw_evidence_path.read_text(encoding="utf-8"))
        safe_evidence = json.loads(safe_evidence_path.read_text(encoding="utf-8"))
    assert raw_evidence.get("outcome") == "passed"
    assert raw_evidence.get("environment") == "staging"
    assert raw_evidence.get("action") == "promotion-plan"
    assert raw_evidence.get("image_digest") == EKS_CONTRACT.DIGEST
    assert SHA256.fullmatch(str(raw_evidence.get("configuration_fingerprint")))
    assert any(
        isinstance(event, dict)
        and event.get("name") == "promotion_plan"
        and event.get("result") == "review_required_no_production_apply"
        for event in raw_evidence.get("events", [])
    )
    assert set(safe_evidence) == EKS_CONTRACT.EKS_DELIVERY.PROMOTION_PLAN_SAFE_EVIDENCE_FIELDS
    assert safe_evidence == {
        "schema_version": 1,
        "outcome": "passed",
        "timestamp": raw_evidence["timestamp"],
        "environment": "staging",
        "action": "promotion-plan",
        "image_digest": EKS_CONTRACT.DIGEST,
        "configuration_fingerprint": raw_evidence["configuration_fingerprint"],
        "promotion_plan_result": "review_required_no_production_apply",
    }
    assert "events" not in safe_evidence and "aws_account_id" not in safe_evidence
    return raw_evidence, safe_evidence


def release_manifest(
    staging_reference: dict[str, str],
    supply_reference: dict[str, str],
    rehearsal_reference: dict[str, str],
    image_digest: str,
    configuration_fingerprint: str,
    now: dt.datetime,
) -> dict[str, object]:
    return {
        "schema_version": 1,
        "release_id": "router-e2e-001",
        "environment": "production",
        "artifact": {"image_digest": image_digest},
        "staging_evidence": staging_reference,
        "supply_chain": {"validation_result": supply_reference},
        "configuration": {"version": "e2e.1", "fingerprint": configuration_fingerprint},
        "migration": {
            "id": "usage-expand-e2e-v1",
            "compatibility": "backward-compatible",
            "rehearsal_evidence": rehearsal_reference,
        },
        "canary": {"scope_id": "e2e-canary-001", "max_traffic_percent": 5, "observation_minutes": 30},
        "rollback": {
            "class": "compatible-schema",
            "known_good_image_digest": "registry.example/router@sha256:" + "b" * 64,
        },
        "approval": {
            "change_reference": "change/e2e-001",
            "release_approved_by": "release-e2e",
            "operations_approved_by": "operations-e2e",
            "issued_at": timestamp(now - dt.timedelta(minutes=5)),
            "expires_at": timestamp(now + dt.timedelta(minutes=30)),
        },
    }


def validate_cli(manifest_path: Path, evidence_root: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["python3", str(VALIDATOR_SCRIPT), "--manifest", str(manifest_path), "--evidence-root", str(evidence_root)],
        cwd=ROOT,
        text=True,
        capture_output=True,
    )


def validation_failure(manifest: dict[str, object], now: dt.datetime, evidence_root: Path) -> str:
    try:
        VALIDATOR.validate(manifest, now, evidence_root)
    except ValueError as exc:
        return str(exc)
    raise AssertionError("expected generated staging-evidence binding validation to fail")


def main() -> int:
    raw_staging, staging = generated_staging_promotion_evidence()
    image_digest = staging["image_digest"]
    configuration_fingerprint = staging["configuration_fingerprint"]
    assert isinstance(image_digest, str)
    assert isinstance(configuration_fingerprint, str)
    with tempfile.TemporaryDirectory(prefix="smartrouter-promotion-validator-") as temporary:
        root = Path(temporary)
        evidence_root = root / "protected-evidence"
        evidence_root.mkdir()
        staging_reference = write_evidence(
            evidence_root,
            VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
            staging,
        )
        _, safe_stdout = safe_staging_supply_chain_result(root, image_digest)
        # The real validator emits its timestamp after it has inspected the
        # bundle. Sample the review clock afterwards so the test cannot race a
        # second boundary and incorrectly classify fresh evidence as future.
        now = dt.datetime.now(dt.timezone.utc)
        rehearsal = {
            "schema_version": 1,
            "outcome": "passed",
            "timestamp": timestamp(now),
            "image_digest": image_digest,
            "migration_id": "usage-expand-e2e-v1",
            "configuration_fingerprint": configuration_fingerprint,
        }
        supply_reference = write_safe_result(evidence_root, "supply-validation-result.json", safe_stdout)
        rehearsal_reference = write_evidence(evidence_root, "rehearsal-generated.json", rehearsal)
        assert {path.name for path in evidence_root.iterdir()} == {
            VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
            "supply-validation-result.json",
            "rehearsal-generated.json",
        }
        manifest = release_manifest(
            staging_reference,
            supply_reference,
            rehearsal_reference,
            image_digest,
            configuration_fingerprint,
            now,
        )
        assert set(manifest["supply_chain"]) == {"validation_result"}
        assert "deployment" not in manifest
        manifest_path = root / "release-manifest.json"
        manifest_path.write_bytes(encoded_json(manifest))
        direct = VALIDATOR.validate(manifest, now, evidence_root)
        assert direct["outcome"] == "review_required_no_production_apply"
        cli = validate_cli(manifest_path, evidence_root)
        assert cli.returncode == 0, cli.stderr
        assert json.loads(cli.stdout)["outcome"] == "review_required_no_production_apply"

        raw_reference = write_evidence(
            evidence_root, "evidence-promotion-plan.json", raw_staging
        )
        raw_candidate = json.loads(json.dumps(manifest))
        raw_candidate["staging_evidence"] = raw_reference
        assert "fixed safe promotion-plan evidence" in validation_failure(
            raw_candidate, now, evidence_root
        )

        full_candidate = json.loads(json.dumps(manifest))
        full_candidate["staging_evidence"] = write_evidence(
            evidence_root,
            VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
            raw_staging,
        )
        full_error = validation_failure(full_candidate, now, evidence_root)
        assert "must contain exactly" in full_error
        assert "events" not in full_error and "aws_account_id" not in full_error

        extra_staging = json.loads(json.dumps(staging))
        extra_staging["authorization"] = "opaque-redacted-test-value"
        extra_candidate = json.loads(json.dumps(manifest))
        extra_candidate["staging_evidence"] = write_evidence(
            evidence_root,
            VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
            extra_staging,
        )
        extra_error = validation_failure(extra_candidate, now, evidence_root)
        assert "must contain exactly" in extra_error
        assert "authorization" not in extra_error

        for field in ("image_digest", "configuration_fingerprint"):
            mutated_staging = json.loads(json.dumps(staging))
            del mutated_staging[field]
            candidate = json.loads(json.dumps(manifest))
            candidate["staging_evidence"] = write_evidence(
                evidence_root,
                VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
                mutated_staging,
            )
            error = validation_failure(candidate, now, evidence_root)
            assert "must contain exactly" in error
            manifest_path.write_bytes(encoded_json(candidate))
            cli = validate_cli(manifest_path, evidence_root)
            assert cli.returncode == 2
            assert "must contain exactly" in cli.stderr

        # Restore the original producer result after the negative staging
        # cases; the next assertion isolates the #557 result binding itself.
        manifest["staging_evidence"] = write_evidence(
            evidence_root,
            VALIDATOR.STAGING_PROMOTION_EVIDENCE_REFERENCE,
            staging,
        )
        safe_result = json.loads(safe_stdout)
        safe_result["supply_chain"]["image_digest"] = "registry.example/router@sha256:" + "c" * 64
        candidate = json.loads(json.dumps(manifest))
        candidate["supply_chain"]["validation_result"] = write_evidence(
            evidence_root, "supply-validation-result.json", safe_result
        )
        assert "summary must bind the promoted artifact" in validation_failure(candidate, now, evidence_root)

    print("generated EKS promotion evidence integration test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
