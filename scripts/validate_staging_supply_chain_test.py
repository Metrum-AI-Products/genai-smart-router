#!/usr/bin/env python3
"""Regression tests for the offline EKS supply-chain evidence contract."""
from __future__ import annotations

import hashlib
import json
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "validate_staging_supply_chain.py"
DIGEST = "registry.example/smart-llmrouter@sha256:" + "a" * 64
REPOSITORY, SHA256 = DIGEST.split("@sha256:", 1)
TARGET_POLICY = json.loads(
    (ROOT / "deploy/aws/genai-smart-router-eks-staging-target.json").read_text(encoding="utf-8")
)
ARCHITECTURE = str(TARGET_POLICY["image_architecture"])
MISMATCHED_ARCHITECTURE = "linux/arm64" if ARCHITECTURE == "linux/amd64" else "linux/amd64"
BINDING_FILENAME = "release-binding.intoto.json"
RELEASE_BINDING_PREDICATE_TYPE = "https://metrum.ai/attestations/eks-release-binding/v1"


def spdx_sbom() -> dict[str, object]:
    return {
        "spdxVersion": "SPDX-2.3",
        "dataLicense": "CC0-1.0",
        "SPDXID": "SPDXRef-DOCUMENT",
        "name": "smart-llmrouter",
        "documentNamespace": "https://example.invalid/spdx/smart-llmrouter",
        "creationInfo": {"created": "2026-07-20T00:00:00Z", "creators": ["Tool: supply-chain-test"]},
        "packages": [{"SPDXID": "SPDXRef-Package-router", "name": "smart-llmrouter"}],
    }


def cyclonedx_sbom() -> dict[str, object]:
    return {
        "bomFormat": "CycloneDX",
        "specVersion": "1.6",
        "version": 1,
        "components": [{"type": "container", "name": "smart-llmrouter"}],
        "metadata": {"component": {"type": "container", "name": "smart-llmrouter"}},
    }


def slsa_provenance() -> dict[str, object]:
    return {
        "_type": "https://in-toto.io/Statement/v1",
        "subject": [{"name": REPOSITORY, "digest": {"sha256": SHA256}}],
        "predicateType": "https://slsa.dev/provenance/v1",
        "predicate": {
            "buildDefinition": {
                "buildType": "https://example.invalid/build/smart-llmrouter",
                "externalParameters": {},
                "internalParameters": {},
                "resolvedDependencies": [],
            },
            "runDetails": {
                "builder": {"id": "https://example.invalid/builders/supply-chain-test"},
                "metadata": {},
            },
        },
    }


def write_json(path: Path, payload: object) -> bytes:
    raw = json.dumps(payload, separators=(",", ":"), sort_keys=True).encode("utf-8")
    path.write_bytes(raw)
    return raw


