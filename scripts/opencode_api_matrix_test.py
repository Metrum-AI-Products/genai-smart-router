#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Tests for opencode_api_matrix.py payload construction."""

from __future__ import annotations

import unittest

import opencode_api_matrix as matrix


class OpencodeApiMatrixTest(unittest.TestCase):
    def test_openai_tool_payload_matches_opencode_shape(self) -> None:
        payload = matrix.build_payload(
            "openai-chat",
            "accounts/fireworks/models/deepseek-v4-flash",
            "tools",
            matrix.DEFAULT_IMAGE_URL,
            64,
        )

        self.assertEqual(payload["model"], "accounts/fireworks/models/deepseek-v4-flash")
        self.assertEqual(payload["tool_choice"], "auto")
        self.assertTrue(payload["parallel_tool_calls"])
        self.assertEqual(payload["tools"][0]["type"], "function")
        self.assertEqual(payload["tools"][0]["function"]["name"], "read_workspace_file")
        self.assertGreaterEqual(payload["max_tokens"], 128)

    def test_anthropic_image_payload_matches_messages_shape(self) -> None:
        payload = matrix.build_payload(
            "anthropic",
            "example-model",
            "image",
            matrix.DEFAULT_IMAGE_URL,
            64,
        )

        content = payload["messages"][0]["content"]
        self.assertEqual(content[1]["type"], "image")
        self.assertEqual(content[1]["source"]["type"], "url")
        self.assertEqual(content[1]["source"]["url"], matrix.DEFAULT_IMAGE_URL)
        self.assertEqual(matrix.count_images(payload), 1)
        self.assertGreaterEqual(payload["max_tokens"], 512)

    def test_dry_run_records_probe_metadata_without_network(self) -> None:
        result = matrix.run_probe(
            base_url="https://api.fireworks.ai/inference/v1",
            api_key="unused",
            dialect="openai-chat",
            model="accounts/fireworks/models/deepseek-v4-flash",
            task="image",
            image_url=matrix.DEFAULT_IMAGE_URL,
            max_tokens=64,
            expected_text="OK",
            expected_image_text="Rite Aid",
            timeout=1,
            user_agent="test",
            dry_run=True,
        )

        self.assertEqual(result.status, "dry-run")
        self.assertEqual(result.http_status, 0)
        self.assertEqual(result.tool_count, 0)
        self.assertEqual(result.image_count, 1)
        self.assertIn("/chat/completions", result.endpoint)
        self.assertGreater(result.request_bytes, 0)

    def test_image_rows_require_expected_receipt_text(self) -> None:
        openai_status, _, _, openai_notes = matrix.classify_success(
            "openai-chat",
            "image",
            {"choices": [{"message": {"content": "I cannot view images."}, "finish_reason": "stop"}]},
            "OK",
            "Rite Aid",
        )
        anthropic_status, _, _, anthropic_notes = matrix.classify_success(
            "anthropic",
            "image",
            {"content": [{"type": "text", "text": "The merchant is Rite Aid."}], "stop_reason": "end_turn"},
            "OK",
            "Rite Aid",
        )

        self.assertEqual(openai_status, "fail")
        self.assertIn("expected receipt text", openai_notes)
        self.assertEqual(anthropic_status, "pass")
        self.assertEqual(anthropic_notes, "")

    def test_text_rows_require_expected_text(self) -> None:
        openai_status, _, _, openai_notes = matrix.classify_success(
            "openai-chat",
            "text",
            {"choices": [{"message": {"content": ""}, "finish_reason": "length"}]},
            "OK",
            "Rite Aid",
        )
        anthropic_status, _, _, anthropic_notes = matrix.classify_success(
            "anthropic",
            "text",
            {"content": [{"type": "text", "text": "OK"}], "stop_reason": "end_turn"},
            "OK",
            "Rite Aid",
        )

        self.assertEqual(openai_status, "fail")
        self.assertIn("expected text", openai_notes)
        self.assertEqual(anthropic_status, "pass")
        self.assertEqual(anthropic_notes, "")


if __name__ == "__main__":
    unittest.main()
