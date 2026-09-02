#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Demo external routing policy service for GenAI Smart Router.

The router POSTs a JSON policy request containing safe derived request context,
caller metadata, and the eligible target list. This service selects a target
based on prompt size without receiving prompt text by default.
"""

from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


PROMPT_CHAR_THRESHOLD = 8_000


class PolicyHandler(BaseHTTPRequestHandler):
    server_version = "prompt-size-policy/1.0"

    def do_POST(self) -> None:
        if self.path != "/route":
            self._write_json(404, {"error": "not found"})
            return

        length = int(self.headers.get("content-length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        targets = payload.get("targets") or []
        context = payload.get("context") or {}
        text_chars = int(context.get("textChars") or 0)

        indexed = list(enumerate(targets))
        candidates = [
            (index, target)
            for index, target in indexed
            if target.get("keyConfigured") and int(target.get("weight") or 0) > 0
        ]
        if not candidates:
            self._write_json(200, {"targetIndex": 0, "classLabel": "prompt-size:no-candidates"})
            return

        preferred_tier = "heavy" if text_chars > PROMPT_CHAR_THRESHOLD else "cheap"
        selected_index = candidates[0][0]
        for index, target in candidates:
            if target.get("tier") == preferred_tier:
                selected_index = index
                break

        fallback_indexes = [index for index, _target in candidates if index != selected_index]
        self._write_json(
            200,
            {
                "targetIndex": selected_index,
                "fallbackIndexes": fallback_indexes,
                "classLabel": f"prompt-size:{preferred_tier}",
                "metadata": {"textChars": text_chars},
            },
        )

    def log_message(self, format: str, *args: object) -> None:
        return

    def _write_json(self, status: int, body: dict) -> None:
        raw = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def main() -> None:
    server = ThreadingHTTPServer(("127.0.0.1", 18090), PolicyHandler)
    print("external routing policy service listening on http://127.0.0.1:18090/route", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
