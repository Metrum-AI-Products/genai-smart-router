# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""CHAT-04: streamed tool id/name and argument fragment reassembly."""

from __future__ import annotations

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.scripted_upstream import FakeUpstream, ScriptedScenario
from harness.sse import sse_events


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

# Arguments intentionally split across a JSON escape boundary (\" after q\":\").
EXPECTED_ARGS = '{"q":"sf\\u2603"}'
ARG_FRAG_A = '{"q":"'
ARG_FRAG_B = 'sf\\u2603"}'


def _accumulate_tool_arguments(raw: bytes) -> tuple[str, str, str]:
    """SDK-style accumulation: id/name from first fragment, args concatenated."""
    call_id = ""
    name = ""
    args = ""
    for _, payload in sse_events(raw):
        if payload == "[DONE]" or not isinstance(payload, dict):
            continue
        choices = payload.get("choices") or []
        if not choices:
            continue
        delta = choices[0].get("delta") or {}
        for tool in delta.get("tool_calls") or []:
            if tool.get("id"):
                call_id = tool["id"]
            fn = tool.get("function") or {}
            if fn.get("name"):
                name = fn["name"]
            if "arguments" in fn and fn["arguments"] is not None:
                # Partial JSON must not be treated as executable yet.
                fragment = fn["arguments"]
                assert isinstance(fragment, str)
                args += fragment
    return call_id, name, args


def test_chat_streamed_tool_argument_fragments(api, router):
    """CHAT-04: id/name in first fragment; args split across escape boundary."""
    scenario = ScriptedScenario()
    call_id = "call_stream_lookup"

    def validate_stream_request(call: dict) -> None:
        body = call["body"]
        assert body.get("stream") is True
        assert body["tools"][0]["function"]["name"] == "lookup"

    frames = [
        (
            "data: "
            + json.dumps(
                {
                    "id": "chatcmpl_stream_tools",
                    "object": "chat.completion.chunk",
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "role": "assistant",
                                "tool_calls": [
                                    {
                                        "index": 0,
                                        "id": call_id,
                                        "type": "function",
                                        "function": {"name": "lookup", "arguments": ""},
                                    }
                                ],
                            },
                            "finish_reason": None,
                        }
                    ],
                },
                separators=(",", ":"),
            )
            + "\n\n"
        ),
        (
            "data: "
            + json.dumps(
                {
                    "id": "chatcmpl_stream_tools",
                    "object": "chat.completion.chunk",
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "tool_calls": [
                                    {
                                        "index": 0,
                                        "function": {"arguments": ARG_FRAG_A},
                                    }
                                ]
                            },
                            "finish_reason": None,
                        }
                    ],
                },
                separators=(",", ":"),
            )
            + "\n\n"
        ),
        (
            "data: "
            + json.dumps(
                {
                    "id": "chatcmpl_stream_tools",
                    "object": "chat.completion.chunk",
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "tool_calls": [
                                    {
                                        "index": 0,
                                        "function": {"arguments": ARG_FRAG_B},
                                    }
                                ]
                            },
                            "finish_reason": None,
                        }
                    ],
                },
                separators=(",", ":"),
            )
            + "\n\n"
        ),
        (
            "data: "
            + json.dumps(
                {
                    "id": "chatcmpl_stream_tools",
                    "object": "chat.completion.chunk",
                    "choices": [{"index": 0, "delta": {}, "finish_reason": "tool_calls"}],
                },
                separators=(",", ":"),
            )
            + "\n\n"
        ),
        "data: [DONE]\n\n",
    ]

    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_stream_request,
        response=frames,
        # Force mid-frame TCP-style splits, including across JSON escape bytes.
        split_sse_bytes=11,
    )
    FakeUpstream.scenario = scenario

    status, headers, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "tools": [LOOKUP_TOOL],
            "tool_choice": "auto",
            "stream": True,
        },
    )
    assert status == 200
    assert "text/event-stream" in headers["Content-Type"]
    got_id, got_name, got_args = _accumulate_tool_arguments(raw)
    assert got_id == call_id
    assert got_name == "lookup"
    assert got_args == EXPECTED_ARGS
    # Only fully reassembled args are valid JSON for an executor.
    assert json.loads(got_args) == {"q": "sf\u2603"}
    scenario.assert_complete()
