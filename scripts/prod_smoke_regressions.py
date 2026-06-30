#!/usr/bin/env python3
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


def load_fixture(args: argparse.Namespace) -> dict[str, Any]:
    path = Path(args.fixtures_dir) / args.fixture
    return json.loads(path.read_text(encoding="utf-8"))


def make_tool(index: int) -> dict[str, Any]:
    pad = "safe schema description " * 42
    return {
        "type": "function",
        "function": {
            "name": f"fixture_tool_{index:02d}",
            "description": pad,
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


def make_payload(fixture: dict[str, Any], model_group: str) -> dict[str, Any]:
    message_count = int(fixture["message_count"])
    tool_count = int(fixture["tool_count"])
    filler = "production-derived-safe-filler " * 62
    messages: list[dict[str, str]] = [
        {
            "role": "system",
            "content": "Synthetic production-derived large coding-agent fixture.",
        }
    ]
    for i in range(1, message_count):
        role = "assistant" if i % 2 == 0 else "user"
        messages.append({"role": role, "content": f"{filler} turn {i}"})
    return {
        "model": model_group,
        "stream": bool(fixture["stream"]),
        "messages": messages,
        "tools": [make_tool(i) for i in range(tool_count)],
        "tool_choice": "auto",
        "metadata": {"fixture": fixture["name"]},
    }


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


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
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
    args = parser.parse_args()

    fixture = load_fixture(args)
    model_group = args.model_group or str(fixture["model_group"])
    payload = make_payload(fixture, model_group)
    request_bytes = len(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
    status, headers, body = http_json(
        args.base_url.rstrip("/") + "/v1/chat/completions",
        load_token(args),
        payload,
        args.timeout,
    )
    request_id = request_id_from(headers, body)
    row = query_telemetry(args, request_id) if request_id else None

    result = {
        "fixture": fixture["name"],
        "status": status,
        "requestId": request_id,
        "requestBytes": request_bytes,
        "messageCount": fixture["message_count"],
        "toolCount": fixture["tool_count"],
        "telemetry": row or {},
    }
    print(json.dumps(result, indent=2, sort_keys=True))

    if status >= 400:
        return 1
    if not request_id:
        print("missing X-Request-Id", file=sys.stderr)
        return 1
    if row:
        for denied in fixture.get("must_not_select", []):
            if (
                row.get("provider") == denied.get("provider")
                and row.get("model") == denied.get("model")
                and row.get("dialect") == denied.get("dialect")
                and normalize_bool(row.get("selected"))
            ):
                print("fixture selected a must-not-select target", file=sys.stderr)
                return 1
        if str(row.get("total_request_bytes_bucket")) != fixture["request_bytes_bucket"]:
            print("request byte bucket mismatch", file=sys.stderr)
            return 1
        if str(row.get("tool_schema_bytes_bucket")) != fixture["tool_schema_bytes_bucket"]:
            print("tool schema byte bucket mismatch", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
