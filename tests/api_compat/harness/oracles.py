# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Streaming oracles for Chat, Responses, and Messages SSE transcripts."""

from __future__ import annotations

from typing import Any

from .sse import sse_events


EXPECTED_CHAT_USAGE = {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}
EXPECTED_RESPONSES_USAGE = {"input_tokens": 3, "output_tokens": 2, "total_tokens": 5}
EXPECTED_MESSAGES_OUTPUT_USAGE = {"output_tokens": 2}


def assert_chat_terminal_usage(
    raw: bytes | str,
    *,
    expected_content: str = "synthetic chat",
    expected_usage: dict[str, int] | None = None,
) -> None:
    """Accept legacy finish+usage-on-choices or official include_usage empty-choices.

    Official include_usage shape:
      1) content chunk
      2) finish_reason chunk (usage optional/null)
      3) usage-only chunk with choices: []
      4) optional [DONE]

    Legacy shape places usage on the nonempty finish chunk, then [DONE].
    Parsers must not index empty choices.
    """
    usage = expected_usage or EXPECTED_CHAT_USAGE
    events = sse_events(raw)
    assert events, "chat SSE produced no events"
    payloads: list[dict[str, Any]] = []
    saw_done = False
    for name, payload in events:
        if payload == "[DONE]":
            saw_done = True
            continue
        assert isinstance(payload, dict), f"unexpected chat payload type: {type(payload)}"
        payloads.append(payload)

    assert any(
        _chat_delta_content(chunk) == expected_content for chunk in payloads
    ), "missing chat content delta"

    usage_chunk = None
    finish_chunk = None
    for chunk in payloads:
        choices = chunk.get("choices")
        if choices == []:
            # Official include_usage usage-only chunk — do not index choices[0].
            assert "usage" in chunk
            usage_chunk = chunk
            continue
        if not isinstance(choices, list) or not choices:
            continue
        choice = choices[0]
        if choice.get("finish_reason") == "stop":
            finish_chunk = chunk
            if chunk.get("usage"):
                usage_chunk = chunk

    assert finish_chunk is not None, "missing chat finish_reason=stop chunk"
    assert usage_chunk is not None, "missing chat usage chunk"
    assert usage_chunk["usage"] == usage
    # [DONE] is tolerated (and common) but not required once usage settles.
    _ = saw_done


def _chat_delta_content(chunk: dict[str, Any]) -> str | None:
    choices = chunk.get("choices")
    if not isinstance(choices, list) or not choices:
        return None
    delta = choices[0].get("delta") or {}
    content = delta.get("content")
    return content if isinstance(content, str) else None


def assert_responses_terminal_usage(
    raw: bytes | str,
    *,
    expected_delta: str = "synthetic responses",
    expected_usage: dict[str, int] | None = None,
    require_done: bool = False,
) -> None:
    """Terminal lifecycle ends at response.completed (or failed/incomplete).

    Do not require Chat's [DONE] sentinel. If present, tolerate it.
    """
    usage = expected_usage or EXPECTED_RESPONSES_USAGE
    events = sse_events(raw)
    assert events, "responses SSE produced no events"

    assert any(
        name == "response.output_text.delta" and isinstance(payload, dict) and payload.get("delta") == expected_delta
        for name, payload in events
    ), "missing response.output_text.delta"

    terminal_names = {"response.completed", "response.failed", "response.incomplete"}
    terminal_index = None
    for index, (name, _) in enumerate(events):
        if name in terminal_names:
            terminal_index = index
    assert terminal_index is not None, "missing responses terminal event"

    name, payload = events[terminal_index]
    assert name == "response.completed", f"expected completed, got {name}"
    assert isinstance(payload, dict)
    completed = payload["response"]
    assert completed["status"] == "completed"
    assert completed["usage"] == usage

    trailing = events[terminal_index + 1 :]
    if require_done:
        assert trailing and trailing[-1][1] == "[DONE]"
    else:
        for _, item in trailing:
            assert item == "[DONE]" or item == "", f"unexpected trailing event after terminal: {item!r}"


def assert_messages_terminal_usage(
    raw: bytes | str,
    *,
    expected_text: str = "synthetic messages",
    expected_usage: dict[str, int] | None = None,
) -> None:
    """Keep existing Anthropic terminal ordering assertions."""
    usage = expected_usage or EXPECTED_MESSAGES_OUTPUT_USAGE
    events = sse_events(raw)
    names = [name for name, _ in events]
    content_index = names.index("content_block_delta")
    usage_index = names.index("message_delta")
    stop_index = names.index("message_stop")
    assert content_index < usage_index < stop_index == len(events) - 1
    assert events[content_index][1]["delta"] == {"type": "text_delta", "text": expected_text}
    assert events[usage_index][1]["usage"] == usage


def chat_include_usage_frames(
    *,
    content: str = "synthetic chat",
    usage: dict[str, int] | None = None,
) -> list[str]:
    """Official Chat stream_options.include_usage frame sequence."""
    usage = usage or EXPECTED_CHAT_USAGE
    return [
        f'data: {{"id":"chatcmpl_synthetic","object":"chat.completion.chunk","choices":[{{"index":0,"delta":{{"role":"assistant","content":{json_dumps(content)}}},"finish_reason":null}}]}}\n\n',
        'data: {"id":"chatcmpl_synthetic","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n',
        f'data: {{"id":"chatcmpl_synthetic","object":"chat.completion.chunk","choices":[],"usage":{json_dumps(usage)}}}\n\n',
        "data: [DONE]\n\n",
    ]


def responses_completed_frames_without_done(
    *,
    delta: str = "synthetic responses",
    usage: dict[str, int] | None = None,
) -> list[str]:
    """Minimal Responses SSE ending at response.completed without [DONE]."""
    usage = usage or EXPECTED_RESPONSES_USAGE
    response = {
        "id": "resp_synthetic",
        "object": "response",
        "status": "completed",
        "usage": usage,
    }
    return [
        f'event: response.output_text.delta\ndata: {{"type":"response.output_text.delta","delta":{json_dumps(delta)}}}\n\n',
        f'event: response.completed\ndata: {{"type":"response.completed","response":{json_dumps(response)}}}\n\n',
    ]


def json_dumps(value: Any) -> str:
    import json

    return json.dumps(value, separators=(",", ":"))
