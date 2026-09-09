#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Run opencode-style API capability probes.

The probes use synthetic prompts, synthetic tool schemas, and a public receipt
image URL. They are intended to answer what an endpoint actually supports for
opencode-compatible request shapes instead of inferring support from catalog
metadata or provider marketing pages.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any


DEFAULT_IMAGE_URL = "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"
DEFAULT_TASKS = ("text", "tools", "image")
DEFAULT_DIALECTS = ("openai-chat", "anthropic")


@dataclass
class ProbeResult:
    dialect: str
    task: str
    model: str
    endpoint: str
    status: str
    http_status: int
    elapsed_ms: int
    request_bytes: int
    tool_count: int
    image_count: int
    finish_reason: str
    stop_reason: str
    usage: dict[str, Any]
    error: dict[str, Any]
    notes: str


def load_env_json(path: str | None) -> None:
    if not path:
        return
    env_path = Path(path)
    if not env_path.exists():
        return
    data = json.loads(env_path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"{path} must contain a JSON object")
    for key, value in data.items():
        if isinstance(value, str) and key not in os.environ:
            os.environ[key] = value


def endpoint_for(base_url: str, dialect: str) -> str:
    base = base_url.rstrip("/")
    if dialect == "openai-chat":
        return base + "/chat/completions"
    if dialect == "anthropic":
        return base + "/messages"
    raise ValueError(f"unsupported dialect {dialect}")


def opencode_tool_openai() -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": "read_workspace_file",
            "description": "Read a synthetic workspace file for capability validation.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "Relative path inside a synthetic workspace.",
                    }
                },
                "required": ["path"],
                "additionalProperties": False,
            },
        },
    }


def opencode_tool_anthropic() -> dict[str, Any]:
    return {
        "name": "read_workspace_file",
        "description": "Read a synthetic workspace file for capability validation.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Relative path inside a synthetic workspace.",
                }
            },
            "required": ["path"],
            "additionalProperties": False,
        },
    }


def build_openai_payload(model: str, task: str, image_url: str, max_tokens: int) -> dict[str, Any]:
    if task == "text":
        return {
            "model": model,
            "messages": [{"role": "user", "content": "Reply OK only."}],
            "max_tokens": max_tokens,
            "stream": False,
        }
    if task == "tools":
        return {
            "model": model,
            "messages": [
                {
                    "role": "user",
                    "content": "Use the read_workspace_file tool for README.md.",
                }
            ],
            "tools": [opencode_tool_openai()],
            "tool_choice": "auto",
            "parallel_tool_calls": True,
            "max_tokens": max(max_tokens, 128),
            "stream": False,
        }
    if task == "image":
        return {
            "model": model,
            "messages": [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "text",
                            "text": "Read the receipt image. Reply with only the merchant name.",
                        },
                        {
                            "type": "image_url",
                            "image_url": {"url": image_url, "detail": "high"},
                        },
                    ],
                }
            ],
            "max_tokens": max(max_tokens, 512),
            "stream": False,
        }
    raise ValueError(f"unsupported task {task}")


def build_anthropic_payload(model: str, task: str, image_url: str, max_tokens: int) -> dict[str, Any]:
    if task == "text":
        return {
            "model": model,
            "messages": [{"role": "user", "content": "Reply OK only."}],
            "max_tokens": max_tokens,
        }
    if task == "tools":
        return {
            "model": model,
            "messages": [
                {
                    "role": "user",
                    "content": "Use the read_workspace_file tool for README.md.",
                }
            ],
            "tools": [opencode_tool_anthropic()],
            "tool_choice": {"type": "auto"},
            "max_tokens": max(max_tokens, 128),
        }
    if task == "image":
        return {
            "model": model,
            "messages": [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "text",
                            "text": "Read the receipt image. Reply with only the merchant name.",
                        },
                        {
                            "type": "image",
                            "source": {"type": "url", "url": image_url},
                        },
                    ],
                }
            ],
            "max_tokens": max(max_tokens, 512),
        }
    raise ValueError(f"unsupported task {task}")


def build_payload(dialect: str, model: str, task: str, image_url: str, max_tokens: int) -> dict[str, Any]:
    if dialect == "openai-chat":
        return build_openai_payload(model, task, image_url, max_tokens)
    if dialect == "anthropic":
        return build_anthropic_payload(model, task, image_url, max_tokens)
    raise ValueError(f"unsupported dialect {dialect}")


def count_images(payload: dict[str, Any]) -> int:
    def visit(value: Any) -> int:
        if isinstance(value, dict):
            count = 1 if value.get("type") in {"image", "image_url"} else 0
            return count + sum(visit(child) for child in value.values())
        if isinstance(value, list):
            return sum(visit(child) for child in value)
        return 0

    return visit(payload)


