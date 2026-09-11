# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""CHAT-01: roles, content parts, and tool message shapes."""

from __future__ import annotations

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.scripted_upstream import FakeUpstream, ScriptedScenario


LOOKUP_TOOL = {
    "type": "function",
    "function": {
        "name": "lookup",
        "description": TOOL_CANARY,
        "parameters": {
            "type": "object",
            "properties": {"q": {"type": "string"}},
            "required": ["q"],
        },
    },
}

TOOL_ARGS = '{"q":"sf"}'


def _body(raw: bytes) -> dict:
    return json.loads(raw)


def test_chat_roles_content_parts_and_tool_message_shapes(api, router):
    """CHAT-01: role order, string vs parts, empty/null assistant+tool_calls."""
    scenario = ScriptedScenario()

    def validate_turn_one(call: dict) -> None:
        messages = call["body"]["messages"]
        assert [m["role"] for m in messages] == [
            "system",
            "developer",
            "user",
            "user",
            "assistant",
            "assistant",
            "tool",
        ]
        assert messages[0]["content"] == "system-rules"
        assert messages[1]["content"] == "developer-rules"
        assert messages[2]["content"] == PROMPT_CANARY
        assert messages[3]["content"] == [
            {"type": "text", "text": "parts-" + PROMPT_CANARY},
        ]
        assert messages[4]["content"] is None
        assert messages[4]["tool_calls"][0]["id"] == "call_null_content"
        assert messages[5]["content"] == ""
        assert messages[5]["tool_calls"][0]["id"] == "call_empty_content"
        assert messages[6]["role"] == "tool"
        assert messages[6]["tool_call_id"] == "call_null_content"
        assert messages[6]["content"] == '{"ok":true}'

    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_turn_one,
        response={
            "id": "chatcmpl_roles",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "roles-ok"},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
        },
    )
    FakeUpstream.scenario = scenario

    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [
                {"role": "system", "content": "system-rules"},
                {"role": "developer", "content": "developer-rules"},
                {"role": "user", "content": PROMPT_CANARY},
                {
                    "role": "user",
                    "content": [{"type": "text", "text": "parts-" + PROMPT_CANARY}],
                },
                {
                    "role": "assistant",
                    "content": None,
                    "tool_calls": [
                        {
                            "id": "call_null_content",
                            "type": "function",
                            "function": {"name": "lookup", "arguments": TOOL_ARGS},
                        }
                    ],
                },
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [
                        {
                            "id": "call_empty_content",
                            "type": "function",
                            "function": {"name": "lookup", "arguments": TOOL_ARGS},
                        }
                    ],
                },
                {
                    "role": "tool",
                    "tool_call_id": "call_null_content",
                    "content": '{"ok":true}',
                },
            ],
            "tools": [LOOKUP_TOOL],
        },
    )
    assert status == 200
    assert _body(raw)["choices"][0]["message"]["content"] == "roles-ok"
    scenario.assert_complete()
