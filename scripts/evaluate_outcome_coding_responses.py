#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Evaluate bounded, synthetic coding responses for the live routing reference.

This verifier intentionally accepts only the three published synthetic task
shapes. It rejects Markdown wrappers, dangerous imports, dynamic evaluation,
and suspicious names before running code in a disposable directory with a
short timeout. It is not a general-purpose sandbox.
"""

from __future__ import annotations

import argparse
import ast
import json
import http.server
import threading
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any


ALLOWED_IMPORTS = {
    "medium-csv": {"__future__", "csv", "io", "typing"},
    "difficult-benchmark": {"argparse", "concurrent.futures", "json", "math", "statistics", "sys", "threading", "time", "urllib.error", "urllib.parse", "urllib.request"},
}
FORBIDDEN_NAMES = {"__builtins__", "__import__", "compile", "eval", "exec", "globals", "locals", "open", "input"}


def code_only(response: str) -> str:
    text = response.strip()
    first = text.find("```")
    if first >= 0:
        lines = text[first:].splitlines()
        if len(lines) < 3 or not any(line.strip().startswith("```") for line in lines[1:]):
            raise ValueError("markdown-wrapper")
        end = next(index for index, line in enumerate(lines[1:], 1) if line.strip().startswith("```"))
        text = "\n".join(lines[1:end]).strip()
    return text


def validate_ast(case_id: str, code: str) -> None:
    tree = ast.parse(code)
    allowed = ALLOWED_IMPORTS.get(case_id, set())
    for node in ast.walk(tree):
        if isinstance(node, (ast.Import, ast.ImportFrom)):
            module = node.names[0].name if isinstance(node, ast.Import) else (node.module or "")
            if not module or module not in allowed:
                raise ValueError("import-not-allowed")
        if isinstance(node, ast.Name) and node.id in FORBIDDEN_NAMES:
            raise ValueError("unsafe-name")


def run_python(path: Path, code: str) -> subprocess.CompletedProcess[str]:
    source = path / "candidate.py"
    source.write_text(code, encoding="utf-8")
    return subprocess.run([sys.executable, str(source)], cwd=path, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=8, env={"PATH": "/usr/bin:/bin", "PYTHONIOENCODING": "utf-8"})


def evaluate(case_id: str, response: str) -> tuple[bool, str]:
    try:
        code = code_only(response)
        validate_ast(case_id, code)
    except (SyntaxError, ValueError) as error:
        return False, str(error)
    with tempfile.TemporaryDirectory(prefix="outcome-coding-eval-") as raw_dir:
        path = Path(raw_dir)
        try:
            if case_id == "simple-add":
                test = "\n".join([code, "assert add_two(2, 2) == 4", "assert add_two(-3, 5) == 2"])
                result = run_python(path, test)
            elif case_id == "medium-csv":
                test = "\n".join([code, "assert parse_scores('ada, 3\\nada,9\\nbea, 4\\n') == {'ada': 9, 'bea': 4}", "for value in ('bad', ',1', 'a,nope'):", "    try:", "        parse_scores(value)", "        raise AssertionError(value)", "    except ValueError:", "        pass"])
                result = run_python(path, test)
            elif case_id == "difficult-benchmark":
                source = path / "candidate.py"
                source.write_text(code, encoding="utf-8")
                httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), http.server.SimpleHTTPRequestHandler)
                worker = threading.Thread(target=httpd.serve_forever, daemon=True)
                worker.start()
                try:
                    url = f"http://127.0.0.1:{httpd.server_port}/"
                    result = subprocess.run([sys.executable, str(source), url, "--requests", "3", "--concurrency", "2"], cwd=path, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=8, env={"PATH": "/usr/bin:/bin", "PYTHONIOENCODING": "utf-8"})
                finally:
                    httpd.shutdown()
                    httpd.server_close()
                if result.returncode == 0:
                    output = json.loads(result.stdout)
                    if output == {"requested": 3, "completed": 3, "succeeded": 3, "failed": 0, "p95_ms": output.get("p95_ms")} and isinstance(output.get("p95_ms"), (float, int)):
                        return True, "benchmark-execution-passed"
                return False, "benchmark-execution-failed"
            else:
                return False, "unknown-case"
        except subprocess.TimeoutExpired:
            return False, "execution-timeout"
        if result.returncode == 0:
            return True, "checks-passed"
        return False, "checks-failed"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reviews", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    rows = [json.loads(line) for line in args.reviews.read_text(encoding="utf-8").splitlines() if line.strip()]
    outcomes: list[dict[str, Any]] = []
    for row in rows:
        passed, verdict = evaluate(str(row.get("caseId", "")), str(row.get("response", ""))) if not row.get("error") else (False, "router-request-failed")
        row["passed"] = passed
        row["verdict"] = verdict
        outcomes.append(row)
    args.out.write_text("".join(json.dumps(row, sort_keys=True) + "\n" for row in outcomes), encoding="utf-8")
    print(json.dumps({"reviews": len(outcomes), "passed": sum(bool(row["passed"]) for row in outcomes), "out": str(args.out)}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
