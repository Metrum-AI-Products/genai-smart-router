# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Read-only relational usage import for LRP explore metadata.

Joins safe scalar columns from ``request_usage``, ``request_policy_executions``,
and selected ``request_attempts``. Usage and policy rows cannot reconstruct
prompts; operators must join governed content exports separately. This module
never issues INSERT/UPDATE/DELETE/DDL and opens SQLite/PostgreSQL
connections in read-only mode.
"""

from __future__ import annotations

import importlib
import re
import sqlite3
from collections.abc import Iterator, Mapping, Sequence
from pathlib import Path
from typing import Any, cast
from urllib.parse import unquote, urlparse

from lrp.collect import (
    DataError,
    append_row,
    canonical,
    journal,
    protected_path,
    read_rows,
)

EXPLORE_CLASS_LABEL = "lrp:explore"
MAX_IMPORT_ROWS = 100_000

# Explicit projection only. Never SELECT *.
_EXPLORE_METADATA_SQL = """
SELECT
  u.request_id AS request_id,
  u.ts AS ts,
  u.resolved_group AS resolved_group,
  u.inbound_dialect AS inbound_dialect,
  u.caller_project AS caller_project,
  u.caller_environment AS caller_environment,
  u.client AS client,
  u.strategy AS strategy,
  u.target_provider AS target_provider,
  u.target_model AS target_model,
  u.target_dialect AS target_dialect,
  u.status AS status,
  u.attempts AS attempts,
  u.fallback_used AS fallback_used,
  u.latency_ms AS latency_ms,
  u.input_tokens AS input_tokens,
  u.output_tokens AS output_tokens,
  u.total_tokens AS total_tokens,
  u.total_cost_usd AS total_cost_usd,
  u.pricing_source AS pricing_source,
  p.seq AS policy_seq,
  p.policy_kind AS policy_kind,
  p.outcome AS policy_outcome,
  p.duration_ms AS policy_duration_ms,
  p.eligible_target_count AS eligible_target_count,
  p.all_target_count AS all_target_count,
  p.selected_candidate_index AS selected_candidate_index,
  p.fallback_count AS policy_fallback_count,
  p.class_label AS class_label,
  p.error_class AS policy_error_class,
  a.provider AS selected_attempt_provider,
  a.model AS selected_attempt_model,
  a.dialect AS selected_attempt_dialect,
  a.status_code AS selected_attempt_status_code,
  a.duration_ms AS selected_attempt_duration_ms,
  a.error_class AS selected_attempt_error_class
FROM request_usage AS u
INNER JOIN request_policy_executions AS p
  ON p.request_id = u.request_id
LEFT JOIN request_attempts AS a
  ON a.request_id = u.request_id AND a.selected = TRUE
WHERE p.class_label = ?
  AND p.seq = (
    SELECT MAX(p2.seq)
    FROM request_policy_executions AS p2
    WHERE p2.request_id = u.request_id
      AND p2.class_label = ?
  )
