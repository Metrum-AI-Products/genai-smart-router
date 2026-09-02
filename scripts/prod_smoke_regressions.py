#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Run production-derived router smoke fixtures without real customer content."""

from __future__ import annotations

import argparse
import csv
import json
import os
import sqlite3
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


DEFAULT_FIXTURES_DIR = Path("testdata/smokes/production-derived")
DEFAULT_FIXTURE = "large-openai-chat-tools.json"


def load_token(args: argparse.Namespace) -> str:
    if args.token_file:
        token = Path(args.token_file).read_text(encoding="utf-8").strip()
    else:
        token = os.environ.get(args.token_env, "").strip()
    if not token:
        raise SystemExit(f"missing router token; set {args.token_env} or pass --token-file")
    return token


def load_fixture(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def fixture_paths(args: argparse.Namespace) -> list[Path]:
    root = Path(args.fixtures_dir)
    if args.fixture == "all":
        paths: list[Path] = []
        for path in sorted(root.glob("*.json")):
            fixture = load_fixture(path)
            if fixture.get("replayable", True):
                paths.append(path)
        return paths
    return [root / args.fixture]


def make_tool(index: int) -> dict[str, Any]:
    return make_tool_with_description(index, "safe schema description " * 42)


def make_tool_with_description(index: int, description: str) -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": f"fixture_tool_{index:02d}",
            "description": description,
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "safe path field " * 16},
                    "content": {
                        "type": "string",
                        "description": "safe content field " * 16,
                    },
                },
                "required": ["path"],
            },
        },
    }


def fixture_value(fixture: dict[str, Any], scenario: dict[str, Any], key: str, default: Any = None) -> Any:
    if key in scenario:
        return scenario[key]
    return fixture.get(key, default)


def make_chat_payload(fixture: dict[str, Any], scenario: dict[str, Any], model_group: str) -> dict[str, Any]:
    message_count = int(fixture_value(fixture, scenario, "message_count", 1))
    tool_count = int(fixture_value(fixture, scenario, "tool_count", 0))
    filler = "production-derived-safe-filler " * 62
    tool_description = "safe schema description " * 42
    if fixture_value(fixture, scenario, "request_bytes_bucket") == "gt-1mb" or fixture_value(fixture, scenario, "tool_schema_bytes_bucket") == "gt-1mb":
        tool_description = "safe schema description " * 5000
    messages: list[dict[str, str]] = [
        {
            "role": "system",
            "content": "Synthetic production-derived large coding-agent fixture.",
        }
    ]
    for i in range(1, message_count):
        role = "assistant" if i % 2 == 0 else "user"
        messages.append({"role": role, "content": f"{filler} turn {i}"})
    payload: dict[str, Any] = {
        "model": model_group,
        "stream": bool(fixture_value(fixture, scenario, "stream", False)),
        "messages": messages,
        "metadata": {"fixture": fixture["name"], "scenario": scenario.get("name", fixture["name"])},
    }
    if tool_count:
        payload["tools"] = [make_tool_with_description(i, tool_description) for i in range(tool_count)]
        payload["tool_choice"] = fixture_value(fixture, scenario, "tool_choice_mode", "auto")
    if scenario.get("reasoning_control") == "reasoning_effort":
        payload["reasoning_effort"] = scenario.get("reasoning_effort", "low")
    if scenario.get("structured_output"):
        payload["response_format"] = {"type": "json_schema", "json_schema": {"name": "fixture", "schema": {"type": "object"}}}
    return payload


def make_responses_payload(fixture: dict[str, Any], scenario: dict[str, Any], model_group: str) -> dict[str, Any]:
    tool_count = int(fixture_value(fixture, scenario, "tool_count", 0))
    payload: dict[str, Any] = {
        "model": model_group,
        "stream": bool(fixture_value(fixture, scenario, "stream", False)),
        "input": "Synthetic production-derived Responses fixture. Reply OK only.",
        "metadata": {"fixture": fixture["name"], "scenario": scenario.get("name", fixture["name"])},
    }
    if tool_count:
        payload["tools"] = [
            {"type": "function", "name": f"fixture_tool_{i:02d}", "description": "safe tool", "parameters": {"type": "object"}}
            for i in range(tool_count)
        ]
    if scenario.get("reasoning_control") == "reasoning":
        payload["reasoning"] = {"effort": scenario.get("reasoning_effort", "low")}
    if scenario.get("previous_response_id_present"):
        payload["previous_response_id"] = "resp_synthetic_previous"
    return payload


