#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Run and record the repeatable outcome-calibrated routing demonstration."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
EXAMPLES = ROOT / "examples" / "external-routing-policy"
POLICY = EXAMPLES / "outcome_calibrated_policy.py"
DATASET = EXAMPLES / "outcome_coding_dataset.json"
REVIEWS = EXAMPLES / "reviewed_coding_outcomes.jsonl"
EVIDENCE_PATTERN = re.compile(r"OUTCOME_DEMO (\{.*\})")


def run(command: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(command, cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out-dir", required=True, type=Path, help="ignored directory for profile, YAML patch, test log, and evidence")
    args = parser.parse_args()
    out_dir = args.out_dir.resolve()
    out_dir.mkdir(parents=True, exist_ok=True)
    profile = out_dir / "profile.json"
    yaml_patch = out_dir / "target-weights.yaml"
    calibrated = run([sys.executable, str(POLICY), "calibrate", "--dataset", str(DATASET), "--reviews", str(REVIEWS), "--out-profile", str(profile), "--out-yaml", str(yaml_patch)])
    if calibrated.returncode != 0:
        print(calibrated.stdout, file=sys.stderr)
        return calibrated.returncode
    routed = run(["go", "test", "./internal/router", "-run", "^TestOutcomeCalibratedPolicyReferenceEndToEnd$", "-v"])
    (out_dir / "router-test.log").write_text(routed.stdout, encoding="utf-8")
    if routed.returncode != 0:
        print(routed.stdout, file=sys.stderr)
        return routed.returncode
    decisions = [json.loads(match.group(1)) for match in EVIDENCE_PATTERN.finditer(routed.stdout)]
    if len(decisions) != 4 or any(row["selectedModel"] != row["expectedModel"] for row in decisions):
        print("demo did not record the expected routing evidence", file=sys.stderr)
        return 1
    evidence = {
        "demo": "outcome-calibrated-external-routing",
        "dataset": str(DATASET.relative_to(ROOT)),
        "reviewFixture": str(REVIEWS.relative_to(ROOT)),
        "profile": profile.name,
        "weightPatch": yaml_patch.name,
        "decisions": decisions,
    }
    (out_dir / "evidence.json").write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
    rows = ["# Outcome-Calibrated Routing Demo", "", "Calibration completed from synthetic human-reviewed coding outcomes. The router then sent each new request through the external policy service.", "", "| Class | Selected target | Request |", "| --- | --- | --- |"]
    rows.extend(f"| {row['class']} | `{row['selectedModel']}` | {row['prompt']} |" for row in decisions)
    rows.extend(["", "The profile and YAML patch are generated for review only; neither command changes router configuration."])
    (out_dir / "evidence.md").write_text("\n".join(rows) + "\n", encoding="utf-8")
    print(json.dumps({"evidence": str(out_dir / "evidence.json"), "report": str(out_dir / "evidence.md"), "decisions": decisions}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
