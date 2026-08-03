import hashlib
import json
import os
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

import pytest


CALLER = "synthetic-api-compat-caller"
DENIED_CALLER = "synthetic-api-compat-denied"
PROMPT_CANARY = "api-compat-prompt-canary"
TOOL_CANARY = "api-compat-tool-canary"
FORBIDDEN_ARTIFACT_VALUES = {CALLER, DENIED_CALLER, "Authorization:", PROMPT_CANARY, TOOL_CANARY}


def unused_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class FakeUpstream(BaseHTTPRequestHandler):
    calls = []

    def log_message(self, _format, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.__class__.calls.append({"path": self.path, "body": body})
        model = body.get("model", "")
        if self.path.endswith("/chat/completions"):
            message = {"role": "assistant", "content": "synthetic chat"}
            if body.get("tools"):
                message["tool_calls"] = [{"id": "call_synthetic", "type": "function", "function": {"name": "lookup", "arguments": "{}"}}]
            response = {"id": "chatcmpl_synthetic", "object": "chat.completion", "choices": [{"index": 0, "message": message, "finish_reason": "tool_calls" if body.get("tools") else "stop"}], "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}}
        elif self.path.endswith("/responses"):
            output = [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "synthetic responses"}]}]
            if body.get("tools"):
                output = [{"type": "function_call", "id": "fc_synthetic", "call_id": "call_synthetic", "name": "lookup", "arguments": "{}"}]
            response = {"id": "resp_synthetic", "object": "response", "model": model, "status": "completed", "output": output, "output_text": "synthetic responses", "usage": {"input_tokens": 3, "output_tokens": 2, "total_tokens": 5}}
        elif self.path.endswith("/messages"):
            response = {"id": "msg_synthetic", "type": "message", "role": "assistant", "model": model, "stop_reason": "end_turn", "content": [{"type": "text", "text": "synthetic messages"}], "usage": {"input_tokens": 3, "output_tokens": 2}}
        else:
            self.send_error(404)
            return
        raw = json.dumps(response).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def request(base, path, payload=None, token=CALLER):
    headers = {"Authorization": f"Bearer {token}"}
    data = None if payload is None else json.dumps(payload).encode()
    if data:
        headers["Content-Type"] = "application/json"
    try:
        with urlopen(Request(base + path, data=data, headers=headers), timeout=5) as response:
            return response.status, response.headers, response.read()
    except HTTPError as error:
        return error.code, error.headers, error.read()


@pytest.fixture(scope="session")
def router(tmp_path_factory):
    work = tmp_path_factory.mktemp("api-compat")
    FakeUpstream.calls = []
    upstream = ThreadingHTTPServer(("127.0.0.1", 0), FakeUpstream)
    thread = threading.Thread(target=upstream.serve_forever, daemon=True)
    thread.start()
    upstream_url = f"http://127.0.0.1:{upstream.server_port}"
    port = unused_port()
    digest = lambda value: hashlib.sha256(value.encode()).hexdigest()
    config = f'''server:
  listen: "127.0.0.1:{port}"
  default_model_group: chat
  cache: {{enabled: false}}
  usage_db: {{enabled: false}}
  logging: {{path: "{work / 'router.jsonl'}"}}
state_path: "{work / 'state.json'}"
providers:
  chat: {{base_url: "{upstream_url}/v1", dialect: openai-chat}}
  responses: {{base_url: "{upstream_url}/v1", dialect: openai-responses}}
  messages: {{base_url: "{upstream_url}/anthropic", dialect: anthropic}}
models:
  chat: {{strategy: static, targets: [{{provider: chat, model: synthetic-chat, tool_support: {{openai_chat: [function, tool_choice]}}}}]}}
  responses: {{strategy: static, targets: [{{provider: responses, model: synthetic-responses, tool_support: {{openai_responses: [function, tool_choice]}}}}]}}
  messages: {{strategy: static, targets: [{{provider: messages, model: synthetic-messages, tool_support: {{anthropic_messages: [client_tools]}}, request_shape_support: {{supported_inbound_dialects: [anthropic], validation_status: passed}}}}]}}
  responses-to-chat: {{strategy: static, targets: [{{provider: chat, model: synthetic-bridge-chat, tool_support: {{openai_chat: [function, tool_choice]}}, responses_to_chat: {{enabled: true, text: true, function_tools: true, tool_choice: true, validation_status: passed}}}}]}}
  chat-to-responses: {{strategy: static, targets: [{{provider: responses, model: synthetic-bridge-responses, tool_support: {{openai_responses: [function, tool_choice]}}, bridges: {{chat_to_responses: {{enabled: true, text: true, tools: true, tool_choice: true}}}}}}]}}
callers:
  - id: synthetic-allowed
    token_sha256: {digest(CALLER)}
    allow: [chat, responses, messages, responses-to-chat, chat-to-responses]
  - id: synthetic-denied
    token_sha256: {digest(DENIED_CALLER)}
    allow: [chat]
'''
    config_path = work / "config.yaml"
    config_path.write_text(config)
    build_env = os.environ | {"GOPROXY": "off", "GOSUMDB": "off"}
    router_binary = work / "router"
    subprocess.run(["go", "build", "-tags", "dev_no_license", "-o", str(router_binary), "./cmd/router"], cwd=Path(__file__).parents[3], env=build_env, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    process = subprocess.Popen([str(router_binary), "-config", "config.yaml"], cwd=work, env=build_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
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
    yield {"base": base, "work": work, "upstream": FakeUpstream}
    process.terminate()
    process.wait(timeout=5)
    upstream.shutdown()


@pytest.fixture(autouse=True)
def clear_calls(router):
    router["upstream"].calls.clear()
    yield
    assert_generated_artifacts_redacted(router)


def assert_generated_artifacts_redacted(router):
    for path in Path(router["work"]).rglob("*"):
        if not path.is_file():
            continue
        contents = path.read_bytes()
        assert not any(value.encode() in contents for value in FORBIDDEN_ARTIFACT_VALUES)


@pytest.fixture
def api(router):
    return lambda path, payload=None, token=CALLER: request(router["base"], path, payload, token)
