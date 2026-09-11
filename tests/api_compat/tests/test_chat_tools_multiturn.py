# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""CHAT-02/03/05: multi-turn tools and include_usage empty-choices HTTP."""

from __future__ import annotations

import json

from conftest import PROMPT_CANARY, TOOL_CANARY
from harness.oracles import assert_chat_terminal_usage
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

ECHO_TOOL = {
    "type": "function",
    "function": {
        "name": "echo",
        "description": "echo",
        "parameters": {
            "type": "object",
            "properties": {"text": {"type": "string"}},
            "required": ["text"],
        },
    },
}


def _body(raw: bytes) -> dict:
    return json.loads(raw)


def test_chat_full_tool_loop_preserves_call_ids(api, router):
    """CHAT-02: request → tool_calls → tool result → final, plus sequential second call."""
    scenario = ScriptedScenario()
    call_a = "call_lookup_a"
    call_b = "call_lookup_b"
    args_a = '{"q":"alpha"}'
    args_b = '{"q":"beta"}'

    def validate_first(call: dict) -> None:
        body = call["body"]
        assert body["tools"][0]["function"]["name"] == "lookup"
        assert body["messages"][0]["content"] == PROMPT_CANARY
        assert "stream" not in body or body["stream"] is False

    def validate_second(call: dict) -> None:
        messages = call["body"]["messages"]
        assert messages[-3]["role"] == "assistant"
        assert messages[-3]["content"] == "checking"
        tool_calls = messages[-3]["tool_calls"]
        assert [c["id"] for c in tool_calls] == [call_a]
        assert tool_calls[0]["function"]["arguments"] == args_a
        assert messages[-2]["role"] == "tool"
        assert messages[-2]["tool_call_id"] == call_a
        assert messages[-2]["content"] == '{"value":"alpha-result"}'
        assert messages[-1]["role"] == "user"
        assert messages[-1]["content"] == "continue"

    def validate_third(call: dict) -> None:
        messages = call["body"]["messages"]
        assert messages[-2]["role"] == "assistant"
        assert messages[-2]["tool_calls"][0]["id"] == call_b
        assert messages[-2]["tool_calls"][0]["function"]["arguments"] == args_b
        assert messages[-1]["tool_call_id"] == call_b
        assert messages[-1]["content"] == '{"value":"beta-result"}'

    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_first,
        response={
            "id": "chatcmpl_loop1",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": "checking",
                        "tool_calls": [
                            {
                                "id": call_a,
                                "type": "function",
                                "function": {"name": "lookup", "arguments": args_a},
                            }
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
            "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
        },
    )
    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_second,
        response={
            "id": "chatcmpl_loop2",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": call_b,
                                "type": "function",
                                "function": {"name": "lookup", "arguments": args_b},
                            }
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
            "usage": {"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
        },
    )
    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_third,
        response={
            "id": "chatcmpl_loop3",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "final-alpha-beta"},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
        },
    )
    FakeUpstream.scenario = scenario

    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "tools": [LOOKUP_TOOL],
            "tool_choice": "auto",
        },
    )
    assert status == 200
    first = _body(raw)["choices"][0]["message"]
    assert first["content"] == "checking"
    assert first["tool_calls"][0]["id"] == call_a
    assert first["tool_calls"][0]["function"]["arguments"] == args_a

    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [
                {"role": "user", "content": PROMPT_CANARY},
                first,
                {
                    "role": "tool",
                    "tool_call_id": call_a,
                    "content": '{"value":"alpha-result"}',
                },
                {"role": "user", "content": "continue"},
            ],
            "tools": [LOOKUP_TOOL],
        },
    )
    assert status == 200
    second = _body(raw)["choices"][0]["message"]
    assert second["tool_calls"][0]["id"] == call_b

    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [
                {"role": "user", "content": PROMPT_CANARY},
                first,
                {
                    "role": "tool",
                    "tool_call_id": call_a,
                    "content": '{"value":"alpha-result"}',
                },
                {"role": "user", "content": "continue"},
                second,
                {
                    "role": "tool",
                    "tool_call_id": call_b,
                    "content": '{"value":"beta-result"}',
                },
            ],
            "tools": [LOOKUP_TOOL],
        },
    )
    assert status == 200
    assert _body(raw)["choices"][0]["message"]["content"] == "final-alpha-beta"
    scenario.assert_complete()


