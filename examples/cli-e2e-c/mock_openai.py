#!/usr/bin/env python3
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        if length:
            self.rfile.read(length)
        if self.path.endswith("/responses"):
            self.write_json({
                "id": "resp_mock_hello",
                "object": "response",
                "status": "completed",
                "model": "mock-hello",
                "output_text": "hello world",
                "usage": {"input_tokens": 1, "output_tokens": 2, "total_tokens": 3},
            })
            return
        if self.path.endswith("/chat/completions"):
            self.write_json({
                "id": "chatcmpl_mock_hello",
                "object": "chat.completion",
                "model": "mock-hello",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": "hello world"},
                    "finish_reason": "stop",
                }],
                "usage": {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
            })
            return
        self.send_error(404)

    def log_message(self, fmt, *args):
        return

    def write_json(self, body):
        raw = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", 19001), Handler).serve_forever()
