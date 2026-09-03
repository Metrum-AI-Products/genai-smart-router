#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Run or plan coding-agent compatibility matrix checks.

The default mock mode is deterministic and does not call a router or external
client. It creates isolated fixture workspaces and verifies the same file-level
outcomes that live client smokes must prove.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import tempfile
import time
from dataclasses import dataclass, asdict
from pathlib import Path
from typing import Iterable


CLIENTS = ("codex", "claude-code", "opencode", "aider")
TASKS = (
    "text",
    "repo-navigation",
    "single-file-edit",
    "multi-file-edit",
    "tool-heavy",
    "long-context",
    "image",
    "tiny-cap",
    "forced-model-group",
    "disallowed-model-group",
)


@dataclass
class MatrixResult:
    client: str
    client_command: str
    version: str
    model_group: str
    request_dialect: str
    task: str
    status: str
    verifier_result: str
    elapsed_ms: int
    fixture_dir: str
    output_file: str
    notes: str
    request_ids: list[str]
    selected_upstream_provider: str
    selected_upstream_model: str
    selected_upstream_dialect: str
    input_tokens: int | None
    output_tokens: int | None


def dialect_for(client: str) -> str:
    if client == "claude-code":
        return "anthropic_messages"
    if client == "codex":
        return "openai_responses"
    return "openai_chat"


def command_for(client: str) -> str:
    if client == "claude-code":
        return "claude"
    return client


def discover_version(command: str) -> str:
    path = shutil.which(command)
    if not path:
        return "not-installed"
    for args in ([path, "--version"], [path, "version"]):
        try:
            completed = subprocess.run(args, text=True, capture_output=True, timeout=5, check=False)
        except Exception:
            continue
        text = (completed.stdout or completed.stderr).strip().splitlines()
        if text:
            return text[0][:160]
    return "installed-version-unavailable"


def write_fixture(root: Path, client: str, task: str) -> Path:
    workdir = root / client / task
    workdir.mkdir(parents=True, exist_ok=True)
    (workdir / "README.md").write_text(
        "# Router Agent Fixture\n\n"
        "This repository is intentionally tiny. Client smokes must operate only inside this directory.\n",
        encoding="utf-8",
    )
    (workdir / "app.py").write_text(
        "def route_label():\n"
        "    return 'before'\n",
        encoding="utf-8",
    )
    (workdir / "tests.py").write_text(
        "from app import route_label\n\n"
        "assert route_label() == 'after'\n",
        encoding="utf-8",
    )
    (workdir / "long_context.txt").write_text(("router validation context\n" * 256), encoding="utf-8")
    (workdir / "image.txt").write_text("fixture image placeholder: receipt merchant Rite Aid\n", encoding="utf-8")
    return workdir


def apply_mock_task(workdir: Path, task: str) -> tuple[str, str]:
    if task == "text":
        out = workdir / "text_result.txt"
        out.write_text("router-agent-ok\n", encoding="utf-8")
        return str(out), "passed"
    if task == "repo-navigation":
        out = workdir / "repo_summary.txt"
        out.write_text("found app.py and tests.py\n", encoding="utf-8")
        return str(out), "passed"
    if task == "single-file-edit":
        (workdir / "app.py").write_text("def route_label():\n    return 'after'\n", encoding="utf-8")
        return str(workdir / "app.py"), verify_python_fixture(workdir)
    if task == "multi-file-edit":
        (workdir / "app.py").write_text("def route_label():\n    return 'after'\n", encoding="utf-8")
        (workdir / "CHANGELOG.md").write_text("- after\n", encoding="utf-8")
        return str(workdir / "CHANGELOG.md"), verify_python_fixture(workdir)
    if task == "tool-heavy":
        out = workdir / "tool_result.txt"
        listing = sorted(p.name for p in workdir.iterdir())
        out.write_text("\n".join(listing) + "\n", encoding="utf-8")
        return str(out), "passed" if "app.py" in listing else "failed"
    if task == "long-context":
        out = workdir / "long_context_result.txt"
        count = (workdir / "long_context.txt").read_text(encoding="utf-8").count("router validation context")
        out.write_text(f"context_lines={count}\n", encoding="utf-8")
        return str(out), "passed" if count == 256 else "failed"
    if task == "image":
        out = workdir / "image_result.txt"
        out.write_text("Rite Aid\n", encoding="utf-8")
        return str(out), "passed"
    if task == "tiny-cap":
        out = workdir / "tiny_cap_result.txt"
        out.write_text("OK\n", encoding="utf-8")
        return str(out), "passed"
    if task == "forced-model-group":
        out = workdir / "forced_model_group.txt"
        out.write_text("forced model group accepted\n", encoding="utf-8")
        return str(out), "passed"
    if task == "disallowed-model-group":
        out = workdir / "disallowed_model_group.txt"
        out.write_text("expected router 403/404/400 for unavailable group\n", encoding="utf-8")
        return str(out), "manual-negative"
    raise ValueError(f"unknown task {task}")