def sha256(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def release_binding(artifact_bytes: dict[str, bytes]) -> dict[str, object]:
    return {
        "_type": "https://in-toto.io/Statement/v1",
        "subject": [{"name": REPOSITORY, "digest": {"sha256": SHA256}}],
        "predicateType": RELEASE_BINDING_PREDICATE_TYPE,
        "predicate": {
            "architecture": ARCHITECTURE,
            "artifacts": {
                filename: {"sha256": sha256(raw)} for filename, raw in artifact_bytes.items()
            },
        },
    }


def write_evidence(
    directory: Path,
    *,
    scan_verdict: str = "pass",
    verified: bool = True,
    sbom=None,
    provenance=None,
) -> None:
    directory.mkdir(exist_ok=True)
    artifact_bytes = {
        "sbom.json": write_json(directory / "sbom.json", spdx_sbom() if sbom is None else sbom),
        "provenance.json": write_json(
            directory / "provenance.json", slsa_provenance() if provenance is None else provenance
        ),
        "scan.json": write_json(directory / "scan.json", {"verdict": scan_verdict}),
    }
    binding_bytes = write_json(directory / BINDING_FILENAME, release_binding(artifact_bytes))
    write_json(
        directory / "signature-verification.json",
        {"verified": verified, "binding_sha256": sha256(binding_bytes)},
    )


def run(directory: Path, *extra_args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["python3", str(SCRIPT), "--image-digest", DIGEST, "--evidence-dir", str(directory), *extra_args],
        text=True,
        capture_output=True,
    )


def main() -> int:
    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary) / "evidence"
        write_evidence(directory)
        assert run(directory).returncode == 0
        caller_architecture = run(directory, "--architecture", MISMATCHED_ARCHITECTURE)
        assert caller_architecture.returncode != 0 and "unrecognized arguments" in caller_architecture.stderr
        write_evidence(directory, sbom=cyclonedx_sbom())
        assert run(directory).returncode == 0
        nonstandard_spdx = spdx_sbom()
        nonstandard_spdx["image"] = DIGEST
        write_evidence(directory, sbom=nonstandard_spdx)
        assert "sbom.json must not contain custom top-level image" in run(directory).stderr
        nonstandard_provenance = slsa_provenance()
        nonstandard_provenance["architecture"] = ARCHITECTURE
        write_evidence(directory, provenance=nonstandard_provenance)
        assert "provenance.json must not contain custom top-level architecture" in run(directory).stderr
        spdx_files_inventory = spdx_sbom()
        del spdx_files_inventory["packages"]
        spdx_files_inventory["files"] = [{"SPDXID": "SPDXRef-File-router", "fileName": "/app/bin/router"}]
        write_evidence(directory, sbom=spdx_files_inventory)
        assert run(directory).returncode == 0
        empty_spdx_inventory = spdx_sbom()
        empty_spdx_inventory["packages"] = []
        write_evidence(directory, sbom=empty_spdx_inventory)
        assert "non-empty packages or files inventory" in run(directory).stderr
        unrecognized_spdx_inventory = spdx_sbom()
        unrecognized_spdx_inventory["packages"] = [{}]
        write_evidence(directory, sbom=unrecognized_spdx_inventory)
        assert "entries must contain non-empty SPDXID, name" in run(directory).stderr
        empty_cyclonedx_inventory = cyclonedx_sbom()
        empty_cyclonedx_inventory["components"] = []
        write_evidence(directory, sbom=empty_cyclonedx_inventory)
        assert "non-empty components inventory" in run(directory).stderr
        unrecognized_cyclonedx_inventory = cyclonedx_sbom()
        unrecognized_cyclonedx_inventory["components"] = [{}]
        write_evidence(directory, sbom=unrecognized_cyclonedx_inventory)
        assert "entries must contain non-empty type, name" in run(directory).stderr
        invalid_spdx = spdx_sbom()
        invalid_spdx["spdxVersion"] = None
        write_evidence(directory, sbom=invalid_spdx)
        assert "supported SPDX version" in run(directory).stderr
        invalid_cyclonedx = cyclonedx_sbom()
        invalid_cyclonedx["specVersion"] = "not-a-version"
        write_evidence(directory, sbom=invalid_cyclonedx)
        assert "supported CycloneDX specVersion" in run(directory).stderr
        invalid_statement_type = slsa_provenance()
        invalid_statement_type["_type"] = "anything"
        write_evidence(directory, provenance=invalid_statement_type)
        assert "in-toto Statement v1" in run(directory).stderr
        invalid_predicate_type = slsa_provenance()
        invalid_predicate_type["predicateType"] = "https://slsa.dev/provenance/v0.2"
        write_evidence(directory, provenance=invalid_predicate_type)
        assert "SLSA provenance v1" in run(directory).stderr
        invalid_predicate_body = slsa_provenance()
        invalid_predicate_body["predicate"]["buildDefinition"].pop("buildType")
        write_evidence(directory, provenance=invalid_predicate_body)
        assert "buildDefinition" in run(directory).stderr
        subject_without_digest = slsa_provenance()
        subject_without_digest["subject"] = [{"name": REPOSITORY}]
        write_evidence(directory, provenance=subject_without_digest)
        assert "repository name and sha256 digest" in run(directory).stderr
        subject_with_wrong_digest = slsa_provenance()
        subject_with_wrong_digest["subject"] = [{"name": REPOSITORY, "digest": {"sha256": "b" * 64}}]
        write_evidence(directory, provenance=subject_with_wrong_digest)
        assert "repository name and sha256 digest" in run(directory).stderr
        legacy_full_digest_name = slsa_provenance()
        legacy_full_digest_name["subject"] = [{"name": DIGEST, "digest": {"sha256": SHA256}}]
        write_evidence(directory, provenance=legacy_full_digest_name)
        assert "repository name and sha256 digest" in run(directory).stderr
        write_evidence(directory, scan_verdict="fail")
        assert "verdict: pass" in run(directory).stderr
        write_evidence(directory, verified=False)
        assert "verified: true" in run(directory).stderr

        write_evidence(directory)
        (directory / BINDING_FILENAME).unlink()
        assert "missing required release binding statement" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        binding["subject"][0]["digest"]["sha256"] = "b" * 64
        write_json(directory / BINDING_FILENAME, binding)
        assert "repository name and sha256 digest" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        binding["predicate"]["architecture"] = MISMATCHED_ARCHITECTURE
        write_json(directory / BINDING_FILENAME, binding)
        assert "predicate must bind architecture exactly" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        del binding["predicate"]["artifacts"]["scan.json"]
        write_json(directory / BINDING_FILENAME, binding)
        assert "must bind exactly the required artifact files" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        binding["predicate"]["artifacts"]["unexpected.json"] = {"sha256": "a" * 64}
        write_json(directory / BINDING_FILENAME, binding)
        assert "must bind exactly the required artifact files" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        binding["predicate"]["artifacts"]["sbom.json"] = {"sha256": "A" * 64}
        write_json(directory / BINDING_FILENAME, binding)
        assert "must contain a lower-case sha256" in run(directory).stderr

        for filename in ("sbom.json", "provenance.json", "scan.json"):
            write_evidence(directory)
            path = directory / filename
            path.write_bytes(path.read_bytes() + b"\n")
            assert f"does not bind the exact {filename} bytes" in run(directory).stderr

        write_evidence(directory)
        binding_path = directory / BINDING_FILENAME
        binding_path.write_bytes(binding_path.read_bytes() + b"\n")
        assert "binding_sha256 exactly" in run(directory).stderr

        write_evidence(directory)
        signature = json.loads((directory / "signature-verification.json").read_text())
        signature["binding_sha256"] = "b" * 64
        write_json(directory / "signature-verification.json", signature)
        assert "binding_sha256 exactly" in run(directory).stderr

        write_evidence(directory)
        (directory / "scan.json").write_text(json.dumps({"verdict": "pass", "token": "bad"}))
        assert "secret-like" in run(directory).stderr

        write_evidence(directory)
        (directory / "scan.json").write_text(json.dumps({"verdict": "pass", "note": "postgres://user:password@db/router"}))
        assert "secret-like" in run(directory).stderr

        write_evidence(directory)
        (directory / "scan.json").write_text(
            r'{"verdict":"pass","metadata":{"note":"postgres:\/\/user:password@db/router"}}'
        )
        assert "decoded secret-like" in run(directory).stderr

        write_evidence(directory)
        (directory / "scan.json").write_text(
            r'{"verdict":"pass","metadata":{"to\u006ben":"synthetic"}}'
        )
        assert "decoded secret-like" in run(directory).stderr

        write_evidence(directory)
        binding = json.loads((directory / BINDING_FILENAME).read_text())
        binding["predicate"]["token"] = "synthetic"
        write_json(directory / BINDING_FILENAME, binding)
        assert "secret-like" in run(directory).stderr
    print("EKS staging supply-chain evidence tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
