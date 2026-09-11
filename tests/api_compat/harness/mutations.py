# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""ARCH-08 mutation helpers: deliberately bad fixtures must fail intended checks."""

from __future__ import annotations

from typing import Any, Callable

from .oracles import assert_chat_terminal_usage, assert_responses_terminal_usage


def drop_tool_id(message: dict[str, Any]) -> dict[str, Any]:
    """Remove tool/function call IDs so ID association checks fail."""
    out = dict(message)
    tool_calls = out.get("tool_calls")
    if isinstance(tool_calls, list):
        cleaned = []
        for call in tool_calls:
            item = dict(call)
            item.pop("id", None)
            cleaned.append(item)
        out["tool_calls"] = cleaned
    content = out.get("content")
    if isinstance(content, list):
        cleaned_content = []
        for block in content:
            item = dict(block)
            if item.get("type") in {"tool_use", "function_call"}:
                item.pop("id", None)
                item.pop("call_id", None)
            cleaned_content.append(item)
        out["content"] = cleaned_content
    return out


def reorder_image_blocks(content: list[Any]) -> list[Any]:
    """Swap the first image block with a later text block to break order checks."""
    blocks = [dict(block) if isinstance(block, dict) else block for block in content]
    image_indexes = [
        index
        for index, block in enumerate(blocks)
        if isinstance(block, dict)
        and block.get("type") in {"image_url", "input_image", "image"}
    ]
    text_indexes = [
        index
        for index, block in enumerate(blocks)
        if isinstance(block, dict) and block.get("type") in {"text", "input_text"}
    ]
    if not image_indexes or not text_indexes:
        raise ValueError("reorder_image_blocks requires at least one image and one text block")
    i, j = image_indexes[0], text_indexes[-1]
    if i == j:
        raise ValueError("image and text indexes collided")
    blocks[i], blocks[j] = blocks[j], blocks[i]
    return blocks


def duplicate_argument_fragment(arguments: str, fragment: str = '"}') -> str:
    """Duplicate a JSON fragment so argument reconstruction checks fail."""
    return arguments + fragment


def wrong_finish_reason(chunk: dict[str, Any], reason: str = "length") -> dict[str, Any]:
    out = dict(chunk)
    choices = list(out.get("choices") or [])
    if not choices:
        raise ValueError("wrong_finish_reason requires nonempty choices")
    choice = dict(choices[0])
    choice["finish_reason"] = reason
    out["choices"] = [choice] + list(choices[1:])
    return out


def strip_terminal_event(raw: bytes | str, terminal: str) -> bytes:
    """Remove the named SSE event so terminal oracles fail."""
    text = raw.decode() if isinstance(raw, (bytes, bytearray)) else raw
    text = text.replace("\r\n", "\n")
    kept: list[str] = []
    for frame in text.split("\n\n"):
        if not frame.strip():
            continue
        if terminal == "[DONE]" and "data: [DONE]" in frame:
            continue
        if terminal == "response.completed" and "response.completed" in frame:
            continue
        if terminal == "message_stop" and "message_stop" in frame:
            continue
        kept.append(frame)
    return ("\n\n".join(kept) + ("\n\n" if kept else "")).encode()


def assert_tool_ids_present(message: dict[str, Any]) -> None:
    tool_calls = message.get("tool_calls") or []
    assert tool_calls, "expected tool_calls"
    for call in tool_calls:
        assert call.get("id"), "tool call missing id"
    content = message.get("content")
    if isinstance(content, list):
        for block in content:
            if isinstance(block, dict) and block.get("type") in {"tool_use", "function_call"}:
                assert block.get("id") or block.get("call_id"), "tool block missing id"


def assert_image_before_text(content: list[Any]) -> None:
    types = [block.get("type") for block in content if isinstance(block, dict)]
    image_pos = next(
        (i for i, t in enumerate(types) if t in {"image_url", "input_image", "image"}),
        None,
    )
    text_pos = next((i for i, t in enumerate(types) if t in {"text", "input_text"}), None)
    assert image_pos is not None and text_pos is not None
    assert image_pos < text_pos, "expected image block before text block"


def expect_assertion_failure(fn: Callable[[], None]) -> None:
    try:
        fn()
    except AssertionError:
        return
    raise AssertionError("intended mutation check unexpectedly passed")


def run_mutation_self_checks() -> None:
    """ARCH-08 skeleton: at least three mutations must make their checks fail."""
    # 1) Dropped tool ID
    good_message = {
        "role": "assistant",
        "tool_calls": [
            {
                "id": "call_synthetic",
                "type": "function",
                "function": {"name": "lookup", "arguments": "{}"},
            }
        ],
    }
    assert_tool_ids_present(good_message)
    expect_assertion_failure(lambda: assert_tool_ids_present(drop_tool_id(good_message)))

    # 2) Reordered image vs text
    content = [
        {"type": "image_url", "image_url": {"url": "data:image/png;base64,aaa"}},
        {"type": "text", "text": "describe"},
    ]
    assert_image_before_text(content)
    expect_assertion_failure(lambda: assert_image_before_text(reorder_image_blocks(content)))

    # 3) Missing Responses terminal
    good_responses = (
        b'event: response.output_text.delta\ndata: {"type":"response.output_text.delta","delta":"synthetic responses"}\n\n'
        b'event: response.completed\ndata: {"type":"response.completed","response":{"id":"resp_synthetic","object":"response","status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}\n\n'
    )
    assert_responses_terminal_usage(good_responses)
    mutated = strip_terminal_event(good_responses, "response.completed")
    expect_assertion_failure(lambda: assert_responses_terminal_usage(mutated))

    # 4) Wrong chat finish reason fails oracle
    bad_chat = (
        b'data: {"id":"chatcmpl_synthetic","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"synthetic chat"},"finish_reason":null}]}\n\n'
        b'data: {"id":"chatcmpl_synthetic","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}\n\n'
        b"data: [DONE]\n\n"
    )
    expect_assertion_failure(lambda: assert_chat_terminal_usage(bad_chat))
