#!/usr/bin/env python3
"""Production/staging reasoning proof smoke for GenAI Smart Router.

The script sends small safe requests through each API surface, checks that
`/v1/models` advertises reasoning metadata for the requested model group, and
verifies usage DB translation telemetry for every returned request ID.
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any


DEFAULT_MODEL_GROUP = "reasoning-smoke"
DEFAULT_SURFACES = ("chat", "responses", "anthropic")
EXPECTED_REASONING_CONTROL = {
    "chat": "reasoning_effort",
    "responses": "reasoning",
    "anthropic": "thinking",
}


@dataclass(frozen=True)
class SurfaceResult:
    surface: str
    status: int
    request_id: str
    provider: str = ""
    model: str = ""
    dialect: str = ""
    translated_reasoning_control: str = ""
    fallback_used: bool = False


def load_token(args: argparse.Namespace) -> str:
    if args.token_file:
        token = Path(args.token_file).read_text(encoding="utf-8").strip()
    else:
        token = os.environ.get(args.token_env, "").strip()
    if not token:
        raise SystemExit(
            f"missing router token; set {args.token_env} or pass --token-file"
        )
    return token


def http_json(
    method: str,
    url: str,
    token: str,
    payload: dict[str, Any] | None = None,
    timeout: float = 30,
    extra_headers: dict[str, str] | None = None,
) -> tuple[int, dict[str, str], Any]:
    body = None
    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/json",
        "User-Agent": "smart-llmrouter-reasoning-smoke",
    }
    if payload is not None:
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if extra_headers:
        headers.update(extra_headers)
    req = urllib.request.Request(url, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read(2 * 1024 * 1024)
            parsed = json.loads(raw.decode("utf-8", errors="replace")) if raw else {}
            return resp.status, dict(resp.headers.items()), parsed
    except urllib.error.HTTPError as exc:
        raw = exc.read(256 * 1024)
        parsed: Any
        try:
            parsed = json.loads(raw.decode("utf-8", errors="replace")) if raw else {}
        except json.JSONDecodeError:
            parsed = {"bodyClass": "non-json", "bodyBytes": len(raw)}
        return exc.code, dict(exc.headers.items()), parsed


def find_model(models_body: Any, model_group: str) -> dict[str, Any]:
    if not isinstance(models_body, dict) or not isinstance(models_body.get("data"), list):
        raise RuntimeError("/v1/models did not return an OpenAI-style model list")
    for item in models_body["data"]:
        if isinstance(item, dict) and item.get("id") == model_group:
            return item
    raise RuntimeError(f"/v1/models did not include model group {model_group!r}")


def assert_reasoning_metadata(model: dict[str, Any], model_group: str) -> None:
    levels = model.get("supported_reasoning_levels")
    if not isinstance(levels, list) or not levels:
        raise RuntimeError(
            f"/v1/models model {model_group!r} did not advertise supported_reasoning_levels"
        )


def content_text(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        parts: list[str] = []
        for item in value:
            if isinstance(item, str):
                parts.append(item)
            elif isinstance(item, dict):
                text = item.get("text") or item.get("content")
                if isinstance(text, str):
                    parts.append(text)
        return "".join(parts)
    return ""


def visible_response_text(surface: str, body: Any) -> str:
    if not isinstance(body, dict):
        return ""
    if surface == "chat":
        choices = body.get("choices")
        if isinstance(choices, list) and choices:
            first = choices[0]
            if isinstance(first, dict):
                message = first.get("message")
                if isinstance(message, dict):
                    return content_text(message.get("content"))
        return ""
    if surface == "responses":
        output_text = body.get("output_text")
        if isinstance(output_text, str):
            return output_text
        parts: list[str] = []
        output = body.get("output")
        if isinstance(output, list):
            for item in output:
                if not isinstance(item, dict):
                    continue
                content = item.get("content")
                if isinstance(content, list):
                    for part in content:
                        if isinstance(part, dict):
                            text = part.get("text")
                            if isinstance(text, str):
                                parts.append(text)
        return "".join(parts)
    if surface == "anthropic":
        return content_text(body.get("content"))
    return ""


def assert_expected_response_text(surface: str, body: Any) -> None:
    text = visible_response_text(surface, body).strip()
    if text.upper() != "OK":
        raise RuntimeError(f"{surface} smoke returned visible text {text!r}, want 'OK'")


def surface_payload(surface: str, model_group: str) -> tuple[str, dict[str, Any], dict[str, str]]:
    if surface == "chat":
        return (
            "/v1/chat/completions",
            {
                "model": model_group,
                "messages": [{"role": "user", "content": "Reply OK only."}],
                "reasoning_effort": "low",
                "max_tokens": 256,
                "stream": False,
            },
            {},
        )
    if surface == "responses":
        return (
            "/v1/responses",
            {
                "model": model_group,
                "input": "Reply OK only.",
                "reasoning": {"effort": "low"},
                "max_output_tokens": 256,
                "stream": False,
            },
            {},
        )
    if surface == "anthropic":
        return (
            "/v1/messages",
            {
                "model": model_group,
                "max_tokens": 1024,
                "thinking": {"type": "enabled", "budget_tokens": 512},
                "messages": [{"role": "user", "content": "Reply OK only."}],
            },
            {"anthropic-version": "2023-06-01"},
        )
    raise ValueError(f"unsupported surface {surface!r}")


def request_id_from(headers: dict[str, str], body: Any) -> str:
    for key, value in headers.items():
        if key.lower() == "x-request-id" and value.strip():
            return value.strip()
    if isinstance(body, dict):
        value = body.get("request_id") or body.get("requestId")
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def run_surface(
    base_url: str,
    token: str,
    model_group: str,
    surface: str,
    timeout: float,
) -> SurfaceResult:
    path, payload, headers = surface_payload(surface, model_group)
    status, response_headers, body = http_json(
        "POST",
        base_url.rstrip("/") + path,
        token,
        payload,
        timeout,
        headers,
    )
    request_id = request_id_from(response_headers, body)
    if status < 200 or status >= 300:
        raise RuntimeError(
            f"{surface} smoke failed with HTTP {status}; request_id={request_id or 'missing'}"
        )
    if not request_id:
        raise RuntimeError(f"{surface} smoke passed but did not return X-Request-Id")
    assert_expected_response_text(surface, body)
    return SurfaceResult(surface=surface, status=status, request_id=request_id)


def parse_expected(values: list[str]) -> dict[str, tuple[str, str, str]]:
    expected: dict[str, tuple[str, str, str]] = {}
    for value in values:
        parts = value.split(":", 3)
        if len(parts) != 4 or parts[0] not in DEFAULT_SURFACES:
            raise SystemExit(
                "--expect must be surface:provider:model:dialect, for example "
                "chat:fireworks:accounts/fireworks/models/gpt-oss-20b:openai-chat"
            )
        expected[parts[0]] = (parts[1], parts[2], parts[3])
    return expected


def telemetry_sql(request_id: str) -> tuple[str, list[str]]:
    return (
        """
