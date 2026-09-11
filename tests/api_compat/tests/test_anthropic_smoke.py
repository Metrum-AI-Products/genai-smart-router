# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.oracles import assert_messages_terminal_usage


def body(reply):
    return json.loads(reply[2])


def test_anthropic_messages_tool_choice_and_sse_usage(api, router):
    status, _, raw = api(
        "/anthropic/v1/messages",
        {
            "model": "messages",
            "max_tokens": 32,
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "tools": [
                {
                    "name": "lookup",
                    "description": TOOL_CANARY,
                    "input_schema": {"type": "object"},
                }
            ],
            "tool_choice": {"type": "tool", "name": "lookup"},
        },
    )
    assert status == 200
    message = body((status, None, raw))
    assert message["type"] == "message"
    assert message["stop_reason"] == "tool_use"
    assert message["content"] == [
        {"type": "tool_use", "id": "toolu_synthetic", "name": "lookup", "input": {}}
    ]
    assert message["usage"] == {"input_tokens": 3, "output_tokens": 2}
    assert router["upstream"].calls[0]["path"] == "/anthropic/v1/messages"
    assert router["upstream"].calls[0]["body"]["tool_choice"]["name"] == "lookup"
    status, headers, raw = api(
        "/anthropic/v1/messages",
        {
            "model": "messages",
            "max_tokens": 32,
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "stream": True,
        },
    )
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_messages_terminal_usage(raw)
