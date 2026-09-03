#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline, scrubbed provider-capability evidence contract.

This deliberately contains no HTTP client, credential lookup, or environment
loading.  It is the deterministic half of issue #575; protected live probing
is intentionally unavailable until a separately reviewed implementation exists.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_VERSION = "capability-smoke/v1"
STATUSES = {"passed", "limited", "failed", "unsupported", "untested"}
LATENCY_BUCKETS = {"under-1s", "1s-to-5s", "over-5s"}
FINISH_CLASSES = {"stop", "tool-call", "length", "unknown"}
ERROR_CLASSES = {"upstream_bad_request", "unsupported", "none"}
SENSITIVE_FIELD = re.compile(r"(?i)(authorization|api[_-]?key|token|secret|password|bearer|prompt|image|tool.*schema|request_body|response_body)")
SENSITIVE_VALUE = re.compile(r"(?i)(bearer\s+|https?://)")
REQUIRED_IDENTITY = (
    "provider", "account_identity_class", "endpoint_fingerprint", "endpoint_path",
    "api_skin", "model", "model_suffix", "inbound_dialect", "bridge_direction",
    "request_shape", "profile_version",
)
FINGERPRINT = re.compile(r"^sha256:[0-9a-f]{64}$")
DIALECTS = {"openai-chat", "openai-responses", "anthropic", "replicate"}
BRIDGE_SURFACES = {
    "none": None,
    "chat_to_responses": ("openai-chat", "openai-responses"),
    "responses_to_chat": ("openai-responses", "openai-chat"),
    "anthropic_to_openai-chat": ("anthropic", "openai-chat"),
    "anthropic_to_openai-responses": ("anthropic", "openai-responses"),
    "anthropic_to_replicate": ("anthropic", "replicate"),
    "openai-chat_to_anthropic": ("openai-chat", "anthropic"),
    "openai-chat_to_replicate": ("openai-chat", "replicate"),
    "openai-responses_to_anthropic": ("openai-responses", "anthropic"),
    "openai-responses_to_replicate": ("openai-responses", "replicate"),
}
BRIDGE_DIRECTIONS = set(BRIDGE_SURFACES)
CAPABILITY_REQUEST_SHAPES = {
    "text": "text",
    "openai-responses": "text",
    "tools-omitted": "tools-omitted",
    "tools-auto": "tools-auto",
    "tools-forced": "tools-forced",
    "image-input": "image",
    "router-selected-ocr": "image",
    "structured-outputs": "structured-outputs",
}


def fail(message: str) -> None:
    raise ValueError(message)


def read_json(path: Path) -> dict:
    try:
        value = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"{path}: invalid JSON: {exc}")
    if not isinstance(value, dict):
        fail(f"{path}: root must be an object")
    return value


def scrub(value: object) -> object:
    """Reject, rather than redact, unsafe durable evidence fields and values."""
    if isinstance(value, dict):
        out = {}
        for key, item in value.items():
            if SENSITIVE_FIELD.search(str(key)):
                fail(f"unsafe evidence field {key!r}")
            out[key] = scrub(item)
        return out
    if isinstance(value, list):
        return [scrub(item) for item in value]
    if isinstance(value, str) and SENSITIVE_VALUE.search(value):
        fail("unsafe evidence value")
    return value

def require_fields(value: dict, required: set[str], allowed: set[str], context: str) -> None:
    missing = required - set(value)
    if missing:
        fail(f"{context} is missing fields: {', '.join(sorted(missing))}")
    extra = set(value) - allowed
    if extra:
        fail(f"{context} has unsupported fields: {', '.join(sorted(extra))}")

def validate_identity(identity: object, capability_case: object) -> None:
    if not isinstance(capability_case, str) or not capability_case:
        fail("result capability_case is required")
    if not isinstance(identity, dict):
        fail("result identity must be an object")
    require_fields(identity, set(REQUIRED_IDENTITY), set(REQUIRED_IDENTITY), "result identity")
    for field in REQUIRED_IDENTITY:
        value = identity.get(field)
        if not isinstance(value, str) or (field != "model_suffix" and not value.strip()):
            fail(f"result identity.{field} is required")
        if value != value.strip():
            fail(f"result identity.{field} must not contain outer whitespace")
    if not FINGERPRINT.fullmatch(identity["endpoint_fingerprint"]):
        fail("result identity.endpoint_fingerprint must be sha256:<64 lowercase hex>")
    if not identity["endpoint_path"].startswith("/"):
        fail("result identity.endpoint_path must be absolute")
    if identity["api_skin"] not in DIALECTS or identity["inbound_dialect"] not in DIALECTS:
        fail("result identity dialect is unsupported")
    if identity["bridge_direction"] not in BRIDGE_DIRECTIONS:
        fail("result identity.bridge_direction is unsupported")
    expected_bridge = BRIDGE_SURFACES[identity["bridge_direction"]]
    if expected_bridge is None:
        if identity["inbound_dialect"] != identity["api_skin"]:
            fail("result direct identity must use the API skin as inbound dialect")
    elif expected_bridge != (identity["inbound_dialect"], identity["api_skin"]):
        fail("result bridge_direction does not match inbound_dialect and api_skin")
    if identity["model_suffix"] and not identity["model_suffix"].startswith(":"):
        fail("result identity.model_suffix must be empty or colon-prefixed")
    expected_shape = CAPABILITY_REQUEST_SHAPES.get(capability_case)
    if expected_shape is None:
        fail("result capability_case is unsupported")
    if identity["request_shape"] != expected_shape:
        fail("result identity.request_shape does not match capability_case")


