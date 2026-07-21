#!/usr/bin/env python3
"""Fail closed on the safe evidence required before an EKS staging release.

This local verifier deliberately does not build, push, sign, scan, or deploy.
Those operations belong to the protected release environment.  It ensures the
evidence handed to the Make EKS delivery contract is digest-pinned, complete,
and free of obvious secret material before any Kubernetes operation begins.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import re
import sys
from pathlib import Path

from eks_delivery import read_checked_in_target_policy


DIGEST = re.compile(r"^[a-z0-9][a-z0-9./:_-]*@sha256:[0-9a-f]{64}$")
SECRET = re.compile(r"(?i)[\"']?(authorization|bearer|token|password|secret|api[_-]?key|dsn|database[_-]?url|credentials?)[\"']?\s*[:=]")
SENSITIVE_VALUE = re.compile(r"(?i)(?:postgres(?:ql)?|mysql|mongodb(?:\+[a-z0-9]+)?)://|\bbearer\s+[a-z0-9._~+/=-]+|\bbasic\s+[a-z0-9+/=]+")
SUPPORTED_SPDX_VERSIONS = {"SPDX-2.2", "SPDX-2.3"}
SUPPORTED_CYCLONEDX_SPEC_VERSIONS = {"1.4", "1.5", "1.6"}
IN_TOTO_STATEMENT_TYPE = "https://in-toto.io/Statement/v1"
SLSA_PROVENANCE_PREDICATE_TYPE = "https://slsa.dev/provenance/v1"
RELEASE_BINDING_PREDICATE_TYPE = "https://metrum.ai/attestations/eks-release-binding/v1"
BINDING_FILENAME = "release-binding.intoto.json"
BOUND_ARTIFACT_FILENAMES = (
    "sbom.json",
    "provenance.json",
    "scan.json",
)
SHA256 = re.compile(r"^[0-9a-f]{64}$")
REQUIRED = {
    BINDING_FILENAME: "release binding statement",
    "sbom.json": "SBOM",
    "provenance.json": "provenance",
    "signature-verification.json": "signature verification",
    "scan.json": "vulnerability scan",
}


def fail(message: str) -> None:
    raise ValueError(message)


def decoded_contains_secret_like(value: object) -> bool:
    """Reject secret-like decoded JSON keys and string values recursively.

    The raw-text scan remains useful for catching malformed or plainly encoded
    evidence. JSON escaping can hide a URL or field name from that scan, so
    inspect the semantic values after decoding as well. This returns only a
    boolean to keep caller-facing failures free of the sensitive value.
    """
    if isinstance(value, dict):
        for key, child in value.items():
            if not isinstance(key, str):
                return True
            if SECRET.search(f"{key}:") or SENSITIVE_VALUE.search(key):
                return True
            if decoded_contains_secret_like(child):
                return True
        return False
    if isinstance(value, list):
        return any(decoded_contains_secret_like(item) for item in value)
    if isinstance(value, str):
        return bool(SECRET.search(value) or SENSITIVE_VALUE.search(value))
    return False


def load(directory: Path, filename: str) -> tuple[dict[str, object], bytes]:
    path = directory / filename
    if not path.is_file():
        fail(f"missing required {REQUIRED[filename]} evidence: {path}")
    raw_bytes = path.read_bytes()
    try:
        raw = raw_bytes.decode("utf-8")
    except UnicodeDecodeError:
        fail(f"{filename} must be UTF-8 JSON")
    if SECRET.search(raw) or SENSITIVE_VALUE.search(raw):
        fail(f"{filename} contains a secret-like field or value")
    try:
        result = json.loads(raw)
    except json.JSONDecodeError as exc:
        fail(f"{filename} is not valid JSON: {exc.msg}")
    if not isinstance(result, dict):
        fail(f"{filename} must contain a JSON object")
    if decoded_contains_secret_like(result):
        fail(f"{filename} contains a decoded secret-like field or value")
    return result, raw_bytes


def require_exact_string(payload: dict[str, object], field: str, expected: str, filename: str) -> None:
    if payload.get(field) != expected:
        fail(f"{filename} must bind {field} exactly to the requested value")


def require_nonempty_string(payload: dict[str, object], field: str, filename: str) -> str:
    value = payload.get(field)
    if not isinstance(value, str) or not value.strip():
        fail(f"{filename} must contain a non-empty {field}")
    return value


def require_object(payload: dict[str, object], field: str, filename: str) -> dict[str, object]:
    value = payload.get(field)
    if not isinstance(value, dict):
        fail(f"{filename} must contain an object at {field}")
    return value


def require_inventory(
    payload: dict[str, object], fields: dict[str, tuple[str, ...]], filename: str
) -> None:
    """Require at least one non-empty, recognized SBOM inventory list."""
    present = False
    nonempty = False
    for field, required_fields in fields.items():
        if field not in payload:
            continue
        present = True
        value = payload[field]
        if not isinstance(value, list):
            fail(f"{filename} {field} must be an array of objects")
        for item in value:
            if not isinstance(item, dict) or any(
                not isinstance(item.get(required), str) or not item[required].strip()
                for required in required_fields
            ):
                fail(
                    f"{filename} {field} entries must contain non-empty "
                    + ", ".join(required_fields)
                )
        nonempty = nonempty or bool(value)
    if not present or not nonempty:
        joined = " or ".join(fields)
        fail(f"{filename} must contain a non-empty {joined} inventory")


def validate_spdx_sbom(sbom: dict[str, object]) -> None:
    """Validate the required SPDX document marker and minimum document shape."""
    if sbom.get("spdxVersion") not in SUPPORTED_SPDX_VERSIONS:
        fail("sbom.json must use a supported SPDX version")
    if sbom.get("SPDXID") != "SPDXRef-DOCUMENT":
        fail("sbom.json SPDX documents must use SPDXID SPDXRef-DOCUMENT")
    if sbom.get("dataLicense") != "CC0-1.0":
        fail("sbom.json SPDX documents must use dataLicense CC0-1.0")
    require_nonempty_string(sbom, "name", "sbom.json")
    require_nonempty_string(sbom, "documentNamespace", "sbom.json")
    creation_info = require_object(sbom, "creationInfo", "sbom.json")
    require_nonempty_string(creation_info, "created", "sbom.json creationInfo")
    creators = creation_info.get("creators")
    if not isinstance(creators, list) or not creators or not all(isinstance(creator, str) and creator.strip() for creator in creators):
        fail("sbom.json creationInfo must contain non-empty creators")
    require_inventory(
        sbom,
        {"packages": ("SPDXID", "name"), "files": ("SPDXID", "fileName")},
        "sbom.json SPDX document",
    )


def validate_cyclonedx_sbom(sbom: dict[str, object]) -> None:
    """Validate the required CycloneDX marker and minimum BOM shape."""
    if sbom.get("bomFormat") != "CycloneDX":
        fail("sbom.json must use bomFormat CycloneDX")
    if sbom.get("specVersion") not in SUPPORTED_CYCLONEDX_SPEC_VERSIONS:
        fail("sbom.json must use a supported CycloneDX specVersion")
    version = sbom.get("version")
    if not isinstance(version, int) or isinstance(version, bool) or version < 1:
        fail("sbom.json CycloneDX documents must contain a positive integer version")
    require_inventory(sbom, {"components": ("type", "name")}, "sbom.json CycloneDX document")
    if "metadata" in sbom and not isinstance(sbom["metadata"], dict):
        fail("sbom.json CycloneDX metadata must be an object")


def validate_sbom_format(sbom: dict[str, object]) -> None:
    """Require exactly one recognized SBOM standard instead of marker presence."""
    has_spdx_marker = "spdxVersion" in sbom
    has_cyclonedx_marker = "bomFormat" in sbom
    if has_spdx_marker == has_cyclonedx_marker:
        fail("sbom.json must declare exactly one supported SPDX or CycloneDX format")
    if has_spdx_marker:
        validate_spdx_sbom(sbom)
        return
    validate_cyclonedx_sbom(sbom)


def reject_standard_document_deployment_fields(payload: dict[str, object], filename: str) -> None:
    """Keep deployment-specific bindings out of standards-governed documents."""
    for field in ("image", "architecture"):
        if field in payload:
            fail(f"{filename} must not contain custom top-level {field}; use {BINDING_FILENAME}")


def validate_provenance_format(provenance: dict[str, object]) -> None:
    """Require the recognized in-toto v1/SLSA v1 statement and predicate body."""
    if provenance.get("_type") != IN_TOTO_STATEMENT_TYPE:
        fail("provenance.json must use the in-toto Statement v1 type")
    if provenance.get("predicateType") != SLSA_PROVENANCE_PREDICATE_TYPE:
        fail("provenance.json must use the SLSA provenance v1 predicate type")
    predicate = require_object(provenance, "predicate", "provenance.json")
    build_definition = require_object(predicate, "buildDefinition", "provenance.json predicate")
    require_nonempty_string(build_definition, "buildType", "provenance.json predicate buildDefinition")
    require_object(build_definition, "externalParameters", "provenance.json predicate buildDefinition")
    require_object(build_definition, "internalParameters", "provenance.json predicate buildDefinition")
    dependencies = build_definition.get("resolvedDependencies")
    if not isinstance(dependencies, list):
        fail("provenance.json predicate buildDefinition must contain resolvedDependencies as an array")
    run_details = require_object(predicate, "runDetails", "provenance.json predicate")
    builder = require_object(run_details, "builder", "provenance.json predicate runDetails")
    require_nonempty_string(builder, "id", "provenance.json predicate runDetails builder")
    require_object(run_details, "metadata", "provenance.json predicate runDetails")


def provenance_subject_matches(payload: dict[str, object], image_digest: str) -> bool:
    """Bind an in-toto subject's repository name and sha256 value together.

    SLSA/in-toto subject names identify the artifact/repository while the
    immutable content hash lives in ``subject[].digest.sha256``.  Treating the
    complete ``image@sha256:...`` reference as a subject name is both
    non-standard and does not prove the subject digest matches the requested
    image.
    """
    repository, separator, expected_sha256 = image_digest.partition("@sha256:")
    if not separator:
        return False
    subjects = payload.get("subject")
    if not isinstance(subjects, list):
        return False
    return any(
        isinstance(subject, dict)
        and subject.get("name") == repository
        and isinstance(subject.get("digest"), dict)
        and subject["digest"].get("sha256") == expected_sha256
        for subject in subjects
    )


def sha256_hex(raw_bytes: bytes) -> str:
    """Return the lower-case SHA-256 for exact evidence bytes.

    The release binding is deliberately over raw bytes rather than decoded and
    re-serialized JSON.  A serializer can change whitespace, ordering, or
    escaping without changing parsed data; the protected signer and verifier
    must agree on the exact artifact that was inspected.
    """
    return hashlib.sha256(raw_bytes).hexdigest()


def validate_release_binding(
    binding: dict[str, object],
    artifact_bytes: dict[str, bytes],
    image_digest: str,
    architecture: str,
) -> None:
    """Validate the signed-release binding without modifying standard evidence.

    SPDX/CycloneDX SBOMs and SLSA provenance are passed through in their
    native forms.  The custom in-toto binding statement records the target
    architecture and hashes the exact standard documents plus scan result.
    Its raw hash is independently verified by signature-verification.json.
    """
    if binding.get("_type") != IN_TOTO_STATEMENT_TYPE:
        fail(f"{BINDING_FILENAME} must use the in-toto Statement v1 type")
    if binding.get("predicateType") != RELEASE_BINDING_PREDICATE_TYPE:
        fail(f"{BINDING_FILENAME} must use the approved release-binding predicate type")
    if not provenance_subject_matches(binding, image_digest):
        fail(
            f"{BINDING_FILENAME} must contain a subject with the IMAGE_DIGEST "
            "repository name and sha256 digest"
        )
    predicate = require_object(binding, "predicate", BINDING_FILENAME)
    require_exact_string(predicate, "architecture", architecture, f"{BINDING_FILENAME} predicate")
    artifacts = require_object(predicate, "artifacts", f"{BINDING_FILENAME} predicate")
    expected_artifacts = set(BOUND_ARTIFACT_FILENAMES)
    if set(artifacts) != expected_artifacts:
        fail(f"{BINDING_FILENAME} predicate must bind exactly the required artifact files")
    for filename in BOUND_ARTIFACT_FILENAMES:
        artifact = artifacts.get(filename)
        if not isinstance(artifact, dict) or set(artifact) != {"sha256"}:
            fail(f"{BINDING_FILENAME} predicate {filename} must contain only sha256")
        expected_sha256 = artifact.get("sha256")
        if not isinstance(expected_sha256, str) or not SHA256.fullmatch(expected_sha256):
            fail(f"{BINDING_FILENAME} predicate {filename} must contain a lower-case sha256")
        if expected_sha256 != sha256_hex(artifact_bytes[filename]):
            fail(f"{BINDING_FILENAME} does not bind the exact {filename} bytes")


def approved_target_architecture() -> str:
    """Read the architecture from the reviewed target, never caller input."""
    try:
        return read_checked_in_target_policy().image_architecture
    except RuntimeError as exc:
        raise ValueError("approved staging target policy is invalid") from exc


def validate(args: argparse.Namespace) -> dict[str, object]:
    """Validate raw protected evidence and return its safe, promotable result.

    The returned object intentionally contains only digest-pinned result
    fields. A protected release system may retain that JSON as the sole
    supply-chain reference consumed by the production promotion review gate;
    the raw SBOM, provenance, scan, release binding, and signature report stay
    inside the protected build/release boundary.
    """
    if not DIGEST.fullmatch(args.image_digest):
        fail("IMAGE_DIGEST must be a lower-case immutable image@sha256:<64 hex> reference")
    architecture = approved_target_architecture()
    directory = Path(args.evidence_dir)
    loaded = {filename: load(directory, filename) for filename in REQUIRED}
    evidence = {filename: payload for filename, (payload, _) in loaded.items()}
    raw_evidence = {filename: raw_bytes for filename, (_, raw_bytes) in loaded.items()}
    validate_release_binding(
        evidence[BINDING_FILENAME],
        {filename: raw_evidence[filename] for filename in BOUND_ARTIFACT_FILENAMES},
        args.image_digest,
        architecture,
    )
    if not provenance_subject_matches(evidence["provenance.json"], args.image_digest):
        fail("provenance.json must contain a subject with the IMAGE_DIGEST repository name and sha256 digest")
    reject_standard_document_deployment_fields(evidence["sbom.json"], "sbom.json")
    reject_standard_document_deployment_fields(evidence["provenance.json"], "provenance.json")
    validate_sbom_format(evidence["sbom.json"])
    validate_provenance_format(evidence["provenance.json"])
    signature = evidence["signature-verification.json"]
    if signature.get("verified") is not True:
        fail("signature-verification.json must record verified: true from independent verification")
    require_exact_string(
        signature,
        "binding_sha256",
        sha256_hex(raw_evidence[BINDING_FILENAME]),
        "signature-verification.json",
    )
    scan = evidence["scan.json"]
    if scan.get("verdict") != "pass":
        fail("scan.json must record verdict: pass under the approved severity/exception policy")
    timestamp = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    return {
        "outcome": "passed",
        "timestamp": timestamp,
        "image_digest": args.image_digest,
        "supply_chain": {
            "image_digest": args.image_digest,
            "architecture": architecture,
            "release_binding_sha256": sha256_hex(raw_evidence[BINDING_FILENAME]),
            "sbom_sha256": sha256_hex(raw_evidence["sbom.json"]),
            "provenance_sha256": sha256_hex(raw_evidence["provenance.json"]),
            "scan_sha256": sha256_hex(raw_evidence["scan.json"]),
            "signature_verified": True,
            "scan_verdict": "pass",
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image-digest", required=True)
    parser.add_argument("--evidence-dir", required=True)
    args = parser.parse_args()
    try:
        result = validate(args)
    except ValueError as exc:
        print(f"staging supply-chain validation failed: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
