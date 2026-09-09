# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for read-only relational usage import (#34)."""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path

import pytest
from lrp.collect import collect
from lrp.usage_import import (
    EXPLORE_CLASS_LABEL,
    UsageImportError,
    explore_metadata_sql,
    fetch_explore_metadata,
    import_usage,
    open_readonly,
    parse_dsn,
)

SCHEMA = """
CREATE TABLE request_usage (
  request_id TEXT PRIMARY KEY,
  ts TEXT NOT NULL,
  resolved_group TEXT NOT NULL,
  inbound_dialect TEXT NOT NULL,
  caller_project TEXT NOT NULL,
  caller_environment TEXT NOT NULL,
  client TEXT NOT NULL,
  strategy TEXT NOT NULL,
  target_provider TEXT NOT NULL,
  target_model TEXT NOT NULL,
  target_dialect TEXT NOT NULL,
  status INTEGER NOT NULL,
  attempts INTEGER NOT NULL,
  fallback_used INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL,
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  total_tokens INTEGER NOT NULL,
  total_cost_usd REAL NOT NULL,
  pricing_source TEXT NOT NULL,
  token_id TEXT NOT NULL,
  caller_id TEXT NOT NULL,
  caller_ip TEXT NOT NULL
);
CREATE TABLE request_policy_executions (
  request_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  strategy TEXT NOT NULL,
  policy_kind TEXT NOT NULL,
  outcome TEXT NOT NULL,
  duration_ms INTEGER NOT NULL,
  eligible_target_count INTEGER NOT NULL,
  all_target_count INTEGER NOT NULL,
  selected_candidate_index INTEGER NOT NULL,
  fallback_count INTEGER NOT NULL,
  class_label TEXT NOT NULL,
  error_class TEXT NOT NULL,
  PRIMARY KEY (request_id, seq)
);
CREATE TABLE request_attempts (
  request_id TEXT NOT NULL,
  attempt_index INTEGER NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dialect TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  duration_ms INTEGER NOT NULL,
  error_class TEXT NOT NULL,
  selected INTEGER NOT NULL,
  PRIMARY KEY (request_id, attempt_index)
);
"""


def _seed(db: Path) -> None:
    conn = sqlite3.connect(db)
    conn.executescript(SCHEMA)
    conn.execute(
        """
        INSERT INTO request_usage VALUES (
          'explore-1', '2026-09-09T00:00:00Z', 'demo-group', 'openai-chat',
          'demo', 'staging', 'codex', 'external', 'mock', 'mock-strong',
          'openai-chat', 200, 1, 0, 42, 10, 4, 14, 0.001, 'fixture',
          'rtr_secret_token_id', 'caller-secret', '203.0.113.10'
        )
        """
    )
    conn.execute(
        """
        INSERT INTO request_usage VALUES (
          'baseline-1', '2026-09-09T00:01:00Z', 'demo-group', 'openai-chat',
          'demo', 'staging', 'codex', 'external', 'mock', 'mock-cheap',
          'openai-chat', 200, 1, 0, 20, 8, 2, 10, 0.0002, 'fixture',
          'rtr_other', 'caller-other', '203.0.113.11'
        )
        """
    )
    conn.execute(
        """
        INSERT INTO request_policy_executions VALUES
          ('explore-1', 0, 'external', 'external', 'selected', 3, 2, 2, 1, 0,
           'lrp:cheapest-above-floor', ''),
          ('explore-1', 1, 'external', 'external', 'selected', 4, 2, 2, 0, 0,
           'lrp:explore', ''),
          ('baseline-1', 0, 'external', 'external', 'selected', 2, 2, 2, 0, 0,
           'lrp:cheapest-above-floor', '')
        """
    )
    conn.execute(
        """
        INSERT INTO request_attempts VALUES
          ('explore-1', 0, 'mock', 'mock-strong', 'openai-chat', 200, 40, '', 1),
          ('explore-1', 1, 'mock', 'mock-cheap', 'openai-chat', 500, 5, 'upstream_failed', 0),
          ('baseline-1', 0, 'mock', 'mock-cheap', 'openai-chat', 200, 15, '', 1)
        """
    )
    conn.commit()
    conn.close()
    db.chmod(0o600)


def _write_jsonl(path: Path, rows: list[dict]) -> Path:
    path.write_text("".join(json.dumps(row) + "\n" for row in rows))
    path.chmod(0o600)
    return path


def test_sql_placeholder_parity_sqlite_and_postgres():
    sqlite_sql = explore_metadata_sql(dialect="sqlite")
    postgres_sql = explore_metadata_sql(dialect="postgres")
    assert "?" in sqlite_sql
    assert "%s" in postgres_sql
    assert sqlite_sql.replace("?", "%s") == postgres_sql
    assert "token_id" not in sqlite_sql
    assert "SELECT *" not in sqlite_sql.upper()
    assert "a.selected = TRUE" in sqlite_sql


