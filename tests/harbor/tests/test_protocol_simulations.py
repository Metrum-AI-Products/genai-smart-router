# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Protocol-level simulations for HARBOR-02..06 beyond verifier integrity."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import sys
import tempfile
from pathlib import Path

from harness.paths import TASK_DIR_BY_ID
from harness.runner import evaluate_mode, run_verifier, stage_workspace


def _load(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def test_harbor02_static_canned_fails_fresh_nonce_passes():
    static = evaluate_mode("HARBOR-02", "starter")
    assert static.passed is False
    assert "static" in static.detail.lower() or "canned" in static.detail.lower() or "digest" in static.detail.lower()

    first = evaluate_mode("HARBOR-02", "solution")
    second = evaluate_mode("HARBOR-02", "solution")
    assert first.passed and second.passed
    # Two reference runs use independent nonces; both must still verify.


def test_harbor03_randomized_completion_order_preserves_ids():
    tools = _load(
        TASK_DIR_BY_ID["HARBOR-03"] / "tools" / "parallel_lookup.py",
        "harbor03_sim",
    )
    for seed in (1, 2, 3, 99):
        results = tools.execute_parallel(tools.default_requests(), seed=seed)
        names = {item.name for item in results}
        assert names == {"lookup"}
        mapped = tools.map_by_call_id(results)
        assert mapped == {
            "call_a": "value-alpha",
            "call_b": "value-bravo",
            "call_c": "value-charlie",
        }


def test_harbor04_error_then_retry_bounded():
    tools = _load(
        TASK_DIR_BY_ID["HARBOR-04"] / "tools" / "fetch_record.py",
        "harbor04_sim",
    )
    with tempfile.TemporaryDirectory() as tmp:
        workspace = Path(tmp)
        (workspace / "record.json").write_text("{}\n", encoding="utf-8")
        session = tools.run_reference(workspace)
        assert len(session.attempts) == 2
        assert session.attempts[0]["is_error"] is True
        assert session.attempts[1]["is_error"] is False
        assert len(session.attempts) <= tools.MAX_ATTEMPTS
        data = json.loads((workspace / "record.json").read_text(encoding="utf-8"))
        assert data["id"] == tools.CORRECT_ID


def test_harbor05_stream_fragments_round_trip_hash():
    sim = _load(
        TASK_DIR_BY_ID["HARBOR-05"] / "environment" / "workspace" / "stream_patch.py",
        "harbor05_sim",
    )
    fragments = sim.fragment_argument(sim.TARGET_BODY, chunk_size=17)
    assert len(fragments) > 5
    rebuilt = sim.reassemble_argument(fragments)
    assert rebuilt == sim.TARGET_BODY
    assert hashlib.sha256(rebuilt.encode()).hexdigest() == sim.EXPECTED_SHA256

    # Truncation must not match the expected hash.
    truncated = sim.TARGET_BODY[:-20]
    assert hashlib.sha256(truncated.encode()).hexdigest() != sim.EXPECTED_SHA256


def test_harbor06_post_commit_disconnect_no_replay_and_precommit_control():
    tools = _load(
        TASK_DIR_BY_ID["HARBOR-06"] / "tools" / "side_effect.py",
        "harbor06_sim",
    )
    with tempfile.TemporaryDirectory() as tmp:
        workspace = Path(tmp)
        (workspace / "ledger.json").write_text(
            json.dumps({"counter": 0, "entries": []}) + "\n", encoding="utf-8"
        )
        ok = tools.run_reference_post_commit_disconnect(workspace)
        assert ok.automatic_replays == 0
        ledger = json.loads((workspace / "ledger.json").read_text(encoding="utf-8"))
        assert ledger["counter"] == 1

    with tempfile.TemporaryDirectory() as tmp:
        workspace = Path(tmp)
        (workspace / "ledger.json").write_text(
            json.dumps({"counter": 0, "entries": []}) + "\n", encoding="utf-8"
        )
        bad = tools.run_illegal_replay(workspace)
        assert bad.automatic_replays == 1
        ledger = json.loads((workspace / "ledger.json").read_text(encoding="utf-8"))
        assert ledger["counter"] == 2

    with tempfile.TemporaryDirectory() as tmp:
        workspace = Path(tmp)
        (workspace / "ledger.json").write_text(
            json.dumps({"counter": 0, "entries": []}) + "\n", encoding="utf-8"
        )
        control = tools.run_pre_commit_control(workspace)
        assert control.automatic_replays == 0
        ledger = json.loads((workspace / "ledger.json").read_text(encoding="utf-8"))
        assert ledger["counter"] == 1


def test_harbor06_staged_illegal_replay_fails_verifier():
    workspace = stage_workspace("HARBOR-06", mode="wrong")
    try:
        result = run_verifier("HARBOR-06", workspace)
        assert result.passed is False
    finally:
        import shutil

        shutil.rmtree(workspace.parent, ignore_errors=True)
