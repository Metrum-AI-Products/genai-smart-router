#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Independent offline verifiers for Harbor P1/P2 stubs (issue #94 / #104)."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
TASKS = ROOT / "tasks"


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def task_dir(case_id: str) -> Path:
    return TASKS / case_id


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def verify_harbor_07(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    truth = load_json(task_dir("HARBOR-07") / "fixtures" / "ground_truth.json")
    merchant = str(artifact.get("merchant", ""))
    total = artifact.get("total_usd")
    currency = str(artifact.get("currency", ""))
    source = str(artifact.get("source_modality", ""))
    ok = (
        merchant == truth["merchant"]
        and float(total) == float(truth["total_usd"])
        and currency == truth["currency"]
        and source == "image"
    )
    require(ok == expect_pass, f"HARBOR-07 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_08(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    truth = load_json(task_dir("HARBOR-08") / "fixtures" / "ground_truth.json")
    ok = (
        str(artifact.get("image_fact")) == truth["image_fact"]
        and str(artifact.get("text_fact")) == truth["text_fact"]
        and str(artifact.get("combined")) == truth["combined"]
    )
    require(ok == expect_pass, f"HARBOR-08 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_09(transcript: dict[str, Any], *, expect_pass: bool) -> None:
    """Protocol checker: opaque reasoning must be preserved; content not scored."""
    turns = transcript.get("turns") or []
    preserved = False
    leaked_score_request = False
    for turn in turns:
        reasoning = turn.get("reasoning")
        if isinstance(reasoning, dict):
            opaque = reasoning.get("encrypted_content") or reasoning.get("opaque")
            if isinstance(opaque, str) and opaque.startswith("opaque:"):
                preserved = True
            if reasoning.get("summary") in (None, "") and opaque:
                preserved = True
        text = str(turn.get("assistant_text", ""))
        if re.search(r"score|reveal|decrypt.*(reasoning|thinking)", text, re.I):
            leaked_score_request = True
    tools_ok = bool(transcript.get("tool_loop_completed"))
    ok = preserved and tools_ok and not leaked_score_request
    require(ok == expect_pass, f"HARBOR-09 expected pass={expect_pass}, got ok={ok}")


def verify_harbor_10(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    late = str(artifact.get("late_instruction_echo", ""))
    disposition = str(artifact.get("disposition", "ok"))
    truncated = bool(artifact.get("history_truncated_silently", False))
    ok = (late == "EDGE-CASE-NONCE-77" and disposition == "ok") or disposition == "explicit_limit_error"
    ok = ok and not truncated
    require(ok == expect_pass, f"HARBOR-10 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_11(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    before = str(artifact.get("pre_restart_fact", ""))
    after = str(artifact.get("post_restart_fact", ""))
    duplicated = bool(artifact.get("duplicated_pre_restart_work", False))
    ok = before == "ALPHA-BEFORE" and after == "BETA-AFTER" and not duplicated
    require(ok == expect_pass, f"HARBOR-11 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_12(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    route = str(artifact.get("serving_route", ""))
    attempts = artifact.get("attempts") or []
    unauthorized = str(artifact.get("unauthorized_target", ""))
    ok = (
        route == "eligible-backup"
        and "primary-retryable" in attempts
        and "eligible-backup" in attempts
        and not unauthorized
        and bool(artifact.get("task_passed"))
    )
    require(ok == expect_pass, f"HARBOR-12 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_13(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    disposition = str(artifact.get("disposition", ""))
    forged = bool(artifact.get("forged_success", False))
    reservations_ok = bool(artifact.get("reservations_reconciled", False))
    ok = disposition in {"incomplete", "quota_exhausted", "blocked"} and not forged and reservations_ok
    require(ok == expect_pass, f"HARBOR-13 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_14(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    disposition = str(artifact.get("disposition", ""))
    counted_complete = bool(artifact.get("counted_as_completed", False))
    late = bool(artifact.get("late_output_after_cancel", False))
    ok = disposition in {"cancelled", "timeout", "incomplete"} and not counted_complete and not late
    require(ok == expect_pass, f"HARBOR-14 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_15(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    schema = load_json(task_dir("HARBOR-15") / "fixtures" / "schema.json")
    required = set(schema.get("required") or [])
    enums = schema.get("enums") or {}
    ok = True
    if artifact.get("status") in {"refusal", "truncated"}:
        ok = False
    for key in required:
        if key not in artifact:
            ok = False
    if "tags" in artifact and not isinstance(artifact["tags"], list):
        ok = False
    if "note" in artifact and artifact["note"] is not None and not isinstance(artifact["note"], str):
        ok = False
    for field, allowed in enums.items():
        if field in artifact and artifact[field] not in allowed:
            ok = False
    if "label" in artifact and "\u2603" not in str(artifact["label"]) and expect_pass:
        # reference must keep the Unicode snowman from the fixture
        if str(artifact.get("label")) != "ok-\u2603":
            ok = False
    if expect_pass:
        ok = ok and artifact.get("status") == "complete" and artifact.get("label") == "ok-\u2603"
        ok = ok and artifact.get("note") is None and artifact.get("tags") == ["a", "b"]
        ok = ok and artifact.get("kind") == "alpha"
    require(ok == expect_pass, f"HARBOR-15 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_16(artifacts: dict[str, dict[str, Any]], *, expect_pass: bool) -> None:
    a = artifacts.get("caller_a") or {}
    b = artifacts.get("caller_b") or {}
    ok = (
        str(a.get("nonce")) == "NONCE-A-SECRET"
        and str(b.get("nonce")) == "NONCE-B-SECRET"
        and str(a.get("nonce")) not in json.dumps(b, sort_keys=True)
        and str(b.get("nonce")) not in json.dumps(a, sort_keys=True)
    )
    require(ok == expect_pass, f"HARBOR-16 expected pass={expect_pass}, got ok={ok}")


def verify_harbor_17(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    """Held-out multi-language repair: Python + C; starter/wrong results fail."""
    py = artifact.get("python") or {}
    c = artifact.get("c") or {}
    obs = artifact.get("protocol_observations") or {}
    py_ok = (
        bool(py.get("held_out_passed"))
        and int(py.get("add_result") or -1) == 42
        and int(py.get("mul_result") or -1) == 120
        and isinstance(py.get("files_edited"), list)
        and len(py.get("files_edited") or []) >= 1
    )
    c_ok = (
        bool(c.get("held_out_passed"))
        and int(c.get("gcd_result") or -1) == 6
        and int(c.get("lcm_result") or -1) == 36
        and int(c.get("compile_exit_code") if c.get("compile_exit_code") is not None else 1) == 0
        and isinstance(c.get("files_edited"), list)
        and len(c.get("files_edited") or []) >= 1
        and "warning" in str(c.get("compile_stderr", "")).lower()
    )
    protocol_ok = all(
        bool(obs.get(key))
        for key in ("search", "read", "edit", "shell_tests", "noisy_compiler_output_seen")
    )
    ok = py_ok and c_ok and protocol_ok
    require(ok == expect_pass, f"HARBOR-17 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def verify_harbor_18(artifact: dict[str, Any], *, expect_pass: bool) -> None:
    """Canary must be explicitly supported or unsupported; silent drop / false green fail."""
    disposition = str(artifact.get("disposition", ""))
    acknowledged = bool(artifact.get("acknowledged"))
    forwarded = bool(artifact.get("forwarded"))
    dropped = bool(artifact.get("canary_dropped"))
    completed = bool(artifact.get("trial_completed"))
    canary = artifact.get("canary") or {}
    has_canary = bool(canary.get("name") or canary.get("header") or canary.get("tool_descriptor"))

    if disposition == "supported":
        ok = has_canary and acknowledged and forwarded and not dropped and completed
    elif disposition == "unsupported":
        reject = str(artifact.get("reject_reason", ""))
        ok = (
            has_canary
            and acknowledged
            and not forwarded
            and not dropped
            and not completed
            and bool(reject)
        )
    else:
        # Missing or ambiguous disposition (e.g. silent "ok") is never a pass.
        ok = False
    require(ok == expect_pass, f"HARBOR-18 expected pass={expect_pass}, got ok={ok} artifact={artifact}")


def missing_credentials_disposition(credentials_present: bool) -> str:
    """Live agent runs without credentials are blocked, never a pass."""
    return "ready" if credentials_present else "blocked"