def test_chat_parallel_tool_calls_match_by_id(api, router):
    """CHAT-03: parallel same-name calls; results returned out of order."""
    scenario = ScriptedScenario()
    call_west = "call_lookup_west"
    call_east = "call_lookup_east"
    args_west = '{"q":"west"}'
    args_east = '{"q":"east"}'

    def validate_parallel_request(call: dict) -> None:
        assert call["body"].get("parallel_tool_calls") is True

    def validate_out_of_order_results(call: dict) -> None:
        messages = call["body"]["messages"]
        assistant = messages[-3]
        tool_msgs = messages[-2:]
        assert [c["id"] for c in assistant["tool_calls"]] == [call_west, call_east]
        # Results intentionally reversed relative to call order.
        assert [m["tool_call_id"] for m in tool_msgs] == [call_east, call_west]
        by_id = {m["tool_call_id"]: m["content"] for m in tool_msgs}
        assert by_id[call_west] == '{"side":"west"}'
        assert by_id[call_east] == '{"side":"east"}'
        # Executor uniqueness: each id appears once.
        assert len(by_id) == 2

    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_parallel_request,
        response={
            "id": "chatcmpl_parallel",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": call_west,
                                "type": "function",
                                "function": {"name": "lookup", "arguments": args_west},
                            },
                            {
                                "id": call_east,
                                "type": "function",
                                "function": {"name": "lookup", "arguments": args_east},
                            },
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
            "usage": {"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
        },
    )
    scenario.expect(
        path_suffix="/chat/completions",
        validate=validate_out_of_order_results,
        response={
            "id": "chatcmpl_parallel_final",
            "object": "chat.completion",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "west-and-east"},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
        },
    )
    FakeUpstream.scenario = scenario

    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "tools": [LOOKUP_TOOL, ECHO_TOOL],
            "tool_choice": "auto",
            "parallel_tool_calls": True,
        },
    )
    assert status == 200
    assistant = _body(raw)["choices"][0]["message"]
    assert [c["id"] for c in assistant["tool_calls"]] == [call_west, call_east]

    # Simulate executor matching by id (not name/order): return east then west.
    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [
                {"role": "user", "content": PROMPT_CANARY},
                assistant,
                {
                    "role": "tool",
                    "tool_call_id": call_east,
                    "content": '{"side":"east"}',
                },
                {
                    "role": "tool",
                    "tool_call_id": call_west,
                    "content": '{"side":"west"}',
                },
            ],
            "tools": [LOOKUP_TOOL, ECHO_TOOL],
        },
    )
    assert status == 200
    assert _body(raw)["choices"][0]["message"]["content"] == "west-and-east"
    scenario.assert_complete()


def test_chat_include_usage_empty_choices_http(api, router):
    """CHAT-05: stream_options.include_usage → empty-choices usage chunk over HTTP."""
    FakeUpstream.chat_include_usage_empty_choices = True

    status, headers, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "stream": True,
            "stream_options": {"include_usage": True},
        },
    )
    assert status == 200
    assert "text/event-stream" in headers["Content-Type"]
    upstream_body = FakeUpstream.calls[0]["body"]
    assert upstream_body.get("stream") is True
    assert upstream_body.get("stream_options", {}).get("include_usage") is True
    assert b'"choices":[]' in raw
    assert_chat_terminal_usage(raw)

    # Omitted / false still settle via accepted legacy provider variant.
    FakeUpstream.reset_state()
    FakeUpstream.chat_include_usage_empty_choices = False
    status, headers, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
            "stream": True,
            "stream_options": {"include_usage": False},
        },
    )
    assert status == 200
    assert FakeUpstream.calls[0]["body"].get("stream_options", {}).get("include_usage") is False
    assert_chat_terminal_usage(raw)