def detect_openai_tool_call(parsed: dict[str, Any]) -> bool:
    choices = parsed.get("choices")
    if not isinstance(choices, list) or not choices:
        return False
    message = choices[0].get("message") if isinstance(choices[0], dict) else None
    return isinstance(message, dict) and bool(message.get("tool_calls"))


def detect_anthropic_tool_use(parsed: dict[str, Any]) -> bool:
    content = parsed.get("content")
    if not isinstance(content, list):
        return False
    return any(isinstance(item, dict) and item.get("type") == "tool_use" for item in content)


def extract_openai_text(parsed: dict[str, Any]) -> str:
    choices = parsed.get("choices")
    choice = choices[0] if isinstance(choices, list) and choices else {}
    message = choice.get("message") if isinstance(choice, dict) else {}
    content = message.get("content") if isinstance(message, dict) else ""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, dict) and isinstance(item.get("text"), str):
                parts.append(item["text"])
        return "\n".join(parts)
    return ""


def extract_anthropic_text(parsed: dict[str, Any]) -> str:
    content = parsed.get("content")
    if not isinstance(content, list):
        return ""
    parts = []
    for item in content:
        if isinstance(item, dict) and item.get("type") == "text" and isinstance(item.get("text"), str):
            parts.append(item["text"])
    return "\n".join(parts)


def image_answer_passes(text: str, expected: str) -> bool:
    return expected.casefold() in text.casefold()


def expected_text_passes(text: str, expected: str) -> bool:
    return expected.casefold() in text.casefold()


def classify_success(
    dialect: str,
    task: str,
    parsed: dict[str, Any],
    expected_text: str,
    expected_image_text: str,
) -> tuple[str, str, str, str]:
    if dialect == "openai-chat":
        choices = parsed.get("choices")
        choice = choices[0] if isinstance(choices, list) and choices else {}
        finish = str(choice.get("finish_reason") or "") if isinstance(choice, dict) else ""
        if task == "tools":
            return ("pass" if detect_openai_tool_call(parsed) else "fail", finish, "", "")
        if task == "text" and not expected_text_passes(extract_openai_text(parsed), expected_text):
            return "fail", finish, "", "text response did not contain expected text"
        if task == "image" and not image_answer_passes(extract_openai_text(parsed), expected_image_text):
            return "fail", finish, "", "image response did not contain expected receipt text"
        return "pass", finish, "", ""
    if dialect == "anthropic":
        stop = str(parsed.get("stop_reason") or "")
        if task == "tools":
            return ("pass" if detect_anthropic_tool_use(parsed) else "fail", "", stop, "")
        if task == "text" and not expected_text_passes(extract_anthropic_text(parsed), expected_text):
            return "fail", "", stop, "text response did not contain expected text"
        if task == "image" and not image_answer_passes(extract_anthropic_text(parsed), expected_image_text):
            return "fail", "", stop, "image response did not contain expected receipt text"
        return "pass", "", stop, ""
    return "fail", "", "", ""


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


def run_probe(
    base_url: str,
    api_key: str,
    dialect: str,
    model: str,
    task: str,
    image_url: str,
    max_tokens: int,
    expected_text: str,
    expected_image_text: str,
    timeout: float,
    user_agent: str,
    dry_run: bool,
) -> ProbeResult:
    endpoint = endpoint_for(base_url, dialect)
    payload = build_payload(dialect, model, task, image_url, max_tokens)
    raw = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    if dry_run:
        return ProbeResult(
            dialect=dialect,
            task=task,
            model=model,
            endpoint=endpoint,
            status="dry-run",
            http_status=0,
            elapsed_ms=0,
            request_bytes=len(raw),
            tool_count=len(payload.get("tools", [])),
            image_count=count_images(payload),
            finish_reason="",
            stop_reason="",
            usage={},
            error={},
            notes="request was built but not sent",
        )

    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "Accept": "application/json",
        "User-Agent": user_agent,
    }
    if dialect == "anthropic":
        headers["anthropic-version"] = "2023-06-01"

    req = urllib.request.Request(
        endpoint,
        data=raw,
        method="POST",
        headers=headers,
    )
    started = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read(2 * 1024 * 1024)
            elapsed_ms = int((time.monotonic() - started) * 1000)
            parsed = json.loads(body.decode("utf-8", errors="replace"))
            status, finish, stop, notes = classify_success(
                dialect,
                task,
                parsed,
                expected_text,
                expected_image_text,
            )
            usage = parsed.get("usage") if isinstance(parsed, dict) else {}
            return ProbeResult(
                dialect=dialect,
                task=task,
                model=model,
                endpoint=endpoint,
                status=status,
                http_status=resp.status,
                elapsed_ms=elapsed_ms,
                request_bytes=len(raw),
                tool_count=len(payload.get("tools", [])),
                image_count=count_images(payload),
                finish_reason=finish,
                stop_reason=stop,
                usage=usage if isinstance(usage, dict) else {},
                error={},
                notes=notes,
            )
    except urllib.error.HTTPError as exc:
        body = exc.read(2 * 1024 * 1024)
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return ProbeResult(
            dialect=dialect,
            task=task,
            model=model,
            endpoint=endpoint,
            status="http-error",
            http_status=exc.code,
            elapsed_ms=elapsed_ms,
            request_bytes=len(raw),
            tool_count=len(payload.get("tools", [])),
            image_count=count_images(payload),
            finish_reason="",
            stop_reason="",
            usage={},
            error=sanitize_error_body(body),
            notes="provider or router returned a non-2xx response",
        )
    except (urllib.error.URLError, TimeoutError) as exc:
        elapsed_ms = int((time.monotonic() - started) * 1000)
        return ProbeResult(
            dialect=dialect,
            task=task,
            model=model,
            endpoint=endpoint,
            status="network-error",
            http_status=0,
            elapsed_ms=elapsed_ms,
            request_bytes=len(raw),
            tool_count=len(payload.get("tools", [])),
            image_count=count_images(payload),
            finish_reason="",
            stop_reason="",
            usage={},
            error={"reasonClass": type(exc).__name__},
            notes="request did not complete",
        )


