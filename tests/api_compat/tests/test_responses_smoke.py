# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.oracles import assert_responses_terminal_usage


def body(reply):
    return json.loads(reply[2])


def test_responses_tool_and_sse_terminal_usage(api, router):
    payload = {
        "model": "responses",
        "input": PROMPT_CANARY,
        "tools": [
            {
                "type": "function",
                "name": "lookup",
                "description": TOOL_CANARY,
                "parameters": {"type": "object"},
            }
        ],
        "tool_choice": "auto",
    }
    status, _, raw = api("/v1/responses", payload)
    assert status == 200
    assert body((status, None, raw))["output"][0]["type"] == "function_call"
    payload.pop("tools")
    payload["stream"] = True
    status, headers, raw = api("/v1/responses", payload)
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_responses_terminal_usage(raw)
    assert router["upstream"].calls[-1]["path"] == "/v1/responses"
    assert router["upstream"].calls[0]["body"]["tool_choice"] == "auto"