def verify_python_fixture(workdir: Path) -> str:
    completed = subprocess.run(
        ["python3", "tests.py"],
        cwd=workdir,
        text=True,
        capture_output=True,
        timeout=10,
        check=False,
    )
    return "passed" if completed.returncode == 0 else "failed"


def load_env_file(path: str | None) -> None:
    if not path:
        return
    for raw_line in Path(path).read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ.setdefault(key.strip(), value.strip().strip('"').strip("'"))


def run_matrix(args: argparse.Namespace) -> list[MatrixResult]:
    load_env_file(args.env_file)
    clients = tuple(c.strip() for c in args.clients.split(",") if c.strip())
    tasks = tuple(t.strip() for t in args.tasks.split(",") if t.strip())
    invalid_clients = sorted(set(clients) - set(CLIENTS))
    invalid_tasks = sorted(set(tasks) - set(TASKS))
    if invalid_clients:
        raise SystemExit(f"unknown clients: {', '.join(invalid_clients)}")
    if invalid_tasks:
        raise SystemExit(f"unknown tasks: {', '.join(invalid_tasks)}")

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    fixture_root = Path(args.fixture_root) if args.fixture_root else Path(tempfile.mkdtemp(prefix="router-agent-matrix-"))
    results: list[MatrixResult] = []
    for client in clients:
        command = command_for(client)
        version = discover_version(command)
        for task in tasks:
            started = time.monotonic()
            workdir = write_fixture(fixture_root, client, task)
            output_file = ""
            verifier = "not-run"
            status = "skipped"
            notes = ""
            if args.mode == "mock":
                output_file, verifier = apply_mock_task(workdir, task)
                status = "passed" if verifier in {"passed", "manual-negative"} else "failed"
                notes = "deterministic mock verifier; no router or external client call"
            elif version == "not-installed":
                status = "skipped"
                verifier = "client-not-installed"
                notes = "install the client and rerun with --mode live, or use the manual runbook"
            else:
                status = "manual"
                verifier = "live-command-not-executed"
                notes = "live mode records availability; run the documented client command in an isolated workspace"
            elapsed_ms = int((time.monotonic() - started) * 1000)
            results.append(
                MatrixResult(
                    client=client,
                    client_command=command,
                    version=version,
                    model_group=args.model_group,
                    request_dialect=dialect_for(client),
                    task=task,
                    status=status,
                    verifier_result=verifier,
                    elapsed_ms=elapsed_ms,
                    fixture_dir=str(workdir),
                    output_file=output_file,
                    notes=notes,
                    request_ids=[],
                    selected_upstream_provider="",
                    selected_upstream_model="",
                    selected_upstream_dialect="",
                    input_tokens=None,
                    output_tokens=None,
                )
            )
    return results


def write_outputs(results: Iterable[MatrixResult], output_dir: Path) -> tuple[Path, Path]:
    rows = [asdict(r) for r in results]
    json_path = output_dir / "coding-agent-matrix.json"
    md_path = output_dir / "coding-agent-matrix.md"
    json_path.write_text(json.dumps({"results": rows}, indent=2) + "\n", encoding="utf-8")
    lines = [
        "# Coding-Agent Compatibility Matrix",
        "",
        "| Client | Version | Dialect | Model group | Task | Status | Verifier | Elapsed ms | Notes |",
        "|---|---|---|---|---|---|---|---:|---|",
    ]
    for row in rows:
        lines.append(
            "| {client} | {version} | {request_dialect} | {model_group} | {task} | {status} | {verifier_result} | {elapsed_ms} | {notes} |".format(
                **{k: str(v).replace("|", "\\|") for k, v in row.items()}
            )
        )
    md_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return json_path, md_path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("mock", "live"), default="mock")
    parser.add_argument("--clients", default=",".join(CLIENTS))
    parser.add_argument("--tasks", default=",".join(TASKS))
    parser.add_argument("--model-group", default="big-coder")
    parser.add_argument("--disallowed-model-group", default="not-allowed-smoke")
    parser.add_argument("--router-base-url", default=os.environ.get("ROUTER_BASE_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--env-file")
    parser.add_argument("--fixture-root")
    parser.add_argument("--output-dir", default="tmp/coding-agent-matrix")
    args = parser.parse_args()
    _ = args.router_base_url
    _ = args.disallowed_model_group
    results = run_matrix(args)
    json_path, md_path = write_outputs(results, Path(args.output_dir))
    failed = [r for r in results if r.status == "failed"]
    print(f"wrote {json_path}")
    print(f"wrote {md_path}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
