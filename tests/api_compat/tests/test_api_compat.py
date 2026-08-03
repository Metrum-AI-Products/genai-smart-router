import json
from conftest import (
    DENIED_CALLER,
    PROMPT_CANARY,
    TOOL_CANARY,
    assert_generated_artifacts_redacted,
)


def body(reply):
    return json.loads(reply[2])


def sse_events(raw):
    events = []
    for frame in raw.decode().replace("\r\n", "\n").split("\n\n"):
        lines = [line[6:] for line in frame.split("\n") if line.startswith("data: ")]
        if lines:
            events.append("\n".join(lines))
    return events


def assert_terminal_usage(raw, terminal_event):
    events = sse_events(raw)
    assert events
    assert any(terminal_event in event for event in events)
    assert any("total_tokens" in event or "input_tokens" in event for event in events)


def test_models_are_caller_filtered(api):
    status, _, raw = api("/v1/models")
    assert status == 200
    ids = {item["id"] for item in json.loads(raw)["data"]}
    assert {"chat", "responses", "messages"} <= ids
    status, _, raw = api("/v1/models", token=DENIED_CALLER)
    assert status == 200
    assert {item["id"] for item in json.loads(raw)["data"]} == {"chat"}


def test_chat_tools_tool_choice_and_sse_usage(api, router):
    status, _, raw = api("/v1/chat/completions", {"model": "chat", "messages": [{"role": "user", "content": PROMPT_CANARY}], "tools": [{"type": "function", "function": {"name": "lookup", "description": TOOL_CANARY, "parameters": {"type": "object"}}}], "tool_choice": "auto"})
    assert status == 200
    assert body((status, None, raw))["choices"][0]["message"]["tool_calls"][0]["function"]["name"] == "lookup"
    assert router["upstream"].calls[0]["path"] == "/v1/chat/completions"
    assert router["upstream"].calls[0]["body"]["tool_choice"] == "auto"
    status, headers, raw = api("/v1/chat/completions", {"model": "chat", "messages": [{"role": "user", "content": PROMPT_CANARY}], "stream": True})
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_terminal_usage(raw, "[DONE]")


def test_responses_tool_and_sse_terminal_usage(api, router):
    payload = {"model": "responses", "input": PROMPT_CANARY, "tools": [{"type": "function", "name": "lookup", "description": TOOL_CANARY, "parameters": {"type": "object"}}], "tool_choice": "auto"}
    status, _, raw = api("/v1/responses", payload)
    assert status == 200
    assert body((status, None, raw))["output"][0]["type"] == "function_call"
    payload.pop("tools")
    payload["stream"] = True
    status, headers, raw = api("/v1/responses", payload)
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_terminal_usage(raw, "response.completed")
    assert router["upstream"].calls[-1]["path"] == "/v1/responses"
    assert router["upstream"].calls[0]["body"]["tool_choice"] == "auto"


def test_anthropic_messages_tool_choice_and_sse_usage(api, router):
    status, _, raw = api("/anthropic/v1/messages", {"model": "messages", "max_tokens": 32, "messages": [{"role": "user", "content": PROMPT_CANARY}], "tools": [{"name": "lookup", "description": TOOL_CANARY, "input_schema": {"type": "object"}}], "tool_choice": {"type": "tool", "name": "lookup"}})
    assert status == 200
    assert body((status, None, raw))["type"] == "message"
    assert router["upstream"].calls[0]["path"] == "/anthropic/v1/messages"
    assert router["upstream"].calls[0]["body"]["tool_choice"]["name"] == "lookup"
    status, headers, raw = api("/anthropic/v1/messages", {"model": "messages", "max_tokens": 32, "messages": [{"role": "user", "content": PROMPT_CANARY}], "stream": True})
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_terminal_usage(raw, "message_stop")


def test_auth_model_access_and_invalid_shape_reject_before_upstream(api, router):
    status, _, raw = api("/v1/chat/completions", {"model": "chat", "messages": [{"role": "user", "content": PROMPT_CANARY}]}, token="not-a-caller")
    assert status == 401 and body((status, None, raw))["error"]["type"] == "unauthorized"
    assert router["upstream"].calls == []
    status, _, raw = api("/v1/responses", {"model": "responses", "input": PROMPT_CANARY}, token=DENIED_CALLER)
    assert status == 403 and body((status, None, raw))["error"]["type"] == "model-not-allowed"
    assert router["upstream"].calls == []
    status, _, raw = api("/v1/chat/completions", {"model": "chat", "input": PROMPT_CANARY})
    assert status == 400 and body((status, None, raw))["error"]["type"] == "responses-body-on-chat-endpoint-disabled"
    assert router["upstream"].calls == []
    status, _, raw = api("/v1/responses", {"model": "responses", "input": PROMPT_CANARY, "tools": [{"type": "sse", "url": "http://127.0.0.1/never-call"}]})
    assert status == 400 and body((status, None, raw))["error"]["type"] == "provider-hosted-tools-forbidden"
    assert router["upstream"].calls == []


def test_stateless_responses_to_chat_and_chat_to_responses_bridges(api, router):
    status, _, raw = api("/v1/responses", {"model": "responses-to-chat", "input": PROMPT_CANARY})
    bridge_text = body((status, None, raw))
    assert status == 200 and bridge_text["object"] == "response" and bridge_text["output_text"] == "synthetic chat"
    assert router["upstream"].calls[0]["path"] == "/v1/chat/completions"
    status, _, raw = api("/v1/responses", {"model": "responses-to-chat", "input": PROMPT_CANARY, "tools": [{"type": "function", "name": "lookup", "description": TOOL_CANARY, "parameters": {"type": "object"}}], "tool_choice": "auto"})
    bridge_tool = body((status, None, raw))
    assert status == 200 and any(item["type"] == "function_call" and item["name"] == "lookup" for item in bridge_tool["output"])
    status, _, raw = api("/v1/chat/completions", {"model": "chat-to-responses", "messages": [{"role": "user", "content": PROMPT_CANARY}], "tools": [{"type": "function", "function": {"name": "lookup", "description": TOOL_CANARY, "parameters": {"type": "object"}}}], "tool_choice": "auto"})
    chat_tool = body((status, None, raw))
    assert status == 200 and chat_tool["object"] == "chat.completion" and chat_tool["choices"][0]["message"]["tool_calls"][0]["function"]["name"] == "lookup"
    assert router["upstream"].calls[-1]["path"] == "/v1/responses"
    before = len(router["upstream"].calls)
    status, _, raw = api("/v1/responses", {"model": "responses-to-chat", "input": PROMPT_CANARY, "previous_response_id": "resp_synthetic"})
    assert status == 502 and body((status, None, raw))["error"]["type"] == "no-eligible-target"
    assert len(router["upstream"].calls) == before
    status, _, raw = api("/v1/chat/completions", {"model": "chat-to-responses", "messages": [{"role": "user", "content": PROMPT_CANARY}], "stream": True})
    assert status == 502 and body((status, None, raw))["error"]["type"] == "no-eligible-target"
    assert len(router["upstream"].calls) == before


def test_generated_artifacts_redact_synthetic_auth_and_provider_values(router):
    assert_generated_artifacts_redacted(router)
