# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Run Harbor task verifiers against staged workspaces (offline)."""

from __future__ import annotations

import importlib.util
import shutil
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from .paths import TASK_DIR_BY_ID


@dataclass(frozen=True)
class VerifierResult:
    passed: bool
    reward: float
    detail: str = ""


def _load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise ImportError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def stage_workspace(task_id: str, *, mode: str) -> Path:
    """Copy task workspace into a temp dir and apply starter / solution / noop.

    Modes:
      - starter: leave the checked-in starter artifacts (must fail verifier)
      - noop: identical to starter (explicit no-edit control)
      - solution: apply reference solution
      - wrong: apply a deliberately incorrect artifact when available
    """
    task_dir = TASK_DIR_BY_ID[task_id]
    src = task_dir / "environment" / "workspace"
    if not src.is_dir():
        raise FileNotFoundError(f"missing workspace for {task_id}: {src}")

    root = Path(tempfile.mkdtemp(prefix=f"harbor-{task_id.lower()}-"))
    dest = root / "workspace"
    shutil.copytree(src, dest)

    if mode in {"starter", "noop"}:
        return dest

    apply_path = task_dir / "solution" / "apply.py"
    if not apply_path.is_file():
        raise FileNotFoundError(f"missing solution apply.py for {task_id}")
    apply_mod = _load_module(apply_path, f"harbor_apply_{task_id}_{mode}")
    apply_fn: Callable[[Path, str], None] = apply_mod.apply
    apply_fn(dest, mode)
    return dest


def run_verifier(task_id: str, workspace: Path) -> VerifierResult:
    task_dir = TASK_DIR_BY_ID[task_id]
    verify_path = task_dir / "tests" / "verify.py"
    if not verify_path.is_file():
        raise FileNotFoundError(f"missing verify.py for {task_id}")
    verify_mod = _load_module(verify_path, f"harbor_verify_{task_id}")
    result: Any = verify_mod.verify(workspace)
    if isinstance(result, VerifierResult):
        return result
    if isinstance(result, dict):
        return VerifierResult(
            passed=bool(result.get("passed")),
            reward=float(result.get("reward", 1.0 if result.get("passed") else 0.0)),
            detail=str(result.get("detail", "")),
        )
    if isinstance(result, tuple) and len(result) >= 2:
        return VerifierResult(passed=bool(result[0]), reward=float(result[1]), detail=str(result[2] if len(result) > 2 else ""))
    raise TypeError(f"unexpected verifier return type for {task_id}: {type(result)!r}")


def evaluate_mode(task_id: str, mode: str) -> VerifierResult:
    workspace = stage_workspace(task_id, mode=mode)
    try:
        return run_verifier(task_id, workspace)
    finally:
        shutil.rmtree(workspace.parent, ignore_errors=True)
