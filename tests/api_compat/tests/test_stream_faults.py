# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""STREAM P0 fault-injection contracts (issue #94)."""

from __future__ import annotations

import json
import os
import subprocess
import threading
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

import pytest

from conftest import (
    CALLER,
    CALLER_DIGEST,
    PROMPT_CANARY,
    assert_generated_artifacts_redacted,
    request,
    unused_port,
)
from harness.oracles import assert_chat_terminal_usage
from harness.scripted_upstream import FakeUpstream, ScriptedScenario, start_fake_upstream
from harness.sse import sse_events
from harness.stream_faults import (
    attach_stream_faults,
    first_chat_content_event,
    open_stream,
    read_sse_until,
    reconstruct_sse_from_byte_splits,
    rich_chat_stream_frames,
    safe_close,
    stream_fault_writes,
)


TOOL = {
    "type": "function",
    "function": {
        "name": "lookup",
        "description": "stream-fault tool",
        "parameters": {"type": "object", "properties": {"q": {"type": "string"}}},
    },
}


def _chat_stream_payload(*, model: str = "chat", tools: bool = False) -> dict:
    body: dict = {
        "model": model,
        "messages": [{"role": "user", "content": PROMPT_CANARY}],
        "stream": True,
        "stream_options": {"include_usage": True},
    }
    if tools:
        body["tools"] = [TOOL]
        body["tool_choice"] = "auto"
    return body


def test_stream_01_byte_split_reconstruction(api, router):
    """STREAM-01: same transcript reconstructs across every-byte segmentation."""
    frames = rich_chat_stream_frames()
    joined = "".join(frames).encode()

    baseline = reconstruct_sse_from_byte_splits(joined, split_bytes=len(joined) or 1)
    for size in (1, 2, 7, 64):
        assert reconstruct_sse_from_byte_splits(joined, split_bytes=size) == baseline

    scenario = ScriptedScenario()
    scenario.expect(
        path_suffix="/chat/completions",
        response=frames,
        split_sse_bytes=1,
    )
    FakeUpstream.scenario = scenario

    status, headers, raw = api("/v1/chat/completions", _chat_stream_payload())
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_chat_terminal_usage(raw, expected_content='synthetic "café" line\u2014ok')
    scenario.assert_complete()


def test_stream_03_gated_incremental_delivery(api, router):
    """STREAM-03: first event is observable before later frames are released."""
    frames = rich_chat_stream_frames(content="gated-first")
    release_rest = threading.Event()
    first_written = threading.Event()
    # Frame 0 writes immediately; frames 1.. wait on release_rest.
    gates = [None] + [release_rest] * (len(frames) - 1)
    signals = [first_written] + [None] * (len(frames) - 1)

    scenario = ScriptedScenario()
    scenario.expect(path_suffix="/chat/completions", response=frames)
    attach_stream_faults(scenario, frame_gates=gates, frame_signals=signals)
    FakeUpstream.scenario = scenario

    with stream_fault_writes():
        resp = open_stream(router["base"], "/v1/chat/completions", _chat_stream_payload(), token=CALLER)
        try:
            assert first_written.wait(timeout=5), "upstream never signaled first frame"
            partial = read_sse_until(resp, predicate=first_chat_content_event, timeout_s=5)
            events = sse_events(partial)
            assert any(
                isinstance(p, dict)
                and (p.get("choices") or [{}])[0].get("delta", {}).get("content") == "gated-first"
                for _, p in events
            ), partial
            # Later terminal frames must not be present until the barrier is released.
            assert not any(p == "[DONE]" for _, p in events)
            release_rest.set()
            remainder = resp.read()
        finally:
            safe_close(resp)

    full = partial + remainder
    assert_chat_terminal_usage(full, expected_content="gated-first")
    scenario.assert_complete()


def test_stream_04_cancel_mid_stream_then_healthy(api, router):
    """STREAM-04: cancel after first event; healthy follow-up stream still works."""
    frames = rich_chat_stream_frames(content="cancel-me")
    release_rest = threading.Event()
    first_written = threading.Event()
    gates = [None] + [release_rest] * (len(frames) - 1)
    signals = [first_written] + [None] * (len(frames) - 1)

    scenario = ScriptedScenario()
    scenario.expect(path_suffix="/chat/completions", response=frames)
    attach_stream_faults(scenario, frame_gates=gates, frame_signals=signals)
    FakeUpstream.scenario = scenario

    with stream_fault_writes():
        resp = open_stream(router["base"], "/v1/chat/completions", _chat_stream_payload(), token=CALLER)
        try:
            assert first_written.wait(timeout=5)
            partial = read_sse_until(resp, predicate=first_chat_content_event, timeout_s=5)
            assert b"cancel-me" in partial
        finally:
            # Client cancel: drop the body without consuming the remainder.
            safe_close(resp)
            release_rest.set()
            # Allow the upstream writer thread to observe the closed socket.
            time.sleep(0.05)

    # Follow-up healthy request must succeed (reservations/session not wedged).
    healthy = ScriptedScenario()
    healthy.expect(
        path_suffix="/chat/completions",
        response=rich_chat_stream_frames(content="after-cancel"),
    )
    FakeUpstream.scenario = healthy
    status, headers, raw = api("/v1/chat/completions", _chat_stream_payload())
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_chat_terminal_usage(raw, expected_content="after-cancel")
    healthy.assert_complete()


