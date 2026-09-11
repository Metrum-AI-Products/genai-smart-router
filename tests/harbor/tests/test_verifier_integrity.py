# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import pytest

from harness.paths import TASK_DIR_BY_ID, TASK_IDS
from harness.runner import evaluate_mode


@pytest.mark.parametrize("task_id", TASK_IDS)
def test_task_layout_present(task_id: str):
    task_dir = TASK_DIR_BY_ID[task_id]
    assert (task_dir / "task.toml").is_file()
    assert (task_dir / "instruction.md").is_file()
    assert (task_dir / "environment" / "Dockerfile").is_file()
    assert (task_dir / "environment" / "workspace").is_dir()
    assert (task_dir / "tests" / "verify.py").is_file()
    assert (task_dir / "solution" / "apply.py").is_file()


@pytest.mark.parametrize("task_id", TASK_IDS)
def test_reference_passes_and_starter_fails(task_id: str):
    solution = evaluate_mode(task_id, "solution")
    assert solution.passed is True, f"{task_id} solution failed: {solution.detail}"
    assert solution.reward == 1.0

    starter = evaluate_mode(task_id, "starter")
    assert starter.passed is False, f"{task_id} starter unexpectedly passed"
    assert starter.reward == 0.0

    noop = evaluate_mode(task_id, "noop")
    assert noop.passed is False, f"{task_id} noop unexpectedly passed"


@pytest.mark.parametrize("task_id", TASK_IDS)
def test_wrong_result_fails(task_id: str):
    wrong = evaluate_mode(task_id, "wrong")
    assert wrong.passed is False, f"{task_id} wrong result unexpectedly passed: {wrong.detail}"
    assert wrong.reward == 0.0
