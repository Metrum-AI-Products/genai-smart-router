#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression contract for the deterministic capability smoke harness."""
from __future__ import annotations
import os
import importlib.util
import ast
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HARNESS = ROOT / "scripts/provider_capability_smoke.py"

def run(*args: str, **env: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(HARNESS), *args], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env={**os.environ, **env})

def require(value: bool, message: str) -> None:
    if not value: raise AssertionError(message)

def main() -> int:
    unit = run("unit")
    require(unit.returncode == 0 and "scrubbed synthetic" in unit.stdout, unit.stderr)
    live = run("live", LIVE_CAPABILITY_SMOKES="1", CAPABILITY_PROFILE="active")
    require(live.returncode == 2 and "intentionally unavailable" in live.stderr, "live mode must fail closed")
    source = HARNESS.read_text()
    imports = {alias.name.split(".")[0] for node in ast.walk(ast.parse(source)) if isinstance(node, (ast.Import, ast.ImportFrom)) for alias in node.names}
    require(not imports & {"urllib", "requests", "http", "socket"}, "network import found in fake adapter")
    spec = importlib.util.spec_from_file_location("capability_smoke", HARNESS)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    identity = {
        "provider": "synthetic",
        "account_identity_class": "test",
        "endpoint_fingerprint": "sha256:" + "a" * 64,
        "endpoint_path": "/v1/chat/completions",
        "api_skin": "openai-chat",
        "model": "synthetic",
        "model_suffix": "",
        "inbound_dialect": "openai-chat",
        "bridge_direction": "none",
        "request_shape": "text",
        "profile_version": "synthetic/v1",
    }
    safe = {"schema_version": module.SCHEMA_VERSION, "identity": identity, "capability_case": "text", "status": "passed", "observed": {"http_status": 200}}
    module.validate_result(safe)
    unsafe = dict(safe); unsafe["observed"] = {"nested": {"raw_prompt": "sentinel prompt", "image_url": "https://sentinel.invalid", "tool_schema": {"secret": "sentinel"}}}
    try:
        module.validate_result(unsafe)
    except ValueError:
        pass
    else:
        raise AssertionError("raw prompt sentinel survived result serialization")
    raw_output = dict(safe); raw_output["observed"] = {"finish_class": "SENTINEL RAW MODEL OUTPUT"}
    try:
        module.validate_result(raw_output)
    except ValueError:
        pass
    else:
        raise AssertionError("raw output sentinel survived scalar allowlist")
    wrong_shape = dict(safe)
    wrong_shape["identity"] = {**identity, "request_shape": "tools-auto"}
    try:
        module.validate_result(wrong_shape)
    except ValueError:
        pass
    else:
        raise AssertionError("request shape was not bound to capability case")
    for direction, expected in module.BRIDGE_SURFACES.items():
        if expected is None:
            inbound, api_skin = "openai-chat", "openai-responses"
        else:
            inbound, api_skin = expected[1], expected[0]
        mismatched_bridge = dict(safe)
        mismatched_bridge["identity"] = {
            **identity,
            "inbound_dialect": inbound,
            "api_skin": api_skin,
            "bridge_direction": direction,
        }
        try:
            module.validate_result(mismatched_bridge)
        except ValueError:
            pass
        else:
            raise AssertionError(f"mismatched {direction} bridge tuple was accepted")
    for mutated, label in (
        ({**safe, "notes": "benign-looking raw payload"}, "unknown result field"),
        ({**safe, "identity": {**identity, "deployment": "private"}}, "unknown identity field"),
    ):
        try:
            module.validate_result(mutated)
        except ValueError:
            pass
        else:
            raise AssertionError(f"{label} survived closed evidence schema")
    try:
        module.verify({"schema_version": module.SCHEMA_VERSION, "claims": {}}, [safe])
    except ValueError:
        pass
    else:
        raise AssertionError("non-list claims survived closed claims schema")
    valid_claims = {"schema_version": module.SCHEMA_VERSION, "claims": [{"identity": identity, "capability_case": "text"}]}
    try:
        module.verify(valid_claims, [safe, safe])
    except ValueError:
        pass
    else:
        raise AssertionError("duplicate evidence rows were accepted")
    profile_identity = {key: value for key, value in identity.items() if key != "profile_version"}
    duplicate_manifest = {
        "schema_version": module.SCHEMA_VERSION,
        "profiles": [{
            "profile_version": identity["profile_version"],
            "identity": profile_identity,
            "cases": [
                {"capability_case": "text", "status": "passed"},
                {"capability_case": "text", "status": "failed"},
            ],
        }],
    }
    try:
        module.validate_manifest(duplicate_manifest)
    except ValueError:
        pass
    else:
        raise AssertionError("duplicate manifest evidence rows were accepted")
    request = module.fake_request("tools-forced", safe["identity"])
    require(request["tool_choice"] == "forced", "fake adapter failed forced-tool classification")
    omitted = module.fake_request("tools-omitted", safe["identity"])
    require("tool_choice" not in omitted, "fake adapter represented omitted tool choice as an explicit value")
    make = (ROOT / "Makefile").read_text()
    require("capability-smoke-unit" in make and "SKIP_TESTS" in make, "Make capability contract missing")

    normal_make = subprocess.run(["make", "-n", "build-go-only"], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    require(normal_make.returncode == 0 and "provider_capability_smoke.py unit" in normal_make.stdout, "normal build must invoke the unit gate")
    skipped_make = subprocess.run(["make", "capability-smoke-unit", "SKIP_TESTS=true"], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    require(skipped_make.returncode == 0 and "WARNING: SKIP_TESTS=true" in skipped_make.stdout, "explicit skip must be visible")
    invalid_skip = subprocess.run(["make", "capability-smoke-unit", "SKIP_TESTS=1"], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    require(invalid_skip.returncode != 0 and "must be true or false" in invalid_skip.stderr, "invalid skip must fail")
    for target in ("package-one-no-docs", "docker-image-no-docs", "package-docker-one-no-docs"):
        dry_run = subprocess.run(["make", "-n", target], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        require(dry_run.returncode == 0 and "provider_capability_smoke.py unit" in dry_run.stdout, f"{target} bypasses capability gate")
    print("Provider capability smoke regression tests passed")
    return 0

if __name__ == "__main__": raise SystemExit(main())