def make_anthropic_payload(fixture: dict[str, Any], scenario: dict[str, Any], model_group: str) -> dict[str, Any]:
    tool_count = int(fixture_value(fixture, scenario, "tool_count", 0))
    payload: dict[str, Any] = {
        "model": model_group,
        "stream": bool(fixture_value(fixture, scenario, "stream", False)),
        "max_tokens": int(scenario.get("max_tokens", 512)),
        "messages": [{"role": "user", "content": "Synthetic production-derived Messages fixture. Reply OK only."}],
        "metadata": {"fixture": fixture["name"], "scenario": scenario.get("name", fixture["name"])},
    }
    if tool_count:
        payload["tools"] = [{"name": f"fixture_tool_{i:02d}", "description": "safe tool", "input_schema": {"type": "object"}} for i in range(tool_count)]
    if scenario.get("reasoning_control") == "thinking":
        payload["thinking"] = {"type": "enabled", "budget_tokens": int(scenario.get("thinking_budget_tokens", 256))}
    return payload


def make_payload(fixture: dict[str, Any], scenario: dict[str, Any], model_group: str) -> tuple[str, dict[str, Any]]:
    surface = str(fixture_value(fixture, scenario, "surface", "openai_chat"))
    if surface == "openai_responses":
        return "/v1/responses", make_responses_payload(fixture, scenario, model_group)
    if surface == "anthropic_messages":
        return "/v1/messages", make_anthropic_payload(fixture, scenario, model_group)
    return "/v1/chat/completions", make_chat_payload(fixture, scenario, model_group)


def http_json(url: str, token: str, payload: dict[str, Any], timeout: float) -> tuple[int, dict[str, str], str]:
    raw = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=raw,
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "User-Agent": "smart-llmrouter-production-derived-smoke",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, dict(resp.headers.items()), resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        return exc.code, dict(exc.headers.items()), exc.read().decode("utf-8", errors="replace")


def request_id_from(headers: dict[str, str], body: str) -> str:
    for key, value in headers.items():
        if key.lower() == "x-request-id":
            return value.strip()
    try:
        decoded = json.loads(body)
    except json.JSONDecodeError:
        return ""
    if isinstance(decoded, dict):
        return str(decoded.get("request_id") or decoded.get("requestId") or "")
    return ""


def telemetry_sql(request_id: str) -> tuple[str, list[str]]:
    return (
        """
SELECT
  u.request_id,
  u.status,
  u.error,
  u.traffic_shape_decision,
  a.provider,
  a.model,
  a.dialect,
  a.error_class,
  a.selected,
  rs.message_count,
  rs.tool_count,
  rs.image_count,
  rs.total_request_bytes_bucket,
  rs.tool_schema_bytes_bucket,
  rs.estimated_input_tokens_bucket,
  ts.translated_tool_count,
  ts.translated_request_bytes_bucket
FROM request_usage u
LEFT JOIN request_attempts a ON a.request_id = u.request_id AND a.selected = TRUE
LEFT JOIN request_shapes rs ON rs.request_id = u.request_id
LEFT JOIN request_translation_shapes ts ON ts.request_id = u.request_id AND ts.attempt_index = a.attempt_index
WHERE u.request_id = ?
""",
        [request_id],
    )


def query_sqlite(path: str, request_id: str) -> dict[str, Any] | None:
    sql, params = telemetry_sql(request_id)
    with sqlite3.connect(path) as conn:
        conn.row_factory = sqlite3.Row
        row = conn.execute(sql, params).fetchone()
    return dict(row) if row else None


