#!/usr/bin/env python3
"""Offline deterministic regression for the LiveCodeBench evaluator contract."""
from __future__ import annotations
import importlib.util
import subprocess
import types
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("livecodebench_eval", ROOT / "scripts/livecodebench_eval.py")
assert SPEC and SPEC.loader
LCB = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(LCB)

def need(ok: bool, message: str) -> None:
    if not ok: raise AssertionError(message)

def rejects(fn, text: str) -> None:
    try: fn()
    except LCB.ContractError as exc: need(text in str(exc), str(exc))
    else: raise AssertionError("expected contract rejection")

def main() -> int:
    values=LCB.contract(); need(values["release_version"] == "release_v6" and values["task_count"] == 40, "release/sample pin changed")
    tasks=[{"question_id": f"task-{i:03d}"} for i in range(80)]
    first=LCB.select_tasks(tasks, seed=values["sampling_seed"], count=40); second=LCB.select_tasks(reversed(tasks), seed=values["sampling_seed"], count=40)
    first_ids=[LCB.task_id(task) for task in first]; need(first_ids == [LCB.task_id(task) for task in second] and len(set(first_ids)) == 40, "sample is not deterministic and distinct")
    rejects(lambda: LCB.select_tasks(tasks[:39], seed="x", count=40), "fewer tasks")
    rejects(lambda: LCB.select_tasks(tasks + [tasks[0]], seed="x", count=40), "distinct")
    original_import, original_run = LCB.importlib.import_module, LCB.subprocess.run
    try:
        loaded=[]
        fake_loader=lambda release: loaded.append(release) or tasks
        LCB.importlib.import_module=lambda name: types.SimpleNamespace(load_code_generation_dataset=fake_loader) if name == "lcb_runner.benchmarks.code_generation" else types.SimpleNamespace(__version__=values["datasets_version"])
        need(LCB.load_official_tasks() == tasks and loaded == ["release_v6"], "official loader contract did not select pinned release")
        LCB.subprocess.run=lambda *args, **kwargs: subprocess.CompletedProcess(args, 0, values["livecodebench_revision"]+"\n", "")
        LCB.validate_environment(Path("/safe/lcb"))
        LCB.importlib.import_module=lambda name: types.SimpleNamespace(__version__="5.0.0")
        rejects(lambda: LCB.validate_environment(Path("/safe/lcb")), "datasets version")
        LCB.importlib.import_module=lambda name: (_ for _ in ()).throw(ImportError("missing"))
        rejects(LCB.load_official_tasks, "unavailable before inference")
    finally:
        LCB.importlib.import_module, LCB.subprocess.run = original_import, original_run
    calls=[]
    result=LCB.aggregate(first, lambda task: calls.append(LCB.task_id(task)) or "generated", lambda task, response: response == "generated")
    need(len(calls) == 40 and result == {"status":"completed", "release_version":"release_v6", "selected":40, "completed":40, "scored":40, "errors":0, "pass_at_1":1.0}, "40-task completed/scored aggregate changed")
    rejects(lambda: LCB.aggregate(first, lambda task: "", lambda task, response: True), "did not complete")
    rendered=str(result); need("task-" not in rendered and "generated" not in rendered, "aggregate leaked task or response content")
    print("livecodebench evaluator tests passed")
    return 0

if __name__ == "__main__": raise SystemExit(main())
