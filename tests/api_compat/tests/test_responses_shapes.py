# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""RESP-01: string vs item-array input shapes and tool-only output."""

from __future__ import annotations

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.scripted_upstream import FakeUpstream, ScriptedScenario


LOOKUP_TOOL = {
    "type": "function",
    "name": "lookup",
    "description": TOOL_CANARY,
    "parameters": {"type": "object", "properties": {"q": {"type": "string"}}},
}


def _body(raw: bytes) -> dict:
    return json.loads(raw)


def test_responses_string_and_item_array_shapes(api, router):
    """RESP-01: string input, item-array history, instructions, tool-only output."""
    scenario = ScriptedScenario()

    def validate_string(call: dict) -> None:
        body = call["body"]
        assert body["input"] == PROMPT_CANARY
        assert body["instructions"] == "be brief"
        assert body.get("stream") is not True

    def validate_items_passthrough(call: dict) -> None:
        # Tools force same-dialect passthrough, which keeps the item-array wire shape.
        body = call["body"]
        assert isinstance(body["input"], list)
        assert body["input"][0]["role"] == "user"
        assert body["input"][0]["content"][0]["type"] == "input_text"
        assert body["input"][0]["content"][0]["text"] == PROMPT_CANARY
        assert body["instructions"] == "follow tools"
        assert body["tools"][0]["name"] == "lookup"

    def validate_tool_only(call: dict) -> None:
        body = call["body"]
        assert body["tools"][0]["name"] == "lookup"
        assert body["tool_choice"] == "auto"

    scenario.expect(
        path_suffix="/responses",
        validate=validate_string,
        response={
            "id": "resp_shape_text",
            "object": "response",
            "status": "completed",
            "output": [
                {
                    "type": "message",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "ok"}],
                }
            ],
            "usage": {"input_tokens": 3, "output_tokens": 1, "total_tokens": 4},
        },
    )
    scenario.expect(
        path_suffix="/responses",
        validate=validate_items_passthrough,
        response={
            "id": "resp_shape_items",
            "object": "response",
            "status": "completed",
            "output": [
                {
                    "type": "message",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "ok-items"}],
                }
            ],
            "usage": {"input_tokens": 4, "output_tokens": 1, "total_tokens": 5},
        },
    )
    scenario.expect(
        path_suffix="/responses",
        validate=validate_tool_only,
        response={
            "id": "resp_shape_tool",
            "object": "response",
            "status": "completed",
            "output": [
                {
                    "type": "function_call",
                    "id": "fc_1",
                    "call_id": "call_1",
                    "name": "lookup",
                    "arguments": '{"q":"x"}',
                }
            ],
            "usage": {"input_tokens": 5, "output_tokens": 2, "total_tokens": 7},
        },
    )
    FakeUpstream.scenario = scenario

    status, _, raw = api(
        "/v1/responses",
        {"model": "responses", "input": PROMPT_CANARY, "instructions": "be brief"},
    )
    assert status == 200
    assert _body(raw)["output"][0]["type"] == "message"

    status, _, raw = api(
        "/v1/responses",
        {
            "model": "responses",
            "instructions": "follow tools",
            "tools": [LOOKUP_TOOL],
            "input": [
                {
                    "role": "user",
                    "content": [{"type": "input_text", "text": PROMPT_CANARY}],
                }
            ],
        },
    )
    assert status == 200, raw
    assert _body(raw)["output"][0]["content"][0]["text"] == "ok-items"

    status, _, raw = api(
        "/v1/responses",
        {
            "model": "responses",
            "input": PROMPT_CANARY,
            "tools": [LOOKUP_TOOL],
            "tool_choice": "auto",
        },
    )
    assert status == 200
    out = _body(raw)["output"][0]
    assert out["type"] == "function_call"
    assert out["call_id"] == "call_1"
    assert out["name"] == "lookup"
    scenario.assert_complete()