def query_postgres(psql_bin: str, dsn: str, request_id: str) -> dict[str, Any] | None:
    sql, _ = telemetry_sql(request_id)
    pg_sql = sql.replace("?", ":'request_id'")
    cmd = [
        psql_bin,
        dsn,
        "-X",
        "-q",
        "--csv",
        "-v",
        f"request_id={request_id}",
        "-c",
        pg_sql,
    ]
    proc = subprocess.run(cmd, check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    rows = list(csv.DictReader(proc.stdout.splitlines()))
    return rows[0] if rows else None


def query_telemetry(args: argparse.Namespace, request_id: str) -> dict[str, Any] | None:
    if args.sqlite_db:
        return query_sqlite(args.sqlite_db, request_id)
    if args.postgres_dsn:
        return query_postgres(args.psql_bin, args.postgres_dsn, request_id)
    return None


def normalize_bool(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    return str(value).lower() in {"1", "t", "true", "yes"}


def target_matches(row: dict[str, Any], target: dict[str, Any]) -> bool:
    return (
        row.get("provider") == target.get("provider")
        and row.get("model") == target.get("model")
        and row.get("dialect") == target.get("dialect")
        and normalize_bool(row.get("selected"))
    )


def scenario_list(fixture: dict[str, Any]) -> list[dict[str, Any]]:
    scenarios = fixture.get("scenarios")
    if isinstance(scenarios, list):
        return [s for s in scenarios if isinstance(s, dict) and s.get("production_smoke_safe_payload_template", True)]
    return [{}]


def check_result(
    fixture: dict[str, Any],
    scenario: dict[str, Any],
    status: int,
    request_id: str,
    row: dict[str, Any] | None,
) -> int:
    expected_status = scenario.get("expected_caller_status")
    expected_error_class = scenario.get("expected_error_class")
    if expected_status is not None:
        if status != int(expected_status):
            print(f"status {status} did not match expected {expected_status}", file=sys.stderr)
            return 1
    elif expected_error_class and status < 400:
        print(f"expected error class {expected_error_class} but API returned status {status}", file=sys.stderr)
        return 1
    elif status >= 400 and not expected_error_class:
        print(f"unexpected HTTP status {status}", file=sys.stderr)
        return 1
    if not request_id:
        print("missing X-Request-Id", file=sys.stderr)
        return 1
    if expected_error_class and not row:
        print("missing telemetry row for expected error scenario", file=sys.stderr)
        return 1
    if row:
        for denied in fixture.get("must_not_select", []) + scenario.get("must_not_select", []):
            if target_matches(row, denied):
                print("fixture selected a must-not-select target", file=sys.stderr)
                return 1
        allowed_targets = fixture.get("allowed_selected_targets", []) + scenario.get("allowed_selected_targets", [])
        if allowed_targets and not any(target_matches(row, allowed) for allowed in allowed_targets):
            print("fixture selected a target outside allowed_selected_targets", file=sys.stderr)
            return 1
        if expected_error_class and row.get("error_class") != expected_error_class:
            print("telemetry error class mismatch", file=sys.stderr)
            return 1
        request_bytes_bucket = fixture_value(fixture, scenario, "request_bytes_bucket")
        if request_bytes_bucket and str(row.get("total_request_bytes_bucket")) not in {"", "None", request_bytes_bucket}:
            print("request byte bucket mismatch", file=sys.stderr)
            return 1
        tool_schema_bucket = fixture_value(fixture, scenario, "tool_schema_bytes_bucket")
        if tool_schema_bucket and str(row.get("tool_schema_bytes_bucket")) not in {"", "None", tool_schema_bucket}:
            print("tool schema byte bucket mismatch", file=sys.stderr)
            return 1
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=["local", "prod"], default="prod")
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--model-group", default="")
    parser.add_argument("--fixture", default=DEFAULT_FIXTURE)
    parser.add_argument("--fixtures-dir", default=str(DEFAULT_FIXTURES_DIR))
    parser.add_argument("--token-env", default="ROUTER_TOKEN")
    parser.add_argument("--token-file", default="")
    parser.add_argument("--sqlite-db", default="")
    parser.add_argument("--postgres-dsn", default="")
    parser.add_argument("--psql-bin", default="psql")
    parser.add_argument("--timeout", type=float, default=180)
    args = parser.parse_args(argv)

    token = load_token(args)
    failures = 0
    results: list[dict[str, Any]] = []
    for path in fixture_paths(args):
        fixture = load_fixture(path)
        for scenario in scenario_list(fixture):
            model_group = args.model_group or str(fixture_value(fixture, scenario, "model_group"))
            endpoint, payload = make_payload(fixture, scenario, model_group)
            request_bytes = len(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
            status, headers, body = http_json(
                args.base_url.rstrip("/") + endpoint,
                token,
                payload,
                args.timeout,
            )
            request_id = request_id_from(headers, body)
            row = query_telemetry(args, request_id) if request_id else None
            failures += check_result(fixture, scenario, status, request_id, row)
            results.append(
                {
                    "fixture": fixture["name"],
                    "scenario": scenario.get("name", fixture["name"]),
                    "mode": args.mode,
                    "surface": fixture_value(fixture, scenario, "surface", "openai_chat"),
                    "status": status,
                    "requestId": request_id,
                    "requestBytes": request_bytes,
                    "messageCount": fixture_value(fixture, scenario, "message_count", 0),
                    "toolCount": fixture_value(fixture, scenario, "tool_count", 0),
                    "telemetry": row or {},
                }
            )
    print(json.dumps({"results": results}, indent=2, sort_keys=True))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
