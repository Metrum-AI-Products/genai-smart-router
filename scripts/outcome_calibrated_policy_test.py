#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Self-test the outcome-calibrated external policy reference."""

from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
EXAMPLE_DIR = ROOT / "examples" / "external-routing-policy"
SCRIPT = EXAMPLE_DIR / "outcome_calibrated_policy.py"
DATASET = EXAMPLE_DIR / "outcome_dataset.json"
REVIEWS = EXAMPLE_DIR / "reviewed_outcomes.jsonl"


def load_module():
    spec = importlib.util.spec_from_file_location("outcome_policy", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class FakeEmbeddings:
    vectors = {
        "What is 2+2?": [1.0, 0.0, 0.0],
        "Create a runnable load-testing benchmark for HTTP servers.": [0.0, 1.0, 0.0],
        "Show a folder listing.": [0.0, 0.0, 1.0],
        "What is 2+2? Reply with only the answer.": [1.0, 0.0, 0.0],
        "Create a runnable load-testing benchmark for HTTP servers.": [0.0, 1.0, 0.0],
        "Show a folder listing.": [0.0, 0.0, 1.0],
        "Write a poem about a moonlit river.": [-1.0, 0.0, 0.0],
    }

    def embed(self, texts):
        return [self.vectors[text] for text in texts]


def require(value: bool, message: str) -> None:
    if not value:
        raise AssertionError(message)


def request(text: str) -> dict:
    return {
        "targets": [
            {"provider": "mock", "model": "cheap-text", "weight": 50},
            {"provider": "mock", "model": "strong-code", "weight": 50},
        ],
        "request": {"messages": [{"role": "user", "content": text}]},
    }


def main() -> int:
    module = load_module()
    dataset = json.loads(DATASET.read_text(encoding="utf-8"))
    reviews = module.read_jsonl(REVIEWS)
    profile = module.reviewed_profile(dataset, reviews)
    require(profile["promotable"], f"sample profile is not promotable: {profile}")
    embeddings = FakeEmbeddings()
    vectors = module.class_vectors(profile, embeddings)

    math = module.policy_decision(profile, request("What is 2+2? Reply with only the answer."), vectors, embeddings)
    code = module.policy_decision(profile, request("Create a runnable load-testing benchmark for HTTP servers."), vectors, embeddings)
    listing = module.policy_decision(profile, request("Show a folder listing."), vectors, embeddings)
    unknown = module.policy_decision(profile, request("Write a poem about a moonlit river."), vectors, embeddings)
    require(math["targetIndex"] == 0, f"arithmetic did not select cheap candidate: {math}")
    require(code["targetIndex"] == 1, f"coding did not select strong candidate: {code}")
    require(listing["targetIndex"] == 0, f"listing did not select cheap candidate: {listing}")
    require(unknown["targetIndex"] == 1 and unknown["classLabel"] == "outcome-policy:unclassified", f"unknown did not select strong default: {unknown}")
    try:
        module.policy_decision(profile, {"targets": request("Show a folder listing.")["targets"]}, vectors, embeddings)
    except ValueError as error:
        require("include_request" in str(error), f"unexpected trusted-content failure: {error}")
    else:
        raise AssertionError("policy accepted a request without include_request mirror")

    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        profile_path = root / "profile.json"
        patch_path = root / "weights.yaml"
        calibrated = subprocess.run([sys.executable, str(SCRIPT), "calibrate", "--dataset", str(DATASET), "--reviews", str(REVIEWS), "--out-profile", str(profile_path), "--out-yaml", str(patch_path)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        require(calibrated.returncode == 0, f"calibration failed: {calibrated.stdout}\n{calibrated.stderr}")
        require("strategy: external" in patch_path.read_text(encoding="utf-8"), "missing external strategy patch")
        overrides = root / "overrides.json"
        planned = subprocess.run([sys.executable, str(SCRIPT), "plan", "--dataset", str(DATASET), "--out-overrides", str(overrides)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        require(planned.returncode == 0, f"calibration plan failed: {planned.stdout}\n{planned.stderr}")
        require(len(json.loads(overrides.read_text(encoding="utf-8"))["calibrationOverrides"]) == 6, "calibration plan did not cover every case/candidate")
        insufficient = root / "insufficient.jsonl"
        insufficient.write_text(REVIEWS.read_text(encoding="utf-8").splitlines()[0] + "\n", encoding="utf-8")
        failed = subprocess.run([sys.executable, str(SCRIPT), "calibrate", "--dataset", str(DATASET), "--reviews", str(insufficient), "--out-profile", str(root / "failed.json"), "--out-yaml", str(root / "failed.yaml")], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        require(failed.returncode == 2 and "insufficient-evidence" in failed.stdout, f"insufficient evidence was promoted: {failed.stdout}\n{failed.stderr}")

    print("outcome-calibrated policy self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
