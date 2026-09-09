#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Generate and send a sanitized large OpenAI Chat payload.

The fixture intentionally uses synthetic filler and generic tool schemas so it
can be shared in issues and CI logs without exposing prompts, tool outputs, or
customer data.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


DEFAULT_TOOL_COUNT = 24
DEFAULT_TARGET_BYTES = 512 * 1024


def load_env_json(path: str) -> None:
    env_path = Path(path)
    if not env_path.exists():
        return
    data = json.loads(env_path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"{path} must contain a JSON object")
    for key, value in data.items():
        if isinstance(value, str) and key not in os.environ:
            os.environ[key] = value


def make_tool(index: int, padding: str) -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": f"lookup_workspace_symbol_{index:02d}",
            "description": (
                "Synthetic coding-agent tool used only for request-shape validation. "
                "It represents a schema-heavy editor or repository helper. "
                + padding
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "workspace": {
                        "type": "string",
                        "description": "Synthetic workspace identifier.",
                    },
                    "query": {
                        "type": "string",
                        "description": "Synthetic lookup query. " + padding,
                    },
                    "limit": {
                        "type": "integer",
                        "minimum": 1,
                        "maximum": 50,
                    },
                    "include_context": {"type": "boolean"},
                },
                "required": ["workspace", "query"],
                "additionalProperties": False,
            },
        },
    }


def make_payload(
    model: str,
    target_bytes: int,
    tool_count: int,
    stream: bool,
    max_tokens: int,
) -> dict[str, Any]:
    padding_unit = "safe synthetic request-shape filler for large agent validation. "
    tool_padding = padding_unit * 12
    tools = [make_tool(i, tool_padding) for i in range(tool_count)]
    messages: list[dict[str, str]] = [
        {
            "role": "system",
            "content": "You are validating request-shape transport. Reply OK only.",
        }
    ]
    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "tools": tools,
        "tool_choice": "auto",
        "parallel_tool_calls": True,
        "stream": stream,
        "max_tokens": max_tokens,
    }

    base_size = len(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
    remaining = max(0, target_bytes - base_size)
    chunk = (padding_unit * 16).strip()
    turn = 0
    while remaining > 0:
        content = chunk[: max(256, min(len(chunk), remaining))]
        messages.append(
            {
                "role": "user" if turn % 2 == 0 else "assistant",
                "content": f"turn={turn}; {content}",
            }
        )
        remaining = target_bytes - len(
            json.dumps(payload, separators=(",", ":")).encode("utf-8")
        )
        turn += 1
        if turn > 4000:
            raise SystemExit("failed to converge while generating payload")
    messages.append({"role": "user", "content": "Reply OK only."})
    return payload


def payload_summary(payload: dict[str, Any]) -> dict[str, Any]:
    raw = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    tool_bytes = len(
        json.dumps(payload.get("tools", []), separators=(",", ":")).encode("utf-8")
    )
    return {
        "requestBytes": len(raw),
        "messageCount": len(payload.get("messages", [])),
        "toolCount": len(payload.get("tools", [])),
        "toolSchemaBytes": tool_bytes,
        "stream": bool(payload.get("stream")),
        "maxTokens": payload.get("max_tokens"),
    }


def sanitize_error_body(body: bytes) -> dict[str, Any]:
    if not body:
        return {}
    try:
        parsed = json.loads(body.decode("utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {"bodyClass": "non-json", "bodyBytes": len(body)}
    if not isinstance(parsed, dict):
        return {"bodyClass": type(parsed).__name__, "bodyBytes": len(body)}
    err = parsed.get("error")
    if isinstance(err, dict):
        return {
            "code": err.get("code"),
            "type": err.get("type"),
            "param": err.get("param"),
            "messageClass": "present" if err.get("message") else "",
        }
    return {
        "code": parsed.get("code"),
        "type": parsed.get("type"),
        "param": parsed.get("param"),
        "messageClass": "present" if parsed.get("message") else "",
    }


def call_chat(
    base_url: str,
    api_key: str,
    payload: dict[str, Any],
    user_agent: str,
    timeout: float,
) -> dict[str, Any]:
    url = base_url.rstrip("/") + "/chat/completions"
    raw = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=raw,
        method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": user_agent,
        },
    )
    started = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read(1024 * 1024)
            elapsed_ms = int((time.monotonic() - started) * 1000)
            if payload.get("stream"):
                return {
                    "status": resp.status,
                    "elapsedMs": elapsed_ms,
                    "finishReason": first_stream_finish_reason(body),
                    "responseClass": "ok-stream",
                    "responseBytesRead": len(body),
                }
            parsed = json.loads(body.decode("utf-8", errors="replace"))
            usage = parsed.get("usage") if isinstance(parsed, dict) else {}
            return {
                "status": resp.status,
                "elapsedMs": elapsed_ms,
                "usage": usage if isinstance(usage, dict) else {},
                "finishReason": first_finish_reason(parsed),
                "responseClass": "ok",
            }
    except urllib.error.HTTPError as exc:
        body = exc.read(1024 * 1024)
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": exc.code,
            "elapsedMs": elapsed_ms,
            "responseClass": "http-error",
            "error": sanitize_error_body(body),
        }
    except urllib.error.URLError as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return {
            "status": 0,
            "elapsedMs": elapsed_ms,
            "responseClass": "network-error",
            "error": {"reasonClass": type(exc.reason).__name__},
        }


