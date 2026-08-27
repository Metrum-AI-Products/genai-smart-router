#!/usr/bin/env python3
"""Validate a safe, evidence-gated production promotion manifest.

This offline checker only accepts review metadata. It cannot apply manifests,
reach cloud services, run database migrations, change DNS, or use credentials.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import sys
from pathlib import Path

from validate_staging_supply_chain import approved_target_architecture


DIGEST = re.compile(r"^[a-z0-9][a-z0-9./:_-]*@sha256:[0-9a-f]{64}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
SAFE_REF = re.compile(r"^[A-Za-z0-9._:/#-]{3,200}$")
IDENTIFIER = re.compile(r"^[A-Za-z0-9._-]{3,80}$")
RFC3339_TIMESTAMP = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-][0-9]{2}:[0-9]{2})$")
MANIFEST_CREDENTIAL_VALUE = re.compile(
    r"""(?ix)
    (?:
      \b(?:sk-[A-Za-z0-9_-]{20,}|sk_(?:live|test|org)_[A-Za-z0-9_]{20,}|xai-[A-Za-z0-9_-]{20,}|rtr_metrum_[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|AIza[0-9A-Za-z_-]{20,})\b
    )
    """
)
TOP_LEVEL = {
    "schema_version",
    "release_id",
    "environment",
    "artifact",
    "staging_evidence",
    "supply_chain",
    "configuration",
    "migration",
    "canary",
    "rollback",
    "promotion_gates",
}
SUPPLY_CHAIN_RESULT_FIELDS = {"outcome", "timestamp", "image_digest", "supply_chain"}
STAGING_PROMOTION_EVIDENCE_REFERENCE = "evidence-promotion-plan.safe.json"
STAGING_PROMOTION_EVIDENCE_FIELDS = {
    "schema_version",
    "outcome",
    "timestamp",
    "environment",
    "action",
    "image_digest",
    "configuration_fingerprint",
    "promotion_plan_result",
}
REHEARSAL_EVIDENCE_FIELDS = {
    "schema_version",
    "outcome",
    "timestamp",
    "image_digest",
    "migration_id",
    "configuration_fingerprint",
}
SUPPLY_CHAIN_SUMMARY_FIELDS = {
    "image_digest",
    "architecture",
    "release_binding_sha256",
    "sbom_sha256",
    "provenance_sha256",
    "scan_sha256",
    "signature_verified",
    "scan_verdict",
}
MAX_EVIDENCE_AGE = dt.timedelta(hours=24)


def fail(message: str) -> None:
    raise ValueError(message)


class DuplicateJSONMemberError(ValueError):
    """Raised when JSON bytes contain an ambiguous duplicate member."""


class ProtectedInputError(ValueError):
    """Raised when an operator-supplied protected path cannot be resolved safely."""


def reject_duplicate_json_members(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            # Do not include the untrusted key in the error: it can itself be
            # credential-shaped content and this gate must never echo it.
            raise DuplicateJSONMemberError("duplicate JSON member")
        result[key] = value
    return result


def parse_json(raw: str | bytes, name: str) -> object:
    """Parse protected JSON without silently discarding duplicate input bytes."""
    try:
        return json.loads(raw, object_pairs_hook=reject_duplicate_json_members)
    except DuplicateJSONMemberError:
        fail(f"{name} contains duplicate JSON members")
    except (json.JSONDecodeError, UnicodeDecodeError):
        fail(f"{name} is not valid JSON")


def object_at(value: object, name: str, fields: set[str]) -> dict[str, object]:
    if not isinstance(value, dict) or set(value) != fields:
        fail(f"{name} must contain exactly: {', '.join(sorted(fields))}")
    return value


def string_at(value: object, name: str, pattern: re.Pattern[str]) -> str:
    if not isinstance(value, str) or not pattern.fullmatch(value):
        fail(f"{name} has an invalid safe format")
    return value


def parse_time(value: object, name: str) -> dt.datetime:
    if not isinstance(value, str) or not RFC3339_TIMESTAMP.fullmatch(value):
        fail(f"{name} must be an RFC3339 timestamp with a UTC offset")
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        fail(f"{name} must be an RFC3339 timestamp with a UTC offset")
    if parsed.tzinfo is None or parsed.utcoffset() is None:
        fail(f"{name} must be an RFC3339 timestamp with a UTC offset")
    return parsed.astimezone(dt.timezone.utc)


def fresh_evidence_timestamp(value: object, name: str, now: dt.datetime) -> dt.datetime:
    """Require passed evidence to be recent relative to the validator's UTC clock."""
    observed = parse_time(value, name)
    if observed > now:
        fail(f"{name} must not be in the future")
    if now - observed > MAX_EVIDENCE_AGE:
        fail(f"{name} must be no more than 24 hours old")
    return observed


def schema_version_at(value: object, name: str) -> None:
    if type(value) is not int or value != 1:
        fail(f"{name} must be integer 1")


def manifest_scalar(value: str, name: str) -> str:
    """Reject bare credential-shaped fixed manifest scalars before logging them."""
    if MANIFEST_CREDENTIAL_VALUE.search(value):
        fail(f"{name} contains a credential-shaped value")
    return value


def protected_file_bytes(reference: str, expected_hash: str, name: str, evidence_root: Path) -> bytes:
    try:
        evidence_root = evidence_root.resolve()
        candidate = (evidence_root / reference).resolve()
        if evidence_root not in candidate.parents or not candidate.is_file():
            raise ProtectedInputError("protected evidence reference is unavailable")
        raw = candidate.read_bytes()
    except (OSError, RuntimeError) as exc:
        # A symlink loop or unreadable operator-controlled evidence path must
        # not escape as a traceback containing the supplied path.
        raise ProtectedInputError("protected evidence reference is unavailable") from exc
    if hashlib.sha256(raw).hexdigest() != expected_hash:
        fail(f"{name}.sha256 does not match resolved evidence")
    return raw


def referenced_evidence(value: object, name: str, evidence_root: Path) -> dict[str, object]:
    item = object_at(value, name, {"result", "reference", "sha256"})
    if item["result"] != "passed":
        fail(f"{name}.result must be passed")
    reference = manifest_scalar(
        string_at(item["reference"], f"{name}.reference", SAFE_REF),
        f"{name}.reference",
    )
    expected_hash = string_at(item["sha256"], f"{name}.sha256", HASH)
    payload = parse_json(protected_file_bytes(reference, expected_hash, name, evidence_root), f"{name}.reference")
    if not isinstance(payload, dict):
        fail(f"{name}.reference is not a safe evidence object")
    return payload


def staging_promotion_evidence(
    value: object, evidence_root: Path, now: dt.datetime
) -> dict[str, object]:
    name = "staging_evidence"
    item = object_at(value, name, {"result", "reference", "sha256"})
    reference = manifest_scalar(
        string_at(item["reference"], f"{name}.reference", SAFE_REF),
        f"{name}.reference",
    )
    if reference != STAGING_PROMOTION_EVIDENCE_REFERENCE:
        fail(f"{name}.reference must name the fixed safe promotion-plan evidence")
    payload = object_at(
        referenced_evidence(value, name, evidence_root),
        f"{name}.reference",
        STAGING_PROMOTION_EVIDENCE_FIELDS,
    )
    schema_version_at(payload["schema_version"], f"{name}.reference.schema_version")
    if (
        payload["outcome"] != "passed"
        or payload["environment"] != "staging"
        or payload["action"] != "promotion-plan"
        or payload["promotion_plan_result"] != "review_required_no_production_apply"
    ):
        fail(f"{name}.reference is not a passed safe EKS promotion-plan result")
    string_at(payload["image_digest"], f"{name}.reference.image_digest", DIGEST)
    string_at(payload["configuration_fingerprint"], f"{name}.reference.configuration_fingerprint", HASH)
    fresh_evidence_timestamp(payload["timestamp"], f"{name}.reference.timestamp", now)
    return payload


def rehearsal_evidence(value: object, evidence_root: Path, now: dt.datetime) -> dict[str, object]:
    name = "migration.rehearsal_evidence"
    payload = object_at(
        referenced_evidence(value, name, evidence_root),
        f"{name}.reference",
        REHEARSAL_EVIDENCE_FIELDS,
    )
    schema_version_at(payload["schema_version"], f"{name}.reference.schema_version")
    if payload["outcome"] != "passed":
        fail(f"{name}.reference does not record outcome passed")
    string_at(payload["image_digest"], f"{name}.reference.image_digest", DIGEST)
    string_at(payload["migration_id"], f"{name}.reference.migration_id", re.compile(r"^[A-Za-z0-9._-]{1,80}$"))
    string_at(payload["configuration_fingerprint"], f"{name}.reference.configuration_fingerprint", HASH)
    fresh_evidence_timestamp(payload["timestamp"], f"{name}.reference.timestamp", now)
    return payload


def content_digest(reference: str) -> str:
    """Return the immutable sha256 portion after DIGEST validation."""
    return reference.rsplit("@", 1)[1]


def validate(manifest: object, now: dt.datetime, evidence_root: Path) -> dict[str, object]:
    if now.tzinfo is None or now.utcoffset() is None:
        fail("validator clock must be timezone-aware")
    now = now.astimezone(dt.timezone.utc)
    if not isinstance(manifest, dict) or set(manifest) != TOP_LEVEL:
        fail("manifest must contain exactly the approved release-manifest fields")
    if (
        type(manifest["schema_version"]) is not int
        or manifest["schema_version"] != 1
        or manifest["environment"] != "production"
    ):
        fail("only schema_version 1 and environment production are accepted")
    release_id = manifest_scalar(
        string_at(manifest["release_id"], "release_id", re.compile(r"^[a-z0-9][a-z0-9-]{2,62}$")),
        "release_id",
    )
    artifact = object_at(manifest["artifact"], "artifact", {"image_digest"})
    digest = manifest_scalar(
        string_at(artifact["image_digest"], "artifact.image_digest", DIGEST),
        "artifact.image_digest",
    )
    config = object_at(manifest["configuration"], "configuration", {"version", "fingerprint"})
    manifest_scalar(
        string_at(config["version"], "configuration.version", re.compile(r"^[A-Za-z0-9._-]{1,80}$")),
        "configuration.version",
    )
    fingerprint = string_at(config["fingerprint"], "configuration.fingerprint", HASH)
    gates = object_at(manifest["promotion_gates"], "promotion_gates", {"change_reference", "result", "evaluated_at", "valid_until"})
    manifest_scalar(
        string_at(gates["change_reference"], "promotion_gates.change_reference", SAFE_REF),
        "promotion_gates.change_reference",
    )
    if gates["result"] != "passed":
        fail("promotion_gates.result must be passed")
    evaluated_at = parse_time(gates["evaluated_at"], "promotion_gates.evaluated_at")
    valid_until = parse_time(gates["valid_until"], "promotion_gates.valid_until")
    if evaluated_at > now:
        fail("promotion_gates have not been evaluated yet")
    if valid_until <= now:
        fail("promotion_gates validity window has expired")
    if valid_until <= evaluated_at or valid_until - evaluated_at > dt.timedelta(hours=1):
        fail("promotion_gates validity window must be greater than zero and no more than one hour")
    staging = staging_promotion_evidence(manifest["staging_evidence"], evidence_root, now)
    if staging["image_digest"] != digest or staging["configuration_fingerprint"] != fingerprint:
        fail("staging_evidence must bind the promoted artifact and configuration fingerprint")
    # The staging supply-chain contract owns raw SBOM, provenance, scan,
    # release-binding, and signature-verification validation.  Production only
    # accepts its one protected, hash-pinned safe result: dereferencing or
    # reinterpreting the individual artifacts here would duplicate that
    # contract and could drift from its policy-selected architecture.
    supply = object_at(manifest["supply_chain"], "supply_chain", {"validation_result"})
    supply_result = referenced_evidence(
        supply["validation_result"], "supply_chain.validation_result", evidence_root
    )
    supply_result = object_at(
        supply_result,
        "supply_chain.validation_result payload",
        SUPPLY_CHAIN_RESULT_FIELDS,
    )
    if supply_result["outcome"] != "passed":
        fail("supply_chain.validation_result does not record outcome passed")
    fresh_evidence_timestamp(
        supply_result["timestamp"], "supply_chain.validation_result.timestamp", now
    )
    if supply_result["image_digest"] != digest:
        fail("supply_chain.validation_result must bind the promoted artifact")
    supply_summary = object_at(
        supply_result.get("supply_chain"),
        "supply_chain.validation_result payload",
        SUPPLY_CHAIN_SUMMARY_FIELDS,
    )
    if supply_summary["image_digest"] != digest:
        fail("supply_chain.validation_result summary must bind the promoted artifact")
    for field in (
        "release_binding_sha256",
        "sbom_sha256",
        "provenance_sha256",
        "scan_sha256",
    ):
        string_at(
            supply_summary[field],
            f"supply_chain.validation_result.{field}",
            HASH,
        )
    if supply_summary["signature_verified"] is not True or supply_summary["scan_verdict"] != "pass":
        fail("supply-chain signature and scan must both pass")
    try:
        architecture = approved_target_architecture()
    except ValueError:
        fail("approved staging target policy is invalid")
    if supply_summary["architecture"] != architecture:
        fail("supply_chain.validation_result must bind the policy-derived deployment architecture")
    migration = object_at(manifest["migration"], "migration", {"id", "compatibility", "rehearsal_evidence"})
    migration_id = manifest_scalar(
        string_at(migration["id"], "migration.id", re.compile(r"^[A-Za-z0-9._-]{1,80}$")),
        "migration.id",
    )
    if migration["compatibility"] not in {"none", "backward-compatible", "expand-contract"}:
        fail("migration.compatibility is not approved")
    rehearsal = rehearsal_evidence(migration["rehearsal_evidence"], evidence_root, now)
    if (
        rehearsal["image_digest"] != digest
        or rehearsal["migration_id"] != migration_id
        or rehearsal["configuration_fingerprint"] != fingerprint
    ):
        fail("migration.rehearsal_evidence must bind the promoted artifact, declared migration, and configuration fingerprint")
    canary = object_at(manifest["canary"], "canary", {"scope_id", "max_traffic_percent", "observation_minutes"})
    manifest_scalar(string_at(canary["scope_id"], "canary.scope_id", IDENTIFIER), "canary.scope_id")
    if type(canary["max_traffic_percent"]) is not int or not 1 <= canary["max_traffic_percent"] <= 10:
        fail("canary.max_traffic_percent must be 1 through 10")
    if type(canary["observation_minutes"]) is not int or not 15 <= canary["observation_minutes"] <= 240:
        fail("canary.observation_minutes must be 15 through 240")
    rollback = object_at(manifest["rollback"], "rollback", {"class", "known_good_image_digest"})
    if rollback["class"] not in {"traffic-only", "compatible-schema"}:
        fail("rollback.class is not approved")
    rollback_digest = manifest_scalar(
        string_at(rollback["known_good_image_digest"], "rollback.known_good_image_digest", DIGEST),
        "rollback.known_good_image_digest",
    )
    if content_digest(rollback_digest) == content_digest(digest):
        fail("rollback.known_good_image_digest must differ by sha256 content digest from artifact.image_digest")
    return {"outcome": "review_required_no_production_apply", "release_id": release_id, "image_digest": digest, "canary_percent": canary["max_traffic_percent"]}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest")
    parser.add_argument("--evidence-root")
    parser.add_argument(
        "--from-make-environment",
        action="store_true",
        help="read the two production-promotion inputs from the fixed environment passed through by Make",
    )
    args = parser.parse_args()
    if args.from_make_environment:
        if args.manifest is not None or args.evidence_root is not None:
            parser.error("--from-make-environment cannot be combined with direct path inputs")
        manifest_input = os.environ.get("PRODUCTION_RELEASE_MANIFEST", "")
        evidence_root_input = os.environ.get("PRODUCTION_EVIDENCE_ROOT", "")
        if not manifest_input or not evidence_root_input:
            print("production promotion validation failed: required promotion input is missing", file=sys.stderr)
            return 2
    else:
        if args.manifest is None or args.evidence_root is None:
            parser.error("--manifest and --evidence-root are required unless --from-make-environment is used")
        manifest_input = args.manifest
        evidence_root_input = args.evidence_root
    try:
        manifest = parse_json(Path(manifest_input).read_bytes(), "manifest")
        # The command-line production gate always reads the trusted local UTC
        # clock. Tests inject a fixed clock through validate(), not this CLI.
        now = dt.datetime.now(dt.timezone.utc)
        evidence_root = Path(evidence_root_input).resolve()
        if not evidence_root.is_dir():
            fail("--evidence-root must be an existing protected directory")
        print(json.dumps(validate(manifest, now, evidence_root), sort_keys=True))
        return 0
    except (OSError, RuntimeError, ProtectedInputError):
        # Paths are operator-controlled input.  In particular, do not let a
        # missing path echo a credential-shaped directory or file name into a
        # CI/release log.
        print("production promotion validation failed: unable to access protected input", file=sys.stderr)
        return 2
    except (json.JSONDecodeError, ValueError) as exc:
        print(f"production promotion validation failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
