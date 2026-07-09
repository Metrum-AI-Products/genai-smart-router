#!/usr/bin/env python3
"""Tests for prod_smoke_regressions.py."""

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

import prod_smoke_regressions as smoke


class RegressionSmokeHandler(BaseHTTPRequestHandler):
    requests_seen: list[str] = []
    bodies_seen: list[dict[str, Any]] = []

    def log_message(self, fmt: str, *args: Any) -> None:
        return

    def do_POST(self) -> None:
        RegressionSmokeHandler.requests_seen.append(self.path)
        raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        RegressionSmokeHandler.bodies_seen.append(json.loads(raw))
        if self.path == "/v1/chat/completions" and RegressionSmokeHandler.bodies_seen[-1].get("metadata", {}).get("scenario") == "expected-error":
            self.write_json(200, {"choices": [{"message": {"role": "assistant", "content": "unexpected"}}]}, {"X-Request-Id": "req_error_expected"})
            return
        if self.path == "/v1/chat/completions" and RegressionSmokeHandler.bodies_seen[-1].get("metadata", {}).get("scenario") == "unexpected-target":
            self.write_json(200, {"choices": [{"message": {"role": "assistant", "content": "unexpected"}}]}, {"X-Request-Id": "req_unexpected_target"})
            return
        if self.path == "/v1/chat/completions":
            self.write_json(200, {"choices": [{"message": {"role": "assistant", "content": "OK"}}]}, {"X-Request-Id": "req_chat"})
            return
        if self.path == "/v1/responses":
            self.write_json(200, {"output_text": "OK"}, {"X-Request-Id": "req_responses"})
            return
        if self.path == "/v1/messages":
            self.write_json(200, {"content": [{"type": "text", "text": "OK"}]}, {"X-Request-Id": "req_messages"})
            return
        self.send_error(404)

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


