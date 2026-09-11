#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Credential-free contract tests for the live API-compat runner."""

from __future__ import annotations

import json
import os
import stat
import subprocess
import tempfile
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from threading import Thread


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "api_compat_live.py"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def run_script(env: dict[str, str], *args: str) -> subprocess.CompletedProcess[str]:
    merged = os.environ.copy()
    merged.update(env)
    return subprocess.run(
        ["python3", str(SCRIPT), *args],
        cwd=ROOT,
        env=merged,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def write_credential(path: Path, payload: dict | str, mode: int = 0o600) -> None:
    if isinstance(payload, dict):
        path.write_text(json.dumps(payload), encoding="utf-8")
    else:
        path.write_text(payload, encoding="utf-8")
    path.chmod(mode)


def test_ci_report_blocked_without_secrets() -> None:
    completed = run_script(
        {
            "API_COMPAT_LIVE_BASE_URL": "",
            "API_COMPAT_LIVE_CREDENTIAL": "",
            "API_COMPAT_LIVE_CREDENTIAL_FILE": "",
            "API_COMPAT_LIVE_MATRIX": "",
            "API_COMPAT_LIVE_ENVIRONMENT": "",
            "API_COMPAT_LIVE_CALLER": "",
            "API_COMPAT_LIVE_CONFIRM": "",
        },
        "--ci-report",
    )
    require(completed.returncode == 0, f"ci-report must exit 0:\n{completed.stderr}")
    report = json.loads(completed.stdout)
    require(report["disposition"] == "blocked", report)
    require(report["certification"] is False, report)
    require("not a certification pass" in completed.stderr, completed.stderr)


def test_missing_credentials_are_blocked_not_green() -> None:
    with tempfile.TemporaryDirectory(prefix="api-compat-live-") as temporary:
        root = Path(temporary)
        credential = root / "empty.cred"
        write_credential(credential, {"token": ""})
        completed = run_script(
            {
                "API_COMPAT_LIVE_MATRIX": "smoke-nonprod",
                "API_COMPAT_LIVE_ENVIRONMENT": "staging-smoke",
                "API_COMPAT_LIVE_CALLER": "eval-least-privilege",
                "API_COMPAT_LIVE_BASE_URL": "http://127.0.0.1:9",
                "API_COMPAT_LIVE_CREDENTIAL_FILE": str(credential),
                "API_COMPAT_LIVE_CONFIRM": "smoke-nonprod:staging-smoke",
            }
        )
        require(completed.returncode == 2, f"expected blocked exit 2:\n{completed.stderr}")
        report = json.loads(completed.stdout)
        require(report["disposition"] == "blocked", report)
        require(report["certification"] is False, report)
        require("credentials unavailable" in completed.stderr.lower() or "empty token" in completed.stderr.lower(), completed.stderr)


def test_credential_mode_must_be_0600() -> None:
    with tempfile.TemporaryDirectory(prefix="api-compat-live-mode-") as temporary:
        root = Path(temporary)
        credential = root / "loose.cred"
        write_credential(credential, {"token": "rtr_test", "model_group": "smoke"}, mode=0o644)
        completed = run_script(
            {
                "API_COMPAT_LIVE_MATRIX": "smoke-nonprod",
                "API_COMPAT_LIVE_ENVIRONMENT": "staging-smoke",
                "API_COMPAT_LIVE_CALLER": "eval-least-privilege",
                "API_COMPAT_LIVE_BASE_URL": "http://127.0.0.1:9",
                "API_COMPAT_LIVE_CREDENTIAL_FILE": str(credential),
                "API_COMPAT_LIVE_CONFIRM": "smoke-nonprod:staging-smoke",
            }
        )
        require(completed.returncode == 2, completed.stderr)
        require("0600" in completed.stderr, completed.stderr)


def test_production_environment_rejected() -> None:
    with tempfile.TemporaryDirectory(prefix="api-compat-live-prod-") as temporary:
        root = Path(temporary)
        credential = root / "ok.cred"
        write_credential(credential, {"token": "rtr_test", "model_group": "smoke"})
        completed = run_script(
            {
                "API_COMPAT_LIVE_MATRIX": "smoke-nonprod",
                "API_COMPAT_LIVE_ENVIRONMENT": "production",
                "API_COMPAT_LIVE_CALLER": "eval-least-privilege",
                "API_COMPAT_LIVE_BASE_URL": "https://example.invalid",
                "API_COMPAT_LIVE_CREDENTIAL_FILE": str(credential),
                "API_COMPAT_LIVE_CONFIRM": "smoke-nonprod:production",
            }
        )
        require(completed.returncode == 2, completed.stderr)
        require("non-production" in completed.stderr, completed.stderr)


def test_unknown_matrix_rejected() -> None:
    with tempfile.TemporaryDirectory(prefix="api-compat-live-matrix-") as temporary:
        root = Path(temporary)
        credential = root / "ok.cred"
        write_credential(credential, {"token": "rtr_test", "model_group": "smoke"})
        completed = run_script(
            {
                "API_COMPAT_LIVE_MATRIX": "not-a-real-matrix",
                "API_COMPAT_LIVE_ENVIRONMENT": "staging-smoke",
                "API_COMPAT_LIVE_CALLER": "eval-least-privilege",
                "API_COMPAT_LIVE_BASE_URL": "https://example.invalid",
                "API_COMPAT_LIVE_CREDENTIAL_FILE": str(credential),
                "API_COMPAT_LIVE_CONFIRM": "not-a-real-matrix:staging-smoke",
            }
        )
        require(completed.returncode == 2, completed.stderr)
        require("not found" in completed.stderr, completed.stderr)


def test_bounded_loopback_smoke_against_fake_router() -> None:
    class Handler(BaseHTTPRequestHandler):
        counts = {"n": 0}

        def log_message(self, format: str, *args) -> None:  # noqa: A003
            return

        def do_POST(self) -> None:  # noqa: N802
            length = int(self.headers.get("Content-Length", "0"))
            _ = self.rfile.read(length)
            Handler.counts["n"] += 1
            body = {
                "id": "resp_fake",
                "object": "response" if self.path.endswith("/responses") else "chat.completion",
                "choices": [{"message": {"role": "assistant", "content": "ok"}}],
                "usage": {"prompt_tokens": 4, "completion_tokens": 1, "total_tokens": 5},
            }
            if "anthropic" in self.path:
                body = {
                    "id": "msg_fake",
                    "type": "message",
                    "role": "assistant",
                    "content": [{"type": "text", "text": "ok"}],
                    "usage": {"input_tokens": 4, "output_tokens": 1},
                }
            raw = json.dumps(body).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

    server = HTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="api-compat-live-ok-") as temporary:
            root = Path(temporary)
            credential = root / "ok.cred"
            write_credential(credential, {"token": "rtr_test", "model_group": "smoke"})
            port = server.server_address[1]
            completed = run_script(
                {
                    "API_COMPAT_LIVE_MATRIX": "smoke-nonprod",
                    "API_COMPAT_LIVE_ENVIRONMENT": "staging-smoke",
                    "API_COMPAT_LIVE_CALLER": "eval-least-privilege",
                    "API_COMPAT_LIVE_BASE_URL": f"http://127.0.0.1:{port}",
                    "API_COMPAT_LIVE_CREDENTIAL_FILE": str(credential),
                    "API_COMPAT_LIVE_CONFIRM": "smoke-nonprod:staging-smoke",
                }
            )
            require(completed.returncode == 0, f"{completed.stdout}\n{completed.stderr}")
            report = json.loads(completed.stdout)
            require(report["disposition"] == "passed", report)
            require(report["certification"] is False, report)
            require(report["requests"] == 3, report)
            require(Handler.counts["n"] == 3, Handler.counts)
            require(stat.S_IMODE(credential.stat().st_mode) == 0o600, "credential mode drifted")
    finally:
        server.shutdown()


def main() -> int:
    test_ci_report_blocked_without_secrets()
    test_missing_credentials_are_blocked_not_green()
    test_credential_mode_must_be_0600()
    test_production_environment_rejected()
    test_unknown_matrix_rejected()
    test_bounded_loopback_smoke_against_fake_router()
    print("api_compat_live_test: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
