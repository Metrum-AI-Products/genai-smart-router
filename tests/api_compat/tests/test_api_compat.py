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
        event_name = ""
        data_lines = []
        for line in frame.split("\n"):
            if line.startswith("event: "):
                event_name = line[7:]
            elif line.startswith("data: "):
                data_lines.append(line[6:])
        if not data_lines:
            continue
        data = "\n".join(data_lines)
        events.append((event_name, data if data == "[DONE]" else json.loads(data)))
    return events


def assert_chat_terminal_usage(raw):
    events = sse_events(raw)
    assert events[-1] == ("", "[DONE]")
    chunks = [payload for _, payload in events[:-1]]
    assert any(chunk["choices"][0]["delta"].get("content") == "synthetic chat" for chunk in chunks)
    terminal = chunks[-1]
    assert terminal["choices"][0]["finish_reason"] == "stop"
    assert terminal["usage"] == {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}


def assert_responses_terminal_usage(raw):
    events = sse_events(raw)
    assert events[-1] == ("", "[DONE]")
    assert any(
        name == "response.output_text.delta" and payload["delta"] == "synthetic responses"
        for name, payload in events
    )
    assert events[-2][0] == "response.completed"
    completed = events[-2][1]["response"]
    assert completed["status"] == "completed"
    assert completed["usage"] == {"input_tokens": 3, "output_tokens": 2, "total_tokens": 5}


def assert_messages_terminal_usage(raw):
    events = sse_events(raw)
    names = [name for name, _ in events]
    content_index = names.index("content_block_delta")
    usage_index = names.index("message_delta")
    stop_index = names.index("message_stop")
    assert content_index < usage_index < stop_index == len(events) - 1
    assert events[content_index][1]["delta"] == {"type": "text_delta", "text": "synthetic messages"}
    assert events[usage_index][1]["usage"] == {"output_tokens": 2}


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
    assert_chat_terminal_usage(raw)


def test_responses_tool_and_sse_terminal_usage(api, router):
    payload = {"model": "responses", "input": PROMPT_CANARY, "tools": [{"type": "function", "name": "lookup", "description": TOOL_CANARY, "parameters": {"type": "object"}}], "tool_choice": "auto"}
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


def test_anthropic_messages_tool_choice_and_sse_usage(api, router):
    status, _, raw = api("/anthropic/v1/messages", {"model": "messages", "max_tokens": 32, "messages": [{"role": "user", "content": PROMPT_CANARY}], "tools": [{"name": "lookup", "description": TOOL_CANARY, "input_schema": {"type": "object"}}], "tool_choice": {"type": "tool", "name": "lookup"}})
    assert status == 200
    message = body((status, None, raw))
    assert message["type"] == "message"
    assert message["stop_reason"] == "tool_use"
    assert message["content"] == [
        {"type": "tool_use", "id": "toolu_synthetic", "name": "lookup", "input": {}}
    ]
    assert message["usage"] == {"input_tokens": 3, "output_tokens": 2}
    assert router["upstream"].calls[0]["path"] == "/anthropic/v1/messages"
    assert router["upstream"].calls[0]["body"]["tool_choice"]["name"] == "lookup"
    status, headers, raw = api("/anthropic/v1/messages", {"model": "messages", "max_tokens": 32, "messages": [{"role": "user", "content": PROMPT_CANARY}], "stream": True})
    assert status == 200 and "text/event-stream" in headers["Content-Type"]
    assert_messages_terminal_usage(raw)


def test_auth_model_access_and_invalid_shape_reject_before_upstream(api, router):
    status, _, raw = api(
        "/v1/chat/completions",
        {"model": "chat", "messages": [{"role": "user", "content": PROMPT_CANARY}]},
        token=None,
    )
    assert status == 401
    assert body((status, None, raw)) == {
        "error": {"type": "unauthorized", "message": "unauthorized"}
    }
    assert router["upstream"].calls == []
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
    status, _, raw = api(
        "/v1/chat/completions",
        {
            "model": "chat-to-responses",
            "messages": [{"role": "user", "content": PROMPT_CANARY}],
        },
    )
    chat_text = body((status, None, raw))
    assert status == 200
    assert chat_text["object"] == "chat.completion"
    assert chat_text["choices"][0]["message"]["content"] == "synthetic responses"
    assert router["upstream"].calls[-1]["path"] == "/v1/responses"
    assert router["upstream"].calls[-1]["body"]["input"] == [
        {
            "role": "user",
            "content": [{"type": "input_text", "text": PROMPT_CANARY}],
        }
    ]
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
