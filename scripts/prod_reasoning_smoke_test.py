#!/usr/bin/env python3
"""Tests for reasoning_smoke.py."""

from __future__ import annotations

import io
import json
import os
import sqlite3
import tempfile
import threading
import unittest
from contextlib import redirect_stderr, redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from unittest import mock

import prod_reasoning_smoke as wrapper
import reasoning_smoke as smoke


class MockRouterHandler(BaseHTTPRequestHandler):
    request_ids: dict[str, str] = {
        "/v1/chat/completions": "req_chat",
        "/v1/responses": "req_responses",
        "/v1/messages": "req_anthropic",
    }
    advertise_reasoning = True
    advertise_default_reasoning = True
    empty_response_text = False

    def log_message(self, fmt: str, *args: Any) -> None:
        return

    def do_GET(self) -> None:
        if self.path != "/v1/models":
            self.send_error(404)
            return
        model: dict[str, Any] = {"id": "reasoning-smoke", "object": "model"}
        if self.advertise_reasoning:
            model["supported_reasoning_levels"] = [
                {"effort": "low", "description": "Low"},
                {"effort": "medium", "description": "Medium"},
                {"effort": "high", "description": "High"},
            ]
            if self.advertise_default_reasoning:
                model["default_reasoning_level"] = "medium"
        self.write_json(200, {"object": "list", "data": [model]})

    def do_POST(self) -> None:
        if self.path not in self.request_ids:
            self.send_error(404)
            return
        _ = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        text = "" if self.empty_response_text else "OK"
        if self.path == "/v1/chat/completions":
            body = {"choices": [{"message": {"role": "assistant", "content": text}}]}
        elif self.path == "/v1/responses":
            body = {"output_text": text}
        else:
            body = {"content": [{"type": "text", "text": text}]}
        self.write_json(200, body, {"X-Request-Id": self.request_ids[self.path]})

    def write_json(
        self, status: int, body: dict[str, Any], headers: dict[str, str] | None = None
    ) -> None:
        raw = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        for key, value in (headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(raw)


class ProdReasoningSmokeTest(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), MockRouterHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base_url = f"http://127.0.0.1:{self.server.server_port}"
        self.tmp = tempfile.TemporaryDirectory()
        self.db_path = str(Path(self.tmp.name) / "usage.sqlite")
        self.create_db()
        self.token_env = "ROUTER_REASONING_SMOKE_TEST_TOKEN"
        os.environ[self.token_env] = "rtr_test_token_not_printed"
        MockRouterHandler.advertise_reasoning = True
        MockRouterHandler.advertise_default_reasoning = True
        MockRouterHandler.empty_response_text = False

    def tearDown(self) -> None:
        self.server.shutdown()
        self.thread.join(timeout=5)
        self.server.server_close()
        self.tmp.cleanup()
        os.environ.pop(self.token_env, None)

    def create_db(self) -> None:
        with sqlite3.connect(self.db_path) as conn:
            conn.executescript(
                """
CREATE TABLE request_usage (
  request_id TEXT PRIMARY KEY,
  resolved_group TEXT NOT NULL,
  status INTEGER NOT NULL,
  fallback_used BOOLEAN NOT NULL
);
CREATE TABLE request_attempts (
  request_id TEXT NOT NULL,
  attempt_index INTEGER NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dialect TEXT NOT NULL,
  selected BOOLEAN NOT NULL,
  PRIMARY KEY (request_id, attempt_index)
);
CREATE TABLE request_translation_shapes (
  request_id TEXT NOT NULL,
  attempt_index INTEGER NOT NULL,
  translated_reasoning_control TEXT NOT NULL,
  PRIMARY KEY (request_id, attempt_index)
);
"""
            )
            rows = [
                ("req_chat", "fireworks", "accounts/fireworks/models/gpt-oss-20b", "openai-chat", "reasoning_effort"),
                ("req_responses", "minimax_responses", "MiniMax-M3", "openai-responses", "reasoning"),
                ("req_anthropic", "kimi_anthropic", "kimi-k2.7-code", "anthropic", "thinking"),
            ]
            for request_id, provider, model, dialect, control in rows:
                conn.execute(
                    "INSERT INTO request_usage VALUES (?, ?, 200, 0)",
                    (request_id, "reasoning-smoke"),
                )
                conn.execute(
                    "INSERT INTO request_attempts VALUES (?, 0, ?, ?, ?, 1)",
                    (request_id, provider, model, dialect),
                )
                conn.execute(
                    "INSERT INTO request_translation_shapes VALUES (?, 0, ?)",
                    (request_id, control),
                )

    def run_smoke(self, *extra: str) -> tuple[int, str]:
        args = [
            "--base-url",
            self.base_url,
            "--token-env",
            self.token_env,
            "--sqlite-db",
            self.db_path,
            "--telemetry-timeout",
            "0.1",
            "--telemetry-poll-interval",
            "0.01",
            *extra,
        ]
        out = io.StringIO()
        with redirect_stdout(out):
            code = smoke.main(args)
        return code, out.getvalue()

    def test_full_mock_smoke_passes_and_does_not_print_token(self) -> None:
        code, output = self.run_smoke(
            "--expect",
            "chat:fireworks:accounts/fireworks/models/gpt-oss-20b:openai-chat",
            "--expect",
            "responses:minimax_responses:MiniMax-M3:openai-responses",
            "--expect",
            "anthropic:kimi_anthropic:kimi-k2.7-code:anthropic",
        )

        self.assertEqual(code, 0)
        self.assertIn("models_reasoning_metadata", output)
        self.assertIn("req_chat", output)
        self.assertIn("req_responses", output)
        self.assertIn("req_anthropic", output)
        self.assertNotIn(os.environ[self.token_env], output)

    def test_fails_when_models_metadata_missing(self) -> None:
        MockRouterHandler.advertise_reasoning = False

        with self.assertRaisesRegex(RuntimeError, "supported_reasoning_levels"):
            self.run_smoke("--surfaces", "chat")

    def test_accepts_opt_in_reasoning_without_default_level(self) -> None:
        MockRouterHandler.advertise_default_reasoning = False

        code, output = self.run_smoke("--surfaces", "chat")

        self.assertEqual(code, 0)
        self.assertIn('"defaultReasoningLevel": null', output)

    def test_fails_when_surface_returns_empty_visible_text(self) -> None:
        MockRouterHandler.empty_response_text = True

        with self.assertRaisesRegex(RuntimeError, "visible text"):
            self.run_smoke("--surfaces", "chat")

    def test_fails_when_telemetry_reasoning_control_is_wrong(self) -> None:
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                "UPDATE request_translation_shapes SET translated_reasoning_control = '' WHERE request_id = 'req_chat'"
            )

        with self.assertRaisesRegex(RuntimeError, "translated_reasoning_control"):
            self.run_smoke("--surfaces", "chat")

    def test_fails_when_selected_target_is_unexpected(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "selected"):
            self.run_smoke("--surfaces", "chat", "--expect", "chat:wrong:model:openai-chat")

    def test_compatibility_wrapper_preserves_json_runtime_errors(self) -> None:
        stderr = io.StringIO()
        with mock.patch.object(wrapper, "main", side_effect=RuntimeError("telemetry missing")):
            with redirect_stderr(stderr):
                code = wrapper.run([])
        self.assertEqual(code, 1)
        event = json.loads(stderr.getvalue())
        self.assertEqual(event["event"], "reasoning_smoke_failed")
        self.assertEqual(event["error"], "telemetry missing")


if __name__ == "__main__":
    unittest.main()