def test_parse_dsn_sqlite_and_postgres():
    assert parse_dsn("/var/tmp/usage.db") == ("sqlite", "/var/tmp/usage.db")
    assert parse_dsn("sqlite:////var/tmp/usage.db")[0] == "sqlite"
    assert parse_dsn("postgresql://user:pass@127.0.0.1:5432/usage")[0] == "postgres"
    with pytest.raises(UsageImportError, match="unsupported_usage_dsn"):
        parse_dsn("mysql://example.test/db")


def test_import_explore_rows_only_safe_scalars(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    out = tmp_path / "explore.ndjson"
    stats = import_usage(dsn=str(db), out=out)
    assert stats == {
        "written": 1,
        "skipped": 0,
        "explore_rows": 1,
        "missing_content": 0,
    }
    row = json.loads(out.read_text().splitlines()[0])
    assert row["request_id"] == "explore-1"
    assert row["class_label"] == EXPLORE_CLASS_LABEL
    assert row["policy_seq"] == 1
    assert row["selected_attempt_model"] == "mock-strong"
    assert row["content_join_required"] is True
    assert row["schema_version"] == "lrp.usage_explore.v1"
    text = out.read_text()
    for forbidden in (
        "rtr_secret_token_id",
        "caller-secret",
        "203.0.113.10",
        "messages",
        "prompt",
        "What is",
    ):
        assert forbidden not in text


def test_readonly_rejects_writes(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    connection = open_readonly(str(db))
    try:
        with pytest.raises(UsageImportError, match="writes_forbidden"):
            connection.execute("DELETE FROM request_usage")
        with pytest.raises(UsageImportError, match="writes_forbidden"):
            connection.execute("INSERT INTO request_usage(request_id) VALUES ('x')")
    finally:
        connection.close()


def test_content_ids_filter_and_missing_count(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    content = _write_jsonl(
        tmp_path / "content.ndjson",
        [{"request_id": "unrelated", "source": "synthetic"}],
    )
    out = tmp_path / "explore.ndjson"
    stats = import_usage(dsn=str(db), out=out, content_ids=content)
    assert stats["written"] == 0
    assert stats["explore_rows"] == 1
    assert stats["missing_content"] == 1
    assert out.read_bytes() == b""

    content = _write_jsonl(
        tmp_path / "content2.ndjson",
        [{"request_id": "explore-1", "source": "content_capture"}],
    )
    stats = import_usage(dsn=str(db), out=out, content_ids=content)
    assert stats["written"] == 1
    assert stats["missing_content"] == 0


def test_usage_metadata_cannot_reconstruct_prompts(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    out = tmp_path / "requests.ndjson"
    stats = collect(usage_db=str(db), out=out)
    assert stats == {"written": 0, "skipped": 0, "missing_content": 1}
    assert out.read_bytes() == b""


def test_collect_joins_usage_db_with_governed_content(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    source = _write_jsonl(
        tmp_path / "content.ndjson",
        [
            {
                "schema_version": "lrp.request.v1",
                "request_id": "explore-1",
                "source": "content_capture",
                "messages": [{"role": "user", "content": "What is 2+2?"}],
                "max_tokens": 32,
            }
        ],
    )
    out = tmp_path / "requests.ndjson"
    stats = collect(
        usage_db=str(db),
        content_capture=source,
        out=out,
        approved_content=True,
    )
    assert stats["written"] == 1
    row = json.loads(out.read_text().splitlines()[0])
    assert row["group"] == "demo-group"
    assert row["dialect"] == "openai-chat"
    assert row["caller"] == {"project": "demo", "environment": "staging"}
    assert "rtr_secret" not in out.read_text()


def test_resume_is_idempotent(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    out = tmp_path / "explore.ndjson"
    assert import_usage(dsn=str(db), out=out)["written"] == 1
    assert import_usage(dsn=str(db), out=out)["skipped"] == 1


def test_fetch_uses_max_explore_policy_seq(tmp_path: Path):
    db = tmp_path / "usage.db"
    _seed(db)
    connection = open_readonly(str(db))
    try:
        rows = fetch_explore_metadata(connection)
    finally:
        connection.close()
    assert len(rows) == 1
    assert rows[0]["policy_seq"] == 1


def test_cli_import_usage_parser():
    from lrp.cli import parser

    args = parser().parse_args(
        [
            "import-usage",
            "--db",
            "/var/tmp/usage.db",
            "--out",
            "/var/tmp/explore.ndjson",
            "--class-label",
            "lrp:explore",
        ]
    )
    assert args.command == "import-usage"
    assert args.db == "/var/tmp/usage.db"
    args = parser().parse_args(
        ["collect", "--usage-db", "/var/tmp/usage.db", "--out", "/var/tmp/out.ndjson"]
    )
    assert args.usage_db == "/var/tmp/usage.db"