ORDER BY u.ts ASC, u.request_id ASC
"""

_FORBIDDEN_OUTPUT_KEYS = frozenset(
    {
        "token_id",
        "token_hash",
        "caller_id",
        "caller_user",
        "caller_ip",
        "messages",
        "input",
        "system",
        "tools",
        "prompt",
        "content",
        "authorization",
        "api_key",
        "provider_key",
        "password",
        "secret",
        "encrypted_content",
        "nonce",
        "ciphertext",
    }
)

_WRITE_SQL = re.compile(
    r"^\s*(INSERT|UPDATE|DELETE|REPLACE|MERGE|CREATE|DROP|ALTER|TRUNCATE|GRANT|REVOKE|"
    r"COPY|VACUUM|ATTACH|DETACH|PRAGMA\s+writable|REINDEX)\b",
    re.IGNORECASE,
)


class UsageImportError(DataError):
    """Fixed safe classifications for usage-import failures."""


def explore_metadata_sql(*, dialect: str = "sqlite") -> str:
    """Return the explore-metadata join SQL for SQLite or PostgreSQL placeholders."""
    sql = _EXPLORE_METADATA_SQL.strip()
    if dialect == "sqlite":
        return sql
    if dialect == "postgres":
        return sql.replace("?", "%s")
    raise UsageImportError("unsupported_sql_dialect")


def parse_dsn(dsn: str) -> tuple[str, str]:
    """Return ``(dialect, target)`` for a path or URI. Never logs credentials."""
    value = dsn.strip()
    if not value:
        raise UsageImportError("invalid_usage_dsn")
    if "://" not in value:
        return "sqlite", value
    parsed = urlparse(value)
    scheme = (parsed.scheme or "").lower()
    if scheme in ("sqlite", "file"):
        # sqlite:///abs/path and sqlite:////abs/path both appear in operator notes.
        remainder = unquote(value.split("://", 1)[1])
        if remainder.startswith("//"):
            path = remainder[1:]
        elif parsed.netloc and scheme == "file":
            path = f"/{parsed.netloc}{unquote(parsed.path or '')}"
        else:
            path = unquote(parsed.path or remainder)
        if not path or path == "/":
            raise UsageImportError("invalid_usage_dsn")
        return "sqlite", path
    if scheme in ("postgresql", "postgres"):
        return "postgres", value
    raise UsageImportError("unsupported_usage_dsn")


class _ReadOnlyConnection:
    """Reject mutating SQL even if a caller bypasses the open helpers."""

    def __init__(self, connection: Any) -> None:
        self._connection = connection

    def cursor(self) -> _ReadOnlyCursor:
        return _ReadOnlyCursor(self._connection.cursor())

    def execute(self, sql: str, params: Sequence[Any] = ()) -> Any:
        _reject_write(sql)
        return self._connection.execute(sql, params)

    def close(self) -> None:
        self._connection.close()


class _ReadOnlyCursor:
    def __init__(self, cursor: Any) -> None:
        self._cursor = cursor

    @property
    def description(self) -> Sequence[Sequence[Any]] | None:
        return cast(Sequence[Sequence[Any]] | None, self._cursor.description)

    def execute(self, sql: str, params: Sequence[Any] = ()) -> Any:
        _reject_write(sql)
        return self._cursor.execute(sql, params)

    def fetchall(self) -> Sequence[Sequence[Any]]:
        return cast(Sequence[Sequence[Any]], self._cursor.fetchall())

    def close(self) -> None:
        self._cursor.close()


def _reject_write(sql: str) -> None:
    if _WRITE_SQL.search(sql):
        raise UsageImportError("usage_db_writes_forbidden")


def open_readonly(dsn: str) -> _ReadOnlyConnection:
    """Open a usage database without write capability."""
    dialect, target = parse_dsn(dsn)
    if dialect == "sqlite":
        # Fixture DBs may live under tests/fixtures; operator DBs must be outside
        # the repository (enforced by protected_path).
        path = protected_path(Path(target), fixture_read=True)
        uri = path.resolve().as_uri() + "?mode=ro"
        try:
            raw = sqlite3.connect(uri, uri=True)
        except sqlite3.Error as exc:
            raise UsageImportError("usage_db_open_failed") from exc
        raw.row_factory = None
        try:
            raw.execute("PRAGMA query_only = ON")
        except sqlite3.Error as exc:
            raw.close()
            raise UsageImportError("usage_db_open_failed") from exc
        return _ReadOnlyConnection(raw)
    try:
        psycopg = importlib.import_module("psycopg")
    except ImportError as exc:
        raise UsageImportError("postgres_driver_required") from exc
    try:
        raw = psycopg.connect(target, options="-c default_transaction_read_only=on")
    except Exception as exc:
        raise UsageImportError("usage_db_open_failed") from exc
    return _ReadOnlyConnection(raw)


def _scalar(value: Any) -> Any:
    if isinstance(value, memoryview):
        raise UsageImportError("unsafe_usage_column_type")
    if isinstance(value, (bytes, bytearray)):
        raise UsageImportError("unsafe_usage_column_type")
    if isinstance(value, bool) or value is None:
        return value
    if isinstance(value, (int, float, str)):
        return value
    # Decimal and driver-specific numerics.
    if hasattr(value, "as_integer_ratio") and not isinstance(value, bool):
        return float(value)
    raise UsageImportError("unsafe_usage_column_type")


def _row_dict(columns: Sequence[str], values: Sequence[Any]) -> dict[str, Any]:
    row: dict[str, Any] = {}
    for name, value in zip(columns, values, strict=True):
        key = str(name)
        lowered = key.lower()
        if lowered in _FORBIDDEN_OUTPUT_KEYS or any(
            part in lowered for part in ("token_hash", "prompt", "cipher", "secret")
        ):
            raise UsageImportError("forbidden_usage_column")
        row[key] = _scalar(value)
    row["content_join_required"] = True
    row["schema_version"] = "lrp.usage_explore.v1"
    return row


def fetch_explore_metadata(
    connection: _ReadOnlyConnection,
    *,
    class_label: str = EXPLORE_CLASS_LABEL,
    dialect: str = "sqlite",
    limit: int = MAX_IMPORT_ROWS,
) -> list[dict[str, Any]]:
    """Run the read-only explore join and return safe scalar dictionaries."""
    if not isinstance(class_label, str) or not class_label or len(class_label) > 64:
        raise UsageImportError("invalid_class_label")
    if limit < 1 or limit > MAX_IMPORT_ROWS:
        raise UsageImportError("dataset_limit")
    sql = explore_metadata_sql(dialect=dialect)
    cursor = connection.cursor()
    try:
        cursor.execute(sql, (class_label, class_label))
        description = cursor.description
        if not description:
            raise UsageImportError("usage_query_failed")
        columns = [str(col[0]) for col in description]
        rows = cursor.fetchall()
    except UsageImportError:
        raise
    except Exception as exc:
        raise UsageImportError("usage_query_failed") from exc
    finally:
        cursor.close()
    if len(rows) > limit:
        raise UsageImportError("dataset_limit")
    return [_row_dict(columns, row) for row in rows]


def content_request_ids(path: Path) -> set[str]:
    """Load request IDs from a governed content export or ID list (JSONL)."""
    ids: set[str] = set()
    for raw in read_rows(path):
        rid = raw.get("request_id")
        if not isinstance(rid, str) or not rid or len(rid) > 256:
            raise UsageImportError("invalid_content_request_id")
        if rid in ids:
            raise UsageImportError("duplicate_content_request_id")
        ids.add(rid)
        if len(ids) > MAX_IMPORT_ROWS:
            raise UsageImportError("dataset_limit")
    return ids


def metadata_for_collect(rows: Sequence[Mapping[str, Any]]) -> dict[str, dict[str, Any]]:
    """Project import rows into the safe keys ``collect`` accepts from router logs."""
    logs: dict[str, dict[str, Any]] = {}
    for raw in rows:
        rid = raw.get("request_id")
        if not isinstance(rid, str) or not rid or len(rid) > 256:
            raise UsageImportError("invalid_log_request_id")
        if rid in logs:
            raise UsageImportError("duplicate_log_request_id")
        logs[rid] = {
            key: raw[key]
            for key in (
                "ts",
                "resolved_group",
                "inbound_dialect",
                "caller_project",
                "caller_environment",
            )
            if key in raw
        }
    return logs


def iter_usage_log_metadata(dsn: str, *, class_label: str = EXPLORE_CLASS_LABEL) -> Iterator[dict[str, Any]]:
    """Yield collect-compatible metadata rows from a read-only usage DSN."""
    dialect, _ = parse_dsn(dsn)
    connection = open_readonly(dsn)
    try:
        for row in fetch_explore_metadata(connection, class_label=class_label, dialect=dialect):
            projected = metadata_for_collect([row])[str(row["request_id"])]
            projected["request_id"] = row["request_id"]
            yield projected
    finally:
        connection.close()


def import_usage(
    *,
    dsn: str,
    out: Path,
    class_label: str = EXPLORE_CLASS_LABEL,
    content_ids: Path | None = None,
) -> dict[str, int]:
    """Export explore metadata JSONL. Never invents prompts or writes to the DB."""
    dialect, _ = parse_dsn(dsn)
    connection = open_readonly(dsn)
    try:
        rows = fetch_explore_metadata(connection, class_label=class_label, dialect=dialect)
    finally:
        connection.close()

    allowed: set[str] | None = None
    if content_ids is not None:
        allowed = content_request_ids(content_ids)

    stats = {
        "written": 0,
        "skipped": 0,
        "explore_rows": len(rows),
        "missing_content": 0,
    }
    if allowed is not None:
        explore_ids = {str(row["request_id"]) for row in rows}
        stats["missing_content"] = len(explore_ids - allowed)

    with journal(out) as handle:
        existing = _load_existing_metadata(out)
        for row in rows:
            rid = str(row["request_id"])
            if allowed is not None and rid not in allowed:
                continue
            # Defense in depth: never persist content-bearing keys.
            if set(row) & _FORBIDDEN_OUTPUT_KEYS:
                raise UsageImportError("forbidden_usage_column")
            if rid in existing:
                if canonical(existing[rid]) != canonical(row):
                    raise UsageImportError("resume_input_changed")
                stats["skipped"] += 1
                continue
            payload = dict(row)
            append_row(handle, payload)
            existing[rid] = payload
            stats["written"] += 1
            if len(existing) > MAX_IMPORT_ROWS:
                raise UsageImportError("dataset_limit")
    return stats


def _load_existing_metadata(path: Path) -> dict[str, dict[str, Any]]:
    existing: dict[str, dict[str, Any]] = {}
    if not path.exists() or path.stat().st_size == 0:
        return existing
    for raw in read_rows(path):
        if raw.get("schema_version") != "lrp.usage_explore.v1":
            raise UsageImportError("invalid_usage_metadata_schema")
        rid = raw.get("request_id")
        if not isinstance(rid, str) or not rid:
            raise UsageImportError("invalid_log_request_id")
        if set(raw) & _FORBIDDEN_OUTPUT_KEYS:
            raise UsageImportError("forbidden_usage_column")
        if rid in existing:
            raise UsageImportError("duplicate_record_identity")
        existing[rid] = raw
    return existing