def validate_result(result: dict) -> None:
    scrub(result)
    require_fields(
        result,
        {"schema_version", "identity", "capability_case", "status", "observed"},
        {"schema_version", "identity", "capability_case", "status", "observed"},
        "result",
    )
    if result["schema_version"] != SCHEMA_VERSION:
        fail("result schema_version is unsupported")
    identity = result["identity"]
    capability_case = result["capability_case"]
    validate_identity(identity, capability_case)
    if result["status"] not in STATUSES:
        fail("result status must be pass/limited/failed/unsupported/untested")
    observed = result["observed"]
    if not isinstance(observed, dict):
        fail("result observed must be an object")
    allowed = {"http_status", "request_id_present", "latency_bucket", "usage_present", "finish_class", "error_class", "upstream_attempts", "selected_target"}
    extra = set(observed) - allowed
    if extra:
        fail(f"result observed has unsupported fields: {', '.join(sorted(extra))}")
    # Values are deliberately narrow scalar summaries, never response text.
    if "http_status" in observed and (type(observed["http_status"]) is not int or not 100 <= observed["http_status"] <= 599): fail("observed.http_status must be an HTTP status")
    for field in ("request_id_present", "usage_present", "selected_target"):
        if field in observed and type(observed[field]) is not bool: fail(f"observed.{field} must be boolean")
    if "upstream_attempts" in observed and (type(observed["upstream_attempts"]) is not int or not 0 <= observed["upstream_attempts"] <= 9): fail("observed.upstream_attempts must be bounded integer")
    if "latency_bucket" in observed and observed["latency_bucket"] not in LATENCY_BUCKETS: fail("observed.latency_bucket is unsupported")
    if "finish_class" in observed and observed["finish_class"] not in FINISH_CLASSES: fail("observed.finish_class is unsupported")
    if "error_class" in observed and observed["error_class"] not in ERROR_CLASSES: fail("observed.error_class is unsupported")


def identity_key(identity: dict, capability_case: str) -> tuple:
    return tuple(identity[field] for field in REQUIRED_IDENTITY) + (capability_case,)


def validate_manifest(manifest: dict) -> None:
    scrub(manifest)
    require_fields(manifest, {"schema_version", "profiles"}, {"schema_version", "profiles"}, "manifest")
    if manifest["schema_version"] != SCHEMA_VERSION:
        fail("manifest schema_version is unsupported")
    profiles = manifest["profiles"]
    if not isinstance(profiles, list) or not profiles:
        fail("manifest profiles must be a non-empty list")
    seen = set()
    profile_identity_fields = set(REQUIRED_IDENTITY) - {"profile_version"}
    for profile in profiles:
        if not isinstance(profile, dict):
            fail("every profile must be an object")
        require_fields(profile, {"profile_version", "identity", "cases"}, {"profile_version", "identity", "cases"}, "profile")
        if not isinstance(profile["profile_version"], str) or not profile["profile_version"].strip():
            fail("every profile needs profile_version")
        identity = profile["identity"]
        if not isinstance(identity, dict):
            fail("profile identity must be an object")
        require_fields(identity, profile_identity_fields, profile_identity_fields, "profile identity")
        cases = profile["cases"]
        if not isinstance(cases, list) or not cases:
            fail("every profile needs cases")
        for case in cases:
            if not isinstance(case, dict):
                fail("profile case must be an object")
            require_fields(case, {"capability_case", "status"}, {"capability_case", "status", "observed"}, "profile case")
            if "observed" in case and not isinstance(case["observed"], dict):
                fail("profile case observed must be an object")
            if case["status"] not in STATUSES:
                fail("profile cases need a recognized status")
            full_identity = dict(identity)
            full_identity["profile_version"] = profile["profile_version"]
            validate_identity(full_identity, case["capability_case"])
            key = identity_key(full_identity, case["capability_case"])
            if key in seen:
                fail("manifest contains duplicate identity and capability_case")
            seen.add(key)

