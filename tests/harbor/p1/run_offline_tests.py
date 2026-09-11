#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline self-test for Harbor P1/P2 stubs/verifiers (issue #94 / #104)."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))

from verifiers import (  # noqa: E402
    load_json,
    missing_credentials_disposition,
    task_dir,
    verify_harbor_07,
    verify_harbor_08,
    verify_harbor_09,
    verify_harbor_10,
    verify_harbor_11,
    verify_harbor_12,
    verify_harbor_13,
    verify_harbor_14,
    verify_harbor_15,
    verify_harbor_16,
    verify_harbor_17,
    verify_harbor_18,
)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    catalog = (ROOT / "catalog.yaml").read_text(encoding="utf-8")
    for case_id in [f"HARBOR-{i:02d}" for i in range(7, 19)]:
        require(case_id in catalog, f"catalog missing {case_id}")
    for case_id in ("HARBOR-17", "HARBOR-18"):
        require((task_dir(case_id) / "instruction.md").is_file(), f"{case_id} instruction.md missing")
        require(not (task_dir(case_id) / "BLOCKED.md").is_file(), f"{case_id} BLOCKED.md must be removed")
        idx = catalog.index(f"id: {case_id}")
        # Next case or EOF bounds the snippet so we only inspect this row.
        next_ids = [catalog.find(f"id: HARBOR-{i:02d}", idx + 1) for i in range(7, 19)]
        next_ids = [n for n in next_ids if n > idx]
        end = min(next_ids) if next_ids else len(catalog)
        snippet = catalog[idx:end]
        require("disposition: offline_stub" in snippet, f"{case_id} must be offline_stub")
        require("deferred_issue" not in snippet, f"{case_id} deferred_issue must be cleared")
        require("disposition: blocked" not in snippet, f"{case_id} must not remain blocked")

    # HARBOR-07
    verify_harbor_07(load_json(task_dir("HARBOR-07") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_07(load_json(task_dir("HARBOR-07") / "fixtures" / "starter.json"), expect_pass=False)
    verify_harbor_07(load_json(task_dir("HARBOR-07") / "fixtures" / "decoy_text_only.json"), expect_pass=False)

    # HARBOR-08
    verify_harbor_08(load_json(task_dir("HARBOR-08") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_08(load_json(task_dir("HARBOR-08") / "fixtures" / "starter.json"), expect_pass=False)
    verify_harbor_08(load_json(task_dir("HARBOR-08") / "fixtures" / "image_only.json"), expect_pass=False)
    verify_harbor_08(load_json(task_dir("HARBOR-08") / "fixtures" / "text_only.json"), expect_pass=False)

    # HARBOR-09
    verify_harbor_09(load_json(task_dir("HARBOR-09") / "fixtures" / "reference_transcript.json"), expect_pass=True)
    verify_harbor_09(load_json(task_dir("HARBOR-09") / "fixtures" / "starter_transcript.json"), expect_pass=False)

    # HARBOR-10
    verify_harbor_10(load_json(task_dir("HARBOR-10") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_10(load_json(task_dir("HARBOR-10") / "fixtures" / "limit_error.json"), expect_pass=True)
    verify_harbor_10(load_json(task_dir("HARBOR-10") / "fixtures" / "starter.json"), expect_pass=False)

    # HARBOR-11
    verify_harbor_11(load_json(task_dir("HARBOR-11") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_11(load_json(task_dir("HARBOR-11") / "fixtures" / "starter.json"), expect_pass=False)
    verify_harbor_11(load_json(task_dir("HARBOR-11") / "fixtures" / "duplicated.json"), expect_pass=False)

    # HARBOR-12
    verify_harbor_12(load_json(task_dir("HARBOR-12") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_12(load_json(task_dir("HARBOR-12") / "fixtures" / "starter.json"), expect_pass=False)

    # HARBOR-13
    verify_harbor_13(load_json(task_dir("HARBOR-13") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_13(load_json(task_dir("HARBOR-13") / "fixtures" / "starter.json"), expect_pass=False)

    # HARBOR-14
    verify_harbor_14(load_json(task_dir("HARBOR-14") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_14(load_json(task_dir("HARBOR-14") / "fixtures" / "starter.json"), expect_pass=False)

    # HARBOR-15
    verify_harbor_15(load_json(task_dir("HARBOR-15") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_15(load_json(task_dir("HARBOR-15") / "fixtures" / "starter.json"), expect_pass=False)
    verify_harbor_15(load_json(task_dir("HARBOR-15") / "fixtures" / "refusal.json"), expect_pass=False)

    # HARBOR-16
    verify_harbor_16(load_json(task_dir("HARBOR-16") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_16(load_json(task_dir("HARBOR-16") / "fixtures" / "starter.json"), expect_pass=False)

    # HARBOR-17
    verify_harbor_17(load_json(task_dir("HARBOR-17") / "fixtures" / "reference.json"), expect_pass=True)
    verify_harbor_17(load_json(task_dir("HARBOR-17") / "fixtures" / "starter.json"), expect_pass=False)
    verify_harbor_17(load_json(task_dir("HARBOR-17") / "fixtures" / "wrong_result.json"), expect_pass=False)

    # HARBOR-18
    verify_harbor_18(load_json(task_dir("HARBOR-18") / "fixtures" / "supported_ack.json"), expect_pass=True)
    verify_harbor_18(load_json(task_dir("HARBOR-18") / "fixtures" / "unsupported_clean_reject.json"), expect_pass=True)
    verify_harbor_18(load_json(task_dir("HARBOR-18") / "fixtures" / "silent_drop.json"), expect_pass=False)
    verify_harbor_18(load_json(task_dir("HARBOR-18") / "fixtures" / "false_green.json"), expect_pass=False)

    require(missing_credentials_disposition(False) == "blocked", "missing credentials must be blocked")
    require(missing_credentials_disposition(True) == "ready", "present credentials should be ready")

    print("harbor p1 offline verifier self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