def parse_csv(value: str, allowed: tuple[str, ...], label: str) -> tuple[str, ...]:
    items = tuple(item.strip() for item in value.split(",") if item.strip())
    unknown = sorted(set(items) - set(allowed))
    if unknown:
        raise SystemExit(f"unknown {label}: {', '.join(unknown)}")
    return items


def write_outputs(results: list[ProbeResult], output_dir: Path) -> tuple[Path, Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    rows = [asdict(result) for result in results]
    json_path = output_dir / "opencode-api-matrix.json"
    md_path = output_dir / "opencode-api-matrix.md"
    json_path.write_text(json.dumps({"results": rows}, indent=2) + "\n", encoding="utf-8")
    lines = [
        "# opencode API Capability Matrix",
        "",
        "| Dialect | Task | Model | Status | HTTP | ms | Bytes | Tools | Images | Finish | Stop | Notes |",
        "|---|---|---|---|---:|---:|---:|---:|---:|---|---|---|",
    ]
    for row in rows:
        lines.append(
            "| {dialect} | {task} | {model} | {status} | {http_status} | {elapsed_ms} | {request_bytes} | {tool_count} | {image_count} | {finish_reason} | {stop_reason} | {notes} |".format(
                **{k: str(v).replace("|", "\\|") for k, v in row.items()}
            )
        )
    md_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return json_path, md_path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--api-key-env", required=True)
    parser.add_argument("--env-json", default="env.json")
    parser.add_argument("--dialects", default=",".join(DEFAULT_DIALECTS))
    parser.add_argument("--tasks", default=",".join(DEFAULT_TASKS))
    parser.add_argument("--image-url", default=DEFAULT_IMAGE_URL)
    parser.add_argument("--expected-text", default="OK")
    parser.add_argument("--expected-image-text", default="Rite Aid")
    parser.add_argument("--max-tokens", type=int, default=64)
    parser.add_argument("--timeout-seconds", type=float, default=120)
    parser.add_argument("--user-agent", default="opencode-api-capability-smoke")
    parser.add_argument("--output-dir", default="tmp/opencode-api-matrix")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument(
        "--strict-exit",
        action="store_true",
        help="exit 1 when any probe row fails, returns non-2xx, or hits a network error",
    )
    args = parser.parse_args()

    dialects = parse_csv(args.dialects, DEFAULT_DIALECTS, "dialects")
    tasks = parse_csv(args.tasks, DEFAULT_TASKS, "tasks")
    load_env_json(args.env_json)
    api_key = os.environ.get(args.api_key_env, "")
    if not api_key and not args.dry_run:
        raise SystemExit(f"{args.api_key_env} is not set")

    results = [
        run_probe(
            base_url=args.base_url,
            api_key=api_key,
            dialect=dialect,
            model=args.model,
            task=task,
            image_url=args.image_url,
            max_tokens=args.max_tokens,
            expected_text=args.expected_text,
            expected_image_text=args.expected_image_text,
            timeout=args.timeout_seconds,
            user_agent=args.user_agent,
            dry_run=args.dry_run,
        )
        for dialect in dialects
        for task in tasks
    ]
    json_path, md_path = write_outputs(results, Path(args.output_dir))
    print(json.dumps({"results": [asdict(result) for result in results]}, indent=2))
    print(f"wrote {json_path}", file=sys.stderr)
    print(f"wrote {md_path}", file=sys.stderr)
    failed = any(result.status in {"fail", "http-error", "network-error"} for result in results)
    return 1 if args.strict_exit and failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
