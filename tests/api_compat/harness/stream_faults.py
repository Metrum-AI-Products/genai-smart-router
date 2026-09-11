# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Stream fault-injection helpers for STREAM catalog cases (issue #94).

Keeps per-frame gates/signals/truncation off the shared FakeUpstream API so
parallel catalog PRs do not rewrite scripted_upstream.py. Tests opt in via
``stream_fault_writes`` and ``attach_stream_faults``.
"""

from __future__ import annotations

import json
import threading
from contextlib import contextmanager
from typing import Any, Iterator, Sequence
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from .oracles import EXPECTED_CHAT_USAGE, json_dumps
from .scripted_upstream import FakeUpstream, ScriptedScenario, ScriptedUpstreamError
from .sse import iter_sse_incremental, sse_events

FrameGateList = Sequence[threading.Event | None]


def rich_chat_stream_frames(
    *,
    content: str = 'synthetic "café" line\u2014ok',
    usage: dict[str, int] | None = None,
) -> list[str]:
    """Chat SSE with UTF-8, JSON escapes, and CRLF frame separators for STREAM-01."""
    usage = usage or EXPECTED_CHAT_USAGE
    # Use CRLF between fields/frames to exercise delimiter tolerance end-to-end.
    return [
        (
            f'data: {{"id":"chatcmpl_stream01","object":"chat.completion.chunk",'
            f'"choices":[{{"index":0,"delta":{{"role":"assistant","content":{json_dumps(content)}}},'
            f'"finish_reason":null}}]}}\r\n\r\n'
        ),
        (
            'data: {"id":"chatcmpl_stream01","object":"chat.completion.chunk",'
            '"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\r\n\r\n'
        ),
        (
            f'data: {{"id":"chatcmpl_stream01","object":"chat.completion.chunk",'
            f'"choices":[],"usage":{json_dumps(usage)}}}\r\n\r\n'
        ),
        "data: [DONE]\r\n\r\n",
    ]


def sse_semantics_chat_frames(
    *,
    content: str = "sse-field-ok",
    usage: dict[str, int] | None = None,
) -> list[str]:
    """STREAM-02: LF/CRLF, comments, multiline data, optional-space, named events."""
    usage = usage or EXPECTED_CHAT_USAGE
    # Multiline data joins with \\n; keep JSON valid by splitting between tokens.
    content_frame = (
        ": keepalive comment\n"
        "event: message\n"
        'data:{"id":"chatcmpl_stream02","object":"chat.completion.chunk","choices":[{'
        "\n"
        f'data: "index":0,"delta":{{"role":"assistant","content":{json_dumps(content)}}},'
        '"finish_reason":null}]}\n'
        "\n"
    )
    finish_frame = (
        "event:message\r\n"
        "data: "
        '{"id":"chatcmpl_stream02","object":"chat.completion.chunk",'
        '"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\r\n'
        "\r\n"
    )
    usage_frame = (
        f'data: {{"id":"chatcmpl_stream02","object":"chat.completion.chunk",'
        f'"choices":[],"usage":{json_dumps(usage)}}}\n\n'
    )
    # Unsupported additive event must not break subsequent Chat parsing.
    unsupported = 'event: vendor.obfuscation\ndata: {"nonce":"stream02"}\n\n'
    return [unsupported, content_frame, finish_frame, usage_frame, "data:[DONE]\n\n"]


def chat_frames_until_finish(*, content: str, include_done: bool = False) -> list[str]:
    """Content + finish+usage; optional [DONE] for STREAM-06 terminal classes."""
    frames = [
        (
            f'data: {{"id":"chatcmpl_eof","object":"chat.completion.chunk",'
            f'"choices":[{{"index":0,"delta":{{"role":"assistant","content":{json_dumps(content)}}},'
            f'"finish_reason":null}}]}}\n\n'
        ),
        (
            'data: {"id":"chatcmpl_eof","object":"chat.completion.chunk",'
            '"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],'
            f'"usage":{json_dumps(EXPECTED_CHAT_USAGE)}}}\n\n'
        ),
    ]
    if include_done:
        frames.append("data: [DONE]\n\n")
    return frames


def chat_refusal_frames(*, content: str = "partial-before-refusal") -> list[str]:
    """Midstream refusal/content_filter terminal — not a successful stop."""
    return [
        (
            f'data: {{"id":"chatcmpl_refusal","object":"chat.completion.chunk",'
            f'"choices":[{{"index":0,"delta":{{"role":"assistant","content":{json_dumps(content)}}},'
            f'"finish_reason":null}}]}}\n\n'
        ),
        (
            'data: {"id":"chatcmpl_refusal","object":"chat.completion.chunk",'
            '"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}\n\n'
        ),
        "data: [DONE]\n\n",
    ]


def chat_truncated_tool_frames() -> list[str]:
    """Tool id/name plus incomplete argument fragment (no finish_reason)."""
    return [
        (
            "data: "
            + json.dumps(
                {
                    "id": "chatcmpl_trunc_tool",
                    "object": "chat.completion.chunk",
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "role": "assistant",
                                "tool_calls": [
                                    {
                                        "index": 0,
                                        "id": "call_trunc",
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
                    "id": "chatcmpl_trunc_tool",
                    "object": "chat.completion.chunk",
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "tool_calls": [
                                    {"index": 0, "function": {"arguments": '{"q":"par'}}
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
    ]


def responses_error_event_frames() -> list[str]:
    """Top-level Responses error event inside HTTP 200 (no completed terminal)."""
    return [
        (
            "event: response.output_text.delta\n"
            'data: {"type":"response.output_text.delta","delta":"before-error"}\n\n'
        ),
        (
            "event: error\n"
            'data: {"type":"error","error":{"code":"server_error","message":"upstream boom"}}\n\n'
        ),
    ]


def responses_incomplete_frames(*, delta: str = "incomplete-partial") -> list[str]:
    return [
        (
            "event: response.output_text.delta\n"
            f'data: {{"type":"response.output_text.delta","delta":{json_dumps(delta)}}}\n\n'
        ),
        (
            "event: response.incomplete\n"
            "data: "
            '{"type":"response.incomplete","response":{"id":"resp_incomplete",'
            '"object":"response","status":"incomplete",'
            '"incomplete_details":{"reason":"max_output_tokens"},'
            '"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}\n\n'
        ),
    ]


def reconstruct_sse_from_byte_splits(raw: bytes, *, split_bytes: int = 1) -> list[tuple[str, object]]:
    """Reassemble SSE events from fixed-size byte slices (STREAM-01 oracle)."""
    if split_bytes < 1:
        raise ValueError("split_bytes must be >= 1")

    def chunks() -> Iterator[bytes]:
        view = memoryview(raw)
        for start in range(0, len(raw), split_bytes):
            yield bytes(view[start : start + split_bytes])

    frames = list(iter_sse_incremental(chunks()))
    out: list[tuple[str, object]] = []
    for name, data in frames:
        if data == "[DONE]":
            out.append((name, "[DONE]"))
        elif data == "":
            out.append((name, ""))
        else:
            out.append((name, json.loads(data)))
    return out


def attach_stream_faults(
    scenario: ScriptedScenario,
    *,
    frame_gates: FrameGateList | None = None,
    frame_signals: FrameGateList | None = None,
    close_after_frames: int | None = None,
    truncate_bytes: int | None = None,
) -> ScriptedScenario:
    """Annotate the most recent expect() step with stream-fault metadata."""
    if not scenario._steps:  # noqa: SLF001 — test harness helper
        raise ScriptedUpstreamError("attach_stream_faults requires a prior expect()")
    step = scenario._steps[-1]  # noqa: SLF001
    step["frame_gates"] = frame_gates
    step["frame_signals"] = frame_signals
    step["close_after_frames"] = close_after_frames
    step["truncate_bytes"] = truncate_bytes
    return scenario


def _write_bytes_split(handler: FakeUpstream, raw: bytes, *, split_bytes: int | None) -> None:
    if not split_bytes:
        handler.wfile.write(raw)
        handler.wfile.flush()
        return
    view = memoryview(raw)
    for start in range(0, len(raw), split_bytes):
        handler.wfile.write(view[start : start + split_bytes])
        handler.wfile.flush()


def write_sse_frames(
    handler: FakeUpstream,
    frames: list[str],
    *,
    split_bytes: int | None = None,
    frame_gates: FrameGateList | None = None,
    frame_signals: FrameGateList | None = None,
    close_after_frames: int | None = None,
    truncate_bytes: int | None = None,
    status: int = 200,
) -> None:
    """Write SSE frames with optional per-frame barriers (sync Events, not sleeps)."""
    handler.send_response(status)
    handler.send_header("Content-Type", "text/event-stream")
    handler.send_header("Cache-Control", "no-cache")
    incremental = bool(
        split_bytes
        or frame_gates
        or frame_signals
        or close_after_frames is not None
        or truncate_bytes is not None
        or len(frames) > 1
    )
    payload = "".join(frames)
    raw_all = payload.encode()
    if not incremental:
        handler.send_header("Content-Length", str(len(raw_all)))
        handler.end_headers()
        handler.wfile.write(raw_all)
        return
    handler.end_headers()
    written = 0
    for index, frame in enumerate(frames):
        if frame_gates is not None and index < len(frame_gates):
            gate = frame_gates[index]
            if gate is not None and not gate.wait(timeout=30):
                raise ScriptedUpstreamError(f"frame_gates[{index}] timed out")
        if close_after_frames is not None and index >= close_after_frames:
            handler.close_connection = True
            try:
                handler.connection.close()
            except OSError:
                pass
            return
        chunk = frame.encode() if isinstance(frame, str) else bytes(frame)
        if truncate_bytes is not None and written + len(chunk) > int(truncate_bytes):
            remain = int(truncate_bytes) - written
            if remain > 0:
                _write_bytes_split(handler, chunk[:remain], split_bytes=split_bytes)
            handler.close_connection = True
            try:
                handler.connection.close()
            except OSError:
                pass
            return
        _write_bytes_split(handler, chunk, split_bytes=split_bytes)
        written += len(chunk)
        if frame_signals is not None and index < len(frame_signals):
            signal = frame_signals[index]
            if signal is not None:
                signal.set()


@contextmanager
def stream_fault_writes(handler_cls: type[FakeUpstream] = FakeUpstream):
    """Patch scripted SSE serving to honor attach_stream_faults metadata."""
    original = handler_cls._serve_scripted

    def _serve_scripted(self: FakeUpstream, step: dict[str, Any], body: dict[str, Any]) -> None:
        import time

        if step.get("delay_s"):
            time.sleep(float(step["delay_s"]))
        gate = step.get("gate")
        if gate is not None:
            gate.wait(timeout=30)
        if step.get("close_socket") and step.get("response") is None:
            self.close_connection = True
            try:
                self.connection.close()
            except OSError:
                pass
            return

        response = step.get("response")
        if callable(response):
            response = response(body)

        status = int(step.get("status") or 200)
        if response is None:
            self._serve_default(body, status=status)
            return

        if isinstance(response, list):
            write_sse_frames(
                self,
                response,
                split_bytes=step.get("split_sse_bytes"),
                frame_gates=step.get("frame_gates"),
                frame_signals=step.get("frame_signals"),
                close_after_frames=step.get("close_after_frames"),
                truncate_bytes=step.get("truncate_bytes"),
                status=status,
            )
            return
        if isinstance(response, (bytes, bytearray)):
            content_type = step.get("content_type") or "application/json"
            raw = bytes(response)
            truncate = step.get("truncate_bytes")
            if truncate is not None:
                raw = raw[: int(truncate)]
            self._write_raw(raw, content_type=content_type, status=status)
            if step.get("close_socket"):
                self.close_connection = True
                try:
                    self.connection.close()
                except OSError:
                    pass
            return
        if isinstance(response, dict):
            content_type = step.get("content_type") or "application/json"
            raw = json.dumps(response).encode()
            self._write_raw(raw, content_type=content_type, status=status)
            return
        raise ScriptedUpstreamError(f"unsupported scripted response type: {type(response)}")

    handler_cls._serve_scripted = _serve_scripted  # type: ignore[method-assign]
    try:
        yield
    finally:
        handler_cls._serve_scripted = original  # type: ignore[method-assign]


def open_stream(
    base: str,
    path: str,
    payload: dict[str, Any],
    *,
    token: str,
    timeout: float = 10.0,
):
    """Open a streaming HTTP response without reading the body yet."""
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "text/event-stream",
    }
    data = json.dumps(payload).encode()
    return urlopen(Request(base + path, data=data, headers=headers), timeout=timeout)


def read_sse_until(
    response,
    *,
    predicate,
    timeout_s: float = 10.0,
) -> bytes:
    """Read response body incrementally until predicate(raw_so_far) is true."""
    import time

    end = time.monotonic() + timeout_s
    buf = bytearray()
    while time.monotonic() < end:
        chunk = response.read(1)
        if not chunk:
            break
        buf.extend(chunk)
        if predicate(bytes(buf)):
            return bytes(buf)
    raise TimeoutError(f"SSE predicate not met after {timeout_s}s; got {bytes(buf)!r}")


def first_chat_content_event(raw: bytes) -> bool:
    try:
        events = sse_events(raw)
    except json.JSONDecodeError:
        return False
    for _, payload in events:
        if not isinstance(payload, dict):
            continue
        choices = payload.get("choices") or []
        if not choices:
            continue
        delta = choices[0].get("delta") or {}
        if isinstance(delta.get("content"), str):
            return True
    return False


def safe_close(response) -> None:
    try:
        response.close()
    except (OSError, HTTPError, URLError):
        pass