SELECT
  u.request_id,
  u.resolved_group,
  u.status_code,
  u.fallback_used,
  a.provider,
  a.model,
  a.dialect,
  t.translated_reasoning_control
FROM request_usage u
JOIN request_attempts a ON a.request_id = u.request_id AND a.selected = TRUE
JOIN request_translation_shapes t ON t.request_id = u.request_id AND t.attempt_index = a.attempt_index
WHERE u.request_id = ?
LIMIT 1
""".strip(),
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
        "--no-psqlrc",
        "--quiet",
        "--set",
        f"request_id={request_id}",
        "--csv",
        "--command",
        pg_sql,
    ]
    proc = subprocess.run(cmd, check=False, text=True, capture_output=True)
    if proc.returncode != 0:
        message = (proc.stderr or "psql failed").strip().splitlines()[-1]
        raise RuntimeError(f"Postgres telemetry query failed: {message}")
    rows = list(csv.DictReader(proc.stdout.splitlines()))
    return rows[0] if rows else None


def query_telemetry(args: argparse.Namespace, request_id: str) -> dict[str, Any] | None:
    if args.sqlite_db:
        return query_sqlite(args.sqlite_db, request_id)
    if args.postgres_dsn:
        return query_postgres(args.psql_bin, args.postgres_dsn, request_id)
    raise RuntimeError("telemetry proof requires --sqlite-db or --postgres-dsn")


def boolish(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "t", "true", "yes", "y"}


def verify_telemetry(
    args: argparse.Namespace,
    result: SurfaceResult,
    expected: dict[str, tuple[str, str, str]],
) -> SurfaceResult:
    deadline = time.monotonic() + args.telemetry_timeout
    row = None
    while time.monotonic() <= deadline:
        row = query_telemetry(args, result.request_id)
        if row:
            break
        time.sleep(args.telemetry_poll_interval)
    if not row:
        raise RuntimeError(
            f"{result.surface} telemetry missing for request_id={result.request_id}"
        )
    expected_control = EXPECTED_REASONING_CONTROL[result.surface]
    got_control = str(row.get("translated_reasoning_control") or "")
    if got_control != expected_control:
        raise RuntimeError(
            f"{result.surface} translated_reasoning_control={got_control!r}, "
            f"want {expected_control!r}"
        )
    if boolish(row.get("fallback_used")):
        raise RuntimeError(f"{result.surface} used fallback; expected selected primary target")
    if int(row.get("status_code") or 0) < 200 or int(row.get("status_code") or 0) >= 300:
        raise RuntimeError(f"{result.surface} telemetry status_code={row.get('status_code')}")
    provider = str(row.get("provider") or "")
    model = str(row.get("model") or "")
    dialect = str(row.get("dialect") or "")
    if result.surface in expected and (provider, model, dialect) != expected[result.surface]:
        raise RuntimeError(
            f"{result.surface} selected {(provider, model, dialect)!r}, "
            f"want {expected[result.surface]!r}"
        )
    return SurfaceResult(
        surface=result.surface,
        status=result.status,
        request_id=result.request_id,
        provider=provider,
        model=model,
        dialect=dialect,
        translated_reasoning_control=got_control,
        fallback_used=False,
    )


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Prove reasoning metadata, per-surface smokes, and usage DB translation telemetry."
    )
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--model", default=DEFAULT_MODEL_GROUP)
    parser.add_argument("--token-env", default="ROUTER_TOKEN")
    parser.add_argument("--token-file")
    parser.add_argument("--sqlite-db")
    parser.add_argument("--postgres-dsn")
    parser.add_argument("--psql-bin", default="psql")
    parser.add_argument("--surfaces", default=",".join(DEFAULT_SURFACES))
    parser.add_argument("--expect", action="append", default=[])
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--telemetry-timeout", type=float, default=45)
    parser.add_argument("--telemetry-poll-interval", type=float, default=2)
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    surfaces = [s.strip() for s in args.surfaces.split(",") if s.strip()]
    invalid = sorted(set(surfaces) - set(DEFAULT_SURFACES))
    if invalid:
        raise SystemExit(f"unsupported surfaces: {', '.join(invalid)}")
    if not args.sqlite_db and not args.postgres_dsn:
        raise SystemExit("pass --sqlite-db or --postgres-dsn for telemetry proof")
    token = load_token(args)
    expected = parse_expected(args.expect)

    status, _, models_body = http_json(
        "GET", args.base_url.rstrip("/") + "/v1/models", token, timeout=args.timeout
    )
    if status != 200:
        raise RuntimeError(f"/v1/models failed with HTTP {status}")
    model = find_model(models_body, args.model)
    assert_reasoning_metadata(model, args.model)

    print(
        json.dumps(
            {
                "event": "models_reasoning_metadata",
                "model": args.model,
                "defaultReasoningLevel": model.get("default_reasoning_level"),
                "supportedReasoningLevelCount": len(model.get("supported_reasoning_levels", [])),
            },
            sort_keys=True,
        )
    )

    for surface in surfaces:
        api_result = run_surface(args.base_url, token, args.model, surface, args.timeout)
        proof = verify_telemetry(args, api_result, expected)
        print(
            json.dumps(
                {
                    "event": "reasoning_surface_pass",
                    "surface": proof.surface,
                    "requestId": proof.request_id,
                    "provider": proof.provider,
                    "model": proof.model,
                    "dialect": proof.dialect,
                    "translatedReasoningControl": proof.translated_reasoning_control,
                    "fallbackUsed": proof.fallback_used,
                },
                sort_keys=True,
            )
        )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except RuntimeError as exc:
        print(json.dumps({"event": "reasoning_smoke_failed", "error": str(exc)}), file=sys.stderr)
        raise SystemExit(1)
