# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.oracles import assert_chat_terminal_usage


def body(reply):
    return json.loads(reply[2])


def test_chat_tools_tool_choice_and_sse_usage(api, router):
    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "tools": [
                {
                    "type": "function",
                    "function": {
                        "name": "lookup",
                        "description": TOOL_CANARY,
                        "parameters": {"type": "object"},
                    },
                }
            ],
            "tool_choice": "auto",
        },
    )
    assert status == 200
    assert body((status, None, raw))["choices"][0]["message"]["tool_calls"][0]["function"]["name"] == "lookup"
    assert router["upstream"].calls[0]["path"] == "/v1/chat/completions"
    assert router["upstream"].calls[0]["body"]["tool_choice"] == "auto"
    status, headers, raw = api(
        "/v1/chat/completions",
        {"model": "chat", "messages": [{"role": "user", "content": PROMPT_CANARY}], "stream": True},
    )
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_chat_terminal_usage(raw)