def adapter_results(manifest: dict) -> list[dict]:
    """A deterministic fake adapter: profile rows become scalar-only results."""
    results = []
    for profile in manifest["profiles"]:
        for case in profile["cases"]:
            identity = dict(profile["identity"])
            identity["profile_version"] = profile["profile_version"]
            request = fake_request(case["capability_case"], identity)
            response = fake_response(case, request)
            result = {"schema_version": SCHEMA_VERSION, "identity": identity,
                      "capability_case": case["capability_case"], "status": case["status"],
                      "observed": response}
            validate_result(result)
            results.append(result)
    return results

def fake_request(capability_case: str, identity: dict) -> dict:
    """Canned shape only; deliberately excludes prompts, schemas, and media."""
    request = {"api_skin": identity["api_skin"], "capability_case": capability_case,
               "router_selected": capability_case == "router-selected-ocr"}
    if capability_case == "tools-forced":
        request["tool_choice"] = "forced"
    elif capability_case == "tools-auto":
        request["tool_choice"] = "auto"
    return request

def fake_response(case: dict, request: dict) -> dict:
    observed = dict(case.get("observed", {}))
    # The adapter proves request classification: a forced tool cannot be
    # accidentally represented as an ordinary text request.
    if request.get("tool_choice") == "forced" and case["capability_case"] != "tools-forced": fail("fake adapter forced-tool classification mismatch")
    if case["capability_case"] == "tools-omitted" and "tool_choice" in request: fail("fake adapter omitted-tool request included tool_choice")
    if request["router_selected"] and case["capability_case"] != "router-selected-ocr": fail("fake adapter router-selection classification mismatch")
    return observed


def verify(claims: dict, results: list[dict]) -> list[str]:
    scrub(claims)
    require_fields(claims, {"schema_version", "claims"}, {"schema_version", "claims"}, "claims")
    if claims["schema_version"] != SCHEMA_VERSION:
        fail("claims schema_version is unsupported")
    if not isinstance(claims["claims"], list) or not claims["claims"]:
        fail("claims.claims must be a non-empty list")
    index = {}
    for row in results:
        validate_result(row)
        key = identity_key(row["identity"], row["capability_case"])
        if key in index:
            fail("results contain duplicate identity and capability_case")
        index[key] = row
    failures = []
    for claim in claims["claims"]:
        if not isinstance(claim, dict):
            failures.append("claim must be an object")
            continue
        try:
            require_fields(claim, {"identity", "capability_case"}, {"identity", "capability_case"}, "claim")
            identity = claim["identity"]
            capability = claim["capability_case"]
            validate_identity(identity, capability)
        except ValueError as exc:
            failures.append(str(exc))
            continue
        result = index.get(identity_key(identity, capability))
        if not result:
            failures.append(f"missing evidence for {capability}")
        elif result["status"] != "passed":
            failures.append(f"{capability} is {result['status']}, not passed")
    return failures


def unit() -> int:
    manifest = read_json(ROOT / "testdata/capability-smokes/v1/synthetic-profiles.json")
    validate_manifest(manifest)
    results = adapter_results(manifest)
    good = read_json(ROOT / "testdata/capability-smokes/v1/eligible-claims.json")
    bad = read_json(ROOT / "testdata/capability-smokes/v1/ineligible-claims.json")
    if verify(good, results):
        fail("eligible synthetic claims unexpectedly failed")
    if len(verify(bad, results)) != 4:
        fail("ineligible synthetic claims did not prove all four capability gates")
    print(f"capability smoke unit passed: {len(results)} scrubbed synthetic results")
    return 0


def live() -> int:
    print("capability-smoke-live is intentionally unavailable: mock-first suite never probes networks, credentials, or protected evidence.", file=sys.stderr)
    return 2


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("unit", "live", "verify"))
    parser.add_argument("--manifest", type=Path)
    parser.add_argument("--claims", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "unit": return unit()
        if args.command == "live": return live()
        if not args.manifest or not args.claims: fail("verify needs --manifest and --claims")
        manifest = read_json(args.manifest); validate_manifest(manifest)
        failures = verify(read_json(args.claims), adapter_results(manifest))
        if failures:
            print("capability evidence mismatch: " + "; ".join(failures), file=sys.stderr); return 1
        print("capability evidence verified"); return 0
    except ValueError as exc:
        print(f"capability smoke error: {exc}", file=sys.stderr); return 1

if __name__ == "__main__":
    raise SystemExit(main())
