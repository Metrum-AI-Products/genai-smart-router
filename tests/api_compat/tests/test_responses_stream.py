# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""RESP-02..06: native Responses streaming contracts."""

from __future__ import annotations

import json
import threading
from urllib.request import Request, urlopen

from conftest import CALLER, PROMPT_CANARY
from harness.oracles import assert_responses_terminal_usage, responses_completed_frames_without_done
from harness.scripted_upstream import (
    FakeUpstream,
    ScriptedScenario,
    default_responses_stream_frames,
)
from harness.sse import sse_events


def _frame(event: str, payload: dict) -> str:
    return f"event: {event}\ndata: {json.dumps(payload, separators=(',', ':'))}\n\n"


def _stream_open(base: str, path: str, payload: dict):
    data = json.dumps(payload).encode()
    req = Request(
        base + path,
        data=data,
        headers={
            "Authorization": f"Bearer {CALLER}",
            "Content-Type": "application/json",
        },
    )
    return urlopen(req, timeout=10)


def test_responses_native_stream_gated_first_event(router):
    """RESP-02: first text event arrives before gate release (native, not unary synthesize)."""
    gate = threading.Event()
    frames = default_responses_stream_frames()
    delta_index = next(i for i, f in enumerate(frames) if "response.output_text.delta" in f)

    scenario = ScriptedScenario()

    def validate(call: dict) -> None:
        assert call["body"]["stream"] is True
        assert call["body"]["input"] == PROMPT_CANARY

    scenario.expect(
        path_suffix="/responses",
        validate=validate,
        response=frames,
        gate=gate,
        gate_after_frames=delta_index + 1,
    )
    FakeUpstream.scenario = scenario

    resp = _stream_open(
        router["base"],
        "/v1/responses",
        {"model": "responses", "stream": True, "input": PROMPT_CANARY},
    )
    assert resp.status == 200
    assert "text/event-stream" in resp.headers.get("Content-Type", "")

    buf = bytearray()
    saw_delta = False
    while not saw_delta:
        piece = resp.read(64)
        if not piece:
            break
        buf.extend(piece)
        if b"response.output_text.delta" in buf:
            saw_delta = True
            assert not gate.is_set(), "first event arrived after gate release (not native-gated)"
            gate.set()

    assert saw_delta, f"missing first delta before gate release; got {bytes(buf)!r}"

    while True:
        piece = resp.read(4096)
        if not piece:
            break
        buf.extend(piece)
    resp.close()

    assert_responses_terminal_usage(bytes(buf))
    assert router["upstream"].calls[-1]["body"]["stream"] is True
    scenario.assert_complete()


def test_responses_lifecycle_events_forwarded(api, router):
    """RESP-03: created → … → completed lifecycle with stable ids."""
    status, headers, raw = api(
        "/v1/responses",
        {"model": "responses", "stream": True, "input": PROMPT_CANARY},
    )
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    events = sse_events(raw)
    names = [name for name, _ in events]
    for required in (
        "response.created",
        "response.in_progress",
        "response.output_item.added",
        "response.content_part.added",
        "response.output_text.delta",
        "response.output_text.done",
        "response.content_part.done",
        "response.output_item.done",
        "response.completed",
    ):
        assert required in names, f"missing {required} in {names}"
    assert names.index("response.created") < names.index("response.output_text.delta")
    assert names.index("response.output_text.delta") < names.index("response.completed")
    created = next(p for n, p in events if n == "response.created")
    completed = next(p for n, p in events if n == "response.completed")
    assert created["response"]["id"] == completed["response"]["id"] == "resp_synthetic"
    assert router["upstream"].calls[-1]["body"]["stream"] is True