class _FallbackUpstream(FakeUpstream):
    """Isolated class state so STREAM-05 does not share the session FakeUpstream counters."""

    calls: list = []
    scenario: ScriptedScenario | None = None
    chat_include_usage_empty_choices: bool = False
    _attempt_counter: int = 0
    _counter_lock = threading.Lock()


@pytest.fixture
def fallback_router(router, tmp_path_factory):
    """Router with primary+fallback Chat targets sharing one scripted upstream."""
    work = tmp_path_factory.mktemp("stream-fallback")
    _FallbackUpstream.reset_state()
    upstream, thread, upstream_url = start_fake_upstream(_FallbackUpstream)
    port = unused_port()
    config = f'''server:
  listen: "127.0.0.1:{port}"
  default_model_group: chat-fb
  cache: {{enabled: false}}
  usage_db: {{enabled: false}}
  logging: {{path: "{work / 'router.jsonl'}"}}
state_path: "{work / 'state.json'}"
providers:
  primary: {{base_url: "{upstream_url}/v1", dialect: openai-chat}}
  fallback: {{base_url: "{upstream_url}/v1", dialect: openai-chat}}
models:
  chat-fb:
    strategy: static
    targets:
      - {{provider: primary, model: synthetic-primary, tool_support: {{openai_chat: [tools, tool_choice]}}}}
      - {{provider: fallback, model: synthetic-fallback, tool_support: {{openai_chat: [tools, tool_choice]}}}}
callers:
  - id: synthetic-allowed
    token_sha256: {CALLER_DIGEST}
    allow: [chat-fb]
'''
    config_path = work / "config.yaml"
    config_path.write_text(config)
    binary = Path(router["work"]) / "router"
    assert binary.is_file(), "session fixture must have built the router binary"
    build_env = os.environ | {"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local"}
    process = subprocess.Popen(
        [str(binary), "-config", "config.yaml"],
        cwd=work,
        env=build_env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    base = f"http://127.0.0.1:{port}"
    for _ in range(100):
        try:
            if request(base, "/readyz", token=CALLER)[0] == 200:
                break
        except OSError:
            time.sleep(0.05)
    else:
        process.terminate()
        raise RuntimeError(process.stderr.read())
    config_path.unlink()
    try:
        yield {
            "base": base,
            "work": work,
            "upstream": _FallbackUpstream,
            "process": process,
            "server": upstream,
        }
    finally:
        process.terminate()
        process.wait(timeout=5)
        upstream.shutdown()
        thread.join(timeout=5)
        assert_generated_artifacts_redacted({"work": work})


def test_stream_05_pre_commit_fallback_vs_post_commit_no_replay(fallback_router):
    """STREAM-05: 5xx before commitment may fallback; post-commit must not replay."""
    base = fallback_router["base"]
    Upstream = fallback_router["upstream"]

    # --- Pre-commit: primary 503 with empty body → fallback succeeds ---
    Upstream.reset_state()
    pre = ScriptedScenario()
    pre.expect(path_suffix="/chat/completions", status=503, response={"error": "primary-down"})
    pre.expect(
        path_suffix="/chat/completions",
        response=rich_chat_stream_frames(content="fallback-ok"),
    )
    Upstream.scenario = pre

    status, headers, raw = request(base, "/v1/chat/completions", _chat_stream_payload(model="chat-fb"))
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_chat_terminal_usage(raw, expected_content="fallback-ok")
    assert len(Upstream.calls) == 2, f"expected primary+fallback, got {len(Upstream.calls)}"
    pre.assert_complete()

    # --- Post-commit: primary emits a tool chunk then ends; fallback must stay idle ---
    Upstream.reset_state()
    tool_frame = (
        'data: {"id":"chatcmpl_commit","object":"chat.completion.chunk",'
        '"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function",'
        '"function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}\n\n'
    )
    post = ScriptedScenario()
    post.expect(path_suffix="/chat/completions", response=[tool_frame])
    attach_stream_faults(post, close_after_frames=1)
    Upstream.scenario = post

    with stream_fault_writes(Upstream):
        headers = {
            "Authorization": f"Bearer {CALLER}",
            "Content-Type": "application/json",
            "Accept": "text/event-stream",
        }
        data = json.dumps(_chat_stream_payload(model="chat-fb", tools=True)).encode()
        try:
            with urlopen(Request(base + "/v1/chat/completions", data=data, headers=headers), timeout=5) as resp:
                body = resp.read()
                status = resp.status
                content_type = resp.headers.get("Content-Type", "")
        except (HTTPError, URLError, TimeoutError) as exc:
            # Committed streams may surface as incomplete reads; still assert no fallback.
            status = getattr(exc, "code", 200)
            body = getattr(exc, "read", lambda: b"")() if hasattr(exc, "read") else b""
            content_type = "text/event-stream"

    assert status == 200 or b"call_1" in body or b"lookup" in body
    assert "event-stream" in content_type or b"call_1" in body
    assert len(Upstream.calls) == 1, (
        f"post-commit must not fallback/replay; calls={len(Upstream.calls)} body={body!r}"
    )
    assert b"call_1" in body or b"lookup" in body