class ProdSmokeRegressionsTest(unittest.TestCase):
    def setUp(self) -> None:
        RegressionSmokeHandler.requests_seen = []
        RegressionSmokeHandler.bodies_seen = []
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), RegressionSmokeHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base_url = f"http://127.0.0.1:{self.server.server_port}"
        self.tmp = tempfile.TemporaryDirectory()
        self.fixtures = Path(self.tmp.name) / "fixtures"
        self.fixtures.mkdir()
        self.db_path = Path(self.tmp.name) / "usage.sqlite"
        self.create_db()
        self.token_env = "ROUTER_PROD_SMOKE_REGRESSION_TEST_TOKEN"
        os.environ[self.token_env] = "rtr_test_token_not_printed"

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
  status INTEGER NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  traffic_shape_decision TEXT NOT NULL DEFAULT ''
);
CREATE TABLE request_attempts (
  request_id TEXT NOT NULL,
  attempt_index INTEGER NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dialect TEXT NOT NULL,
  error_class TEXT NOT NULL DEFAULT '',
  selected BOOLEAN NOT NULL,
  PRIMARY KEY (request_id, attempt_index)
);
CREATE TABLE request_shapes (
  request_id TEXT PRIMARY KEY,
  message_count INTEGER NOT NULL,
  tool_count INTEGER NOT NULL,
  image_count INTEGER NOT NULL,
  total_request_bytes_bucket TEXT NOT NULL,
  tool_schema_bytes_bucket TEXT NOT NULL,
  estimated_input_tokens_bucket TEXT NOT NULL
);
CREATE TABLE request_translation_shapes (
  request_id TEXT NOT NULL,
  attempt_index INTEGER NOT NULL,
  translated_tool_count INTEGER NOT NULL,
  translated_request_bytes_bucket TEXT NOT NULL,
  PRIMARY KEY (request_id, attempt_index)
);
"""
            )
            rows = [
                ("req_chat", "minimax", "MiniMax-M3", "openai-chat", 3, 1),
                ("req_responses", "minimax_responses", "MiniMax-M3", "openai-responses", 1, 1),
                ("req_messages", "kimi_anthropic", "kimi-k2.7-code", "anthropic", 1, 1),
                ("req_error_expected", "minimax", "MiniMax-M3", "openai-chat", 1, 0),
                ("req_unexpected_target", "kimi", "kimi-k2.7-code", "openai-chat", 1, 1),
            ]
            for request_id, provider, model, dialect, messages, tools in rows:
                conn.execute("INSERT INTO request_usage VALUES (?, 200, '', 'admitted')", (request_id,))
                conn.execute(
                    "INSERT INTO request_attempts VALUES (?, 0, ?, ?, ?, '', 1)",
                    (request_id, provider, model, dialect),
                )
                conn.execute(
                    "INSERT INTO request_shapes VALUES (?, ?, ?, 0, '1kb-16kb', '1b-1kb', 'small')",
                    (request_id, messages, tools),
                )
                conn.execute(
                    "INSERT INTO request_translation_shapes VALUES (?, 0, ?, '1kb-16kb')",
                    (request_id, tools),
                )

    def write_fixture(self) -> None:
        fixture = {
            "name": "agent-smoke-test",
            "model_group": "reasoning-bridge-smoke",
            "scenarios": [
                {
                    "name": "chat",
                    "surface": "openai_chat",
                    "message_count": 3,
                    "tool_count": 1,
                    "reasoning_control": "reasoning_effort",
                    "reasoning_effort": "low",
                    "structured_output": True,
                    "request_bytes_bucket": "1kb-16kb",
                    "tool_schema_bytes_bucket": "1b-1kb",
                    "estimated_input_tokens_bucket": "small",
                    "production_smoke_safe_payload_template": "synthetic-chat-v1",
                },
                {
                    "name": "responses",
                    "surface": "openai_responses",
                    "message_count": 1,
                    "tool_count": 1,
                    "request_bytes_bucket": "1kb-16kb",
                    "tool_schema_bytes_bucket": "1b-1kb",
                    "estimated_input_tokens_bucket": "small",
                    "production_smoke_safe_payload_template": "synthetic-responses-v1",
                },
                {
                    "name": "messages",
                    "surface": "anthropic_messages",
                    "message_count": 1,
                    "tool_count": 1,
                    "request_bytes_bucket": "1kb-16kb",
                    "tool_schema_bytes_bucket": "1b-1kb",
                    "estimated_input_tokens_bucket": "small",
                    "production_smoke_safe_payload_template": "synthetic-messages-v1",
                },
            ],
        }
        (self.fixtures / "agent-smoke-test.json").write_text(json.dumps(fixture), encoding="utf-8")
        non_replayable = {
            "name": "diagnostics-only",
            "replayable": False,
            "surface": "openai_chat",
            "model_group": "diagnostics-only",
            "message_count": 1,
            "tool_count": 0,
        }
        (self.fixtures / "diagnostics-only.json").write_text(json.dumps(non_replayable), encoding="utf-8")

    def write_expected_error_fixture(self) -> None:
        fixture = {
            "name": "expected-error-test",
            "model_group": "reasoning-bridge-smoke",
            "scenarios": [
                {
                    "name": "expected-error",
                    "surface": "openai_chat",
                    "message_count": 1,
                    "tool_count": 0,
                    "expected_error_class": "no-eligible-target",
                    "request_bytes_bucket": "1kb-16kb",
                    "tool_schema_bytes_bucket": "1b-1kb",
                    "estimated_input_tokens_bucket": "small",
                    "production_smoke_safe_payload_template": "synthetic-error-v1",
                }
            ],
        }
        (self.fixtures / "expected-error-test.json").write_text(json.dumps(fixture), encoding="utf-8")

    def write_allowed_target_fixture(self) -> None:
        fixture = {
            "name": "allowed-target-test",
            "model_group": "high",
            "scenarios": [
                {
                    "name": "unexpected-target",
                    "surface": "openai_chat",
                    "message_count": 1,
                    "tool_count": 1,
                    "allowed_selected_targets": [
                        {"provider": "minimax", "model": "MiniMax-M3", "dialect": "openai-chat"}
                    ],
                    "production_smoke_safe_payload_template": "synthetic-allowed-target-v1",
                }
            ],
        }
        (self.fixtures / "allowed-target-test.json").write_text(json.dumps(fixture), encoding="utf-8")

    def run_smoke(self, *extra: str) -> tuple[int, str]:
        args = [
            "--mode",
            "local",
            "--base-url",
            self.base_url,
            "--fixtures-dir",
            str(self.fixtures),
            "--fixture",
            "all",
            "--token-env",
            self.token_env,
            "--sqlite-db",
            str(self.db_path),
            *extra,
        ]
        out = io.StringIO()
        err = io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = smoke.main(args)
        return code, out.getvalue()

    def test_all_surfaces_pass_and_do_not_print_token(self) -> None:
        self.write_fixture()

        code, output = self.run_smoke()

        self.assertEqual(code, 0)
        self.assertEqual(
            RegressionSmokeHandler.requests_seen,
            ["/v1/chat/completions", "/v1/responses", "/v1/messages"],
        )
        chat_body = RegressionSmokeHandler.bodies_seen[0]
        self.assertIn("tools", chat_body)
        self.assertEqual(chat_body["tool_choice"], "auto")
        self.assertEqual(chat_body["reasoning_effort"], "low")
        self.assertIn("response_format", chat_body)
        result = json.loads(output)
        self.assertEqual(len(result["results"]), 3)
        self.assertNotIn(os.environ[self.token_env], output)

    def test_fixture_all_skips_non_replayable_fixtures(self) -> None:
        self.write_fixture()

        code, output = self.run_smoke()

        self.assertEqual(code, 0)
        result = json.loads(output)
        self.assertEqual({row["fixture"] for row in result["results"]}, {"agent-smoke-test"})

    def test_expected_error_class_requires_non_2xx_response(self) -> None:
        self.write_expected_error_fixture()

        code, _ = self.run_smoke()

        self.assertEqual(code, 1)

    def test_allowed_selected_targets_are_enforced(self) -> None:
        self.write_allowed_target_fixture()

        code, _ = self.run_smoke()

        self.assertEqual(code, 1)

    def test_gt1mb_chat_fixture_builds_expected_safe_shape(self) -> None:
        fixture = {
            "name": "gt1mb",
            "model_group": "high",
            "surface": "openai_chat",
            "stream": True,
            "message_count": 6,
            "tool_count": 11,
            "request_bytes_bucket": "gt-1mb",
            "tool_schema_bytes_bucket": "gt-1mb",
        }

        endpoint, payload = smoke.make_payload(fixture, {}, "high")
        raw = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        tool_raw = json.dumps(payload["tools"], separators=(",", ":")).encode("utf-8")

        self.assertEqual(endpoint, "/v1/chat/completions")
        self.assertEqual(payload["model"], "high")
        self.assertEqual(payload["tool_choice"], "auto")
        self.assertEqual(len(payload["messages"]), 6)
        self.assertEqual(len(payload["tools"]), 11)
        self.assertGreater(len(raw), 1024 * 1024)
        self.assertGreater(len(tool_raw), 1024 * 1024)
        self.assertNotIn(os.environ[self.token_env], raw.decode("utf-8"))


if __name__ == "__main__":
    unittest.main()
