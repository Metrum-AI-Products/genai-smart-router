#!/usr/bin/env python3
"""Behavior tests for the printable work-dashboard snapshot."""

from __future__ import annotations

import json
import tempfile
from pathlib import Path

from work_dashboard_snapshot import render_html


TIMESTAMP = "2026-08-06T00:00:00Z"


def task(record_id: str, status: str, **fields: object) -> dict[str, object]:
    return {
        "kind": "task",
        "id": record_id,
        "title": str(fields.pop("title", record_id)),
        "description": str(fields.pop("description", f"Details for {record_id}")),
        "stream": "dashboard-test",
        "phase": 1,
        "priority": "normal",
        "status": status,
        "status_reason": fields.pop("status_reason", f"{record_id} has status {status}."),
        "next_steps": fields.pop("next_steps", [] if status in {"done", "cancelled"} else ["Continue work."]),
        "human_actions": fields.pop("human_actions", []),
        "commands": fields.pop("commands", []),
        "requires": [],
        "references": fields.pop("references", []),
        "assignee": None,
        "due_date": None,
        "started_at": None,
        "completed_at": TIMESTAMP if status == "done" else None,
        "created_at": TIMESTAMP,
        "updated_at": TIMESTAMP,
        **fields,
    }


def write_ndjson(path: Path, records: list[dict[str, object]]) -> None:
    path.write_text("".join(json.dumps(record) + "\n" for record in records), encoding="utf-8")


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="work-dashboard-snapshot-") as temporary:
        root = Path(temporary)
        registry = root / "work-items.ndjson"
        events = root / "work-item-events.ndjson"
        write_ndjson(
            registry,
            [
                {
                    "kind": "plan",
                    "id": "plan.dashboard-test",
                    "schema": "smart-llmrouter.work-items/v1",
                    "status": "active",
                    "due_date": None,
                    "created_at": TIMESTAMP,
                    "updated_at": TIMESTAMP,
                },
                task(
                    "task.active",
                    "blocked",
                    status_reason="Blocked by #769 and PR#770; see https://example.com/runbook.",
                    next_steps=["Resolve #771."],
                    human_actions=[],
                    commands=["rtk npm --prefix work-dashboard test", "rtk npm --prefix work-dashboard run build"],
                    references=["#773", "PR#770", "https://example.com/reference"],
                ),
                task("task.closed", "done", next_steps=[], human_actions=[]),
            ],
        )

        rendered = render_html(registry, events, "Snapshot behavior")

    assert "<strong>Commands</strong>" in rendered
    assert "rtk npm --prefix work-dashboard test" in rendered
    assert "rtk npm --prefix work-dashboard run build" in rendered
    assert "<strong>Why this status?</strong>" in rendered
    assert "<strong>What happens next?</strong>" in rendered
    assert "<strong>What can a human do?</strong>" in rendered
    assert "No next step is required; this task is closed." in rendered
    assert rendered.count("No direct human action is currently required.") == 2
    assert 'href="https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/769"' in rendered
    assert 'href="https://github.com/sysadmin-metrum-ai/genai-smart-router/pull/770"' in rendered
    assert 'href="https://example.com/runbook"' in rendered
    assert 'href="https://example.com/reference"' in rendered
    assert 'target="_blank" rel="noopener noreferrer"' in rendered
    print("work dashboard snapshot behavior tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