def first_finish_reason(parsed: Any) -> str:
    if not isinstance(parsed, dict):
        return ""
    choices = parsed.get("choices")
    if not isinstance(choices, list) or not choices:
        return ""
    choice = choices[0]
    if not isinstance(choice, dict):
        return ""
    return str(choice.get("finish_reason") or "")


def first_stream_finish_reason(body: bytes) -> str:
    text = body.decode("utf-8", errors="replace")
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("data:"):
            continue
        data = line[5:].strip()
        if not data or data == "[DONE]":
            continue
        try:
            parsed = json.loads(data)
        except json.JSONDecodeError:
            continue
        reason = first_finish_reason(parsed)
        if reason:
            return reason
    return ""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--api-key-env", required=True)
    parser.add_argument("--env-json", default="env.json")
    parser.add_argument("--target-bytes", type=int, default=DEFAULT_TARGET_BYTES)
    parser.add_argument("--tool-count", type=int, default=DEFAULT_TOOL_COUNT)
    parser.add_argument("--max-tokens", type=int, default=32)
    parser.add_argument("--stream", action="store_true")
    parser.add_argument("--timeout-seconds", type=float, default=120)
    parser.add_argument("--user-agent", default="smart-llmrouter-large-payload-smoke")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    if args.target_bytes < 1024:
        raise SystemExit("--target-bytes must be at least 1024")
    if args.tool_count < 0:
        raise SystemExit("--tool-count cannot be negative")

    load_env_json(args.env_json)
    payload = make_payload(
        model=args.model,
        target_bytes=args.target_bytes,
        tool_count=args.tool_count,
        stream=args.stream,
        max_tokens=args.max_tokens,
    )
    result: dict[str, Any] = {
        "baseUrlHost": args.base_url.split("//", 1)[-1].split("/", 1)[0],
        "model": args.model,
        "summary": payload_summary(payload),
    }
    if not args.dry_run:
        api_key = os.environ.get(args.api_key_env)
        if not api_key:
            raise SystemExit(f"{args.api_key_env} is not set")
        result["result"] = call_chat(
            args.base_url,
            api_key,
            payload,
            args.user_agent,
            args.timeout_seconds,
        )
    print(json.dumps(result, indent=2, sort_keys=True))
    status = result.get("result", {}).get("status", 200)
    return 0 if 200 <= int(status) < 300 else 1


if __name__ == "__main__":
    sys.exit(main())