def test_responses_function_call_argument_deltas(api, router):
    """RESP-04: function argument fragments reassemble; id ≠ call_id."""
    frames = [
        _frame(
            "response.output_item.added",
            {
                "type": "response.output_item.added",
                "output_index": 0,
                "item": {
                    "id": "fc_item",
                    "type": "function_call",
                    "status": "in_progress",
                    "call_id": "call_abc",
                    "name": "lookup",
                    "arguments": "",
                },
            },
        ),
        _frame(
            "response.function_call_arguments.delta",
            {
                "type": "response.function_call_arguments.delta",
                "item_id": "fc_item",
                "output_index": 0,
                "delta": '{"q":',
            },
        ),
        _frame(
            "response.function_call_arguments.delta",
            {
                "type": "response.function_call_arguments.delta",
                "item_id": "fc_item",
                "output_index": 0,
                "delta": '"x"}',
            },
        ),
        _frame(
            "response.function_call_arguments.done",
            {
                "type": "response.function_call_arguments.done",
                "item_id": "fc_item",
                "output_index": 0,
                "arguments": '{"q":"x"}',
            },
        ),
        _frame(
            "response.output_item.done",
            {
                "type": "response.output_item.done",
                "output_index": 0,
                "item": {
                    "id": "fc_item",
                    "type": "function_call",
                    "status": "completed",
                    "call_id": "call_abc",
                    "name": "lookup",
                    "arguments": '{"q":"x"}',
                },
            },
        ),
        _frame(
            "response.completed",
            {
                "type": "response.completed",
                "response": {
                    "id": "resp_tool",
                    "object": "response",
                    "status": "completed",
                    "usage": {"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
                },
            },
        ),
    ]
    scenario = ScriptedScenario()
    scenario.expect(path_suffix="/responses", response=frames)
    FakeUpstream.scenario = scenario

    status, _, raw = api(
        "/v1/responses",
        {"model": "responses", "stream": True, "input": PROMPT_CANARY},
    )
    assert status == 200
    events = sse_events(raw)
    deltas = [
        p["delta"]
        for n, p in events
        if n == "response.function_call_arguments.delta" and isinstance(p, dict)
    ]
    assert "".join(deltas) == '{"q":"x"}'
    done = next(p for n, p in events if n == "response.function_call_arguments.done")
    assert done["arguments"] == '{"q":"x"}'
    item = next(p for n, p in events if n == "response.output_item.done")["item"]
    assert item["id"] == "fc_item"
    assert item["call_id"] == "call_abc"
    assert item["id"] != item["call_id"]
    scenario.assert_complete()


def test_responses_failed_and_incomplete_terminals(api, router):
    """RESP-05: failed/incomplete terminals are not rewritten to completed."""

    def run(terminal: str, status_value: str) -> None:
        frames = [
            _frame(
                "response.output_text.delta",
                {"type": "response.output_text.delta", "delta": "partial"},
            ),
            _frame(
                terminal,
                {
                    "type": terminal,
                    "response": {
                        "id": "resp_x",
                        "object": "response",
                        "status": status_value,
                        "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
                    },
                },
            ),
        ]
        scenario = ScriptedScenario()
        scenario.expect(path_suffix="/responses", response=frames)
        FakeUpstream.scenario = scenario
        status, _, raw = api(
            "/v1/responses",
            {"model": "responses", "stream": True, "input": PROMPT_CANARY},
        )
        assert status == 200
        events = sse_events(raw)
        names = [n for n, _ in events]
        assert terminal in names
        assert "response.completed" not in names
        payload = next(p for n, p in events if n == terminal)
        assert payload["response"]["status"] == status_value
        scenario.assert_complete()

    run("response.failed", "failed")
    FakeUpstream.reset_state()
    run("response.incomplete", "incomplete")


def test_responses_terminal_without_done_sentinel(api, router):
    """RESP-06: response.completed without [DONE] is a valid terminal lifecycle."""
    frames = responses_completed_frames_without_done()
    assert not any("[DONE]" in f for f in frames)
    scenario = ScriptedScenario()
    scenario.expect(path_suffix="/responses", response=frames)
    FakeUpstream.scenario = scenario

    status, headers, raw = api(
        "/v1/responses",
        {"model": "responses", "stream": True, "input": PROMPT_CANARY},
    )
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert b"[DONE]" not in raw
    assert_responses_terminal_usage(raw, require_done=False)
    assert router["upstream"].calls[-1]["body"]["stream"] is True
    scenario.assert_complete()
