#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Run secret-free, repository-controlled launch readiness checks.

This command deliberately records only safe scalar command outcomes.  It does
not accept credentials and cannot turn local checks into deployment evidence.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CHECKS = (
    ("OPS-01", "config-and-package-contract", [sys.executable, "scripts/validate_release_matrix.py"]),
    ("OPS-03", "coding-agent-fixture-contract", [sys.executable, "scripts/coding_agent_matrix.py", "--mode", "mock", "--output-dir", "tmp/launch-coding-agent-matrix"]),
    ("OPS-04", "fallback-timeout-rate-limit-redaction", ["go", "test", "./internal/router", "-run", "Test(FallbackOrdering429RetryAfterTelemetryAndSecretRedaction|FallbackDoesNotCrossToolEligibility|CallerRPMErrorIsSafeAndSkipsUpstream|UpstreamAttemptTimeoutReturnsGatewayTimeoutAndDiagnostics)$", "-count=1"]),
    ("OPS-05", "quota-usage-report-contract", ["go", "test", "./internal/router", "-run", "Test(DailyQuotaAdmissionReservesRequestedMaxTokens|MonthlyQuotaAdmissionReservesRequestedMaxOutputTokens|UsageReportRendersThroughputAndCacheSnapshots|UsageDBSchemaIsRelationalOnly)$", "-count=1"]),
    ("OPS-08", "amd-manifest-contract", ["make", "test-k8s-amd-instinct-local-serving"]),
)


def git(*args: str) -> str:
    result = subprocess.run(["git", *args], cwd=ROOT, text=True, capture_output=True, check=True)
    return result.stdout.strip()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("tmp/launch-operational-readiness.json"))
    parser.add_argument("--list", action="store_true", help="print check IDs without running them")
    args = parser.parse_args()
    if args.list:
        for gate, name, _ in CHECKS:
            print(f"{gate}\t{name}")
        return 0

    revision = git("rev-parse", "HEAD")
    dirty = bool(git("status", "--porcelain=v1", "--untracked-files=no"))
    results = []
    ok = True
    for gate, name, command in CHECKS:
        started = time.monotonic()
        completed = subprocess.run(command, cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
        passed = completed.returncode == 0
        ok = ok and passed
        results.append({
            "gate": gate,
            "check": name,
            "status": "passed" if passed else "failed",
            "exit_code": completed.returncode,
            "duration_ms": int((time.monotonic() - started) * 1000),
        })

    payload = {
        "schema": "smart-router.launch-operational-readiness/v1",
        "evidence_scope": "local-repository-contracts-only",
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z"),
        "git_revision": revision,
        "tracked_tree_dirty": dirty,
        "results": results,
        "human_gates_not_proven": [
            "exact promoted artifact identity and deployment environment",
            "credential-backed runtime and client smokes",
            "production rollback rehearsal and named rollback owner",
            "monitoring ownership, alert thresholds, incident channel, and public support approval",
            "AMD Instinct hardware, driver, ROCm, and provider-backed inference compatibility",
        ],
    }
    canonical = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    payload["evidence_sha256"] = hashlib.sha256(canonical).hexdigest()
    output = args.output if args.output.is_absolute() else ROOT / args.output
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"launch readiness checks {'passed' if ok else 'failed'}; safe evidence: {output}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
