#!/usr/bin/env python3
"""Self-test the revisioned work-item event journal and reconciled projection."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any


SCRIPT = Path(__file__).resolve().with_name("work_items.py")
SNAPSHOT_SCRIPT = Path(__file__).resolve().with_name("work_dashboard_snapshot.py")
TIMESTAMP = "2026-08-05T00:00:00Z"


def record(kind: str, record_id: str, status: str, **fields: Any) -> dict[str, Any]:
    return {
        "kind": kind,
        "id": record_id,
        **fields,
        "status": status,
        "due_date": None,
        "created_at": TIMESTAMP,
        "updated_at": TIMESTAMP,
    }


def task(record_id: str, status: str, **fields: Any) -> dict[str, Any]:
    return record(
        "task",
        record_id,
        status,
        title=fields.pop("title", record_id),
        description=fields.pop("description", f"Drilldown detail for {record_id}"),
        stream=fields.pop("stream", "test"),
        phase=fields.pop("phase", 1),
        priority=fields.pop("priority", "normal"),
        assignee=fields.pop("assignee", None),
        requires=fields.pop("requires", []),
        references=fields.pop("references", []),
        started_at=fields.pop("started_at", None),
        completed_at=fields.pop("completed_at", None),
        **fields,
    )


def valid_records() -> list[dict[str, Any]]:
    return [
        record("plan", "plan.work_items", "paused", schema="smart-llmrouter.work-items/v1"),
        record("gate", "gate.resume", "blocked"),
        task("task.first", "blocked", title="First task", priority="critical", requires=["gate.resume"]),
        task("task.second", "pending", title="Second task", phase=2, requires=["task.first"]),
    ]


def write_ndjson(path: Path, records: list[dict[str, Any]]) -> None:
    path.write_text("".join(json.dumps(item, separators=(",", ":")) + "\n" for item in records))


def run(registry: Path, events: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(SCRIPT), "--file", str(registry), "--events", str(events), *args],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    with tempfile.TemporaryDirectory() as temporary_directory:
        root = Path(temporary_directory)
        registry = root / "work-items.ndjson"
        events = root / "work-item-events.ndjson"
        projection = root / "projected.ndjson"
        write_ndjson(registry, valid_records())
        original_baseline = registry.read_bytes()

        initial = run(registry, events, "validate")
        require(initial.returncode == 0, initial.stderr)
        require("0 events" in initial.stdout, initial.stdout)
        require(not events.exists(), "validation created an event journal")

        update = run(
            registry,
            events,
            "update",
            "task.first",
            "--expect-status",
            "blocked",
            "--status",
            "in_progress",
            "--assignee",
            "quality-engineering",
        )
        require(update.returncode == 0, update.stderr)
        update_event = json.loads(update.stdout)
        require(update_event["event_type"] == "task.updated", "update did not emit task.updated")
        require(update_event["revision"] == 1 and update_event["base_revision"] == 0, "wrong update revision")
        require(update_event["status"] == "in_progress", "top-level event status missing")
        require(update_event["due_date"] is None, "top-level event due_date missing")
        require(update_event["task"]["assignee"] == "quality-engineering", "snapshot assignee missing")
        require(registry.read_bytes() == original_baseline, "update mutated baseline registry")

        listing = run(registry, events, "list", "--status", "in_progress", "--json")
        require(listing.returncode == 0, listing.stderr)
        listed = [json.loads(line) for line in listing.stdout.splitlines()]
        require([item["id"] for item in listed] == ["task.first"], f"unexpected reconciled list: {listed}")

        second_update = run(
            registry,
            events,
            "update",
            "task.first",
            "--expect-status",
            "in_progress",
            "--due-date",
            "2026-08-20",
        )
        require(second_update.returncode == 0, second_update.stderr)
        second_event = json.loads(second_update.stdout)
        require(second_event["revision"] == 2 and second_event["base_revision"] == 1, "revision did not advance")
        require(second_event["due_date"] == "2026-08-20", "due date not exposed on event")
        require(registry.read_bytes() == original_baseline, "second update mutated baseline registry")

        add = run(
            registry,
            events,
            "add",
            "task.dashboard",
            "--title",
            "Build dashboard",
            "--description",
            "Interactive work-item status, schedule, and reconciliation dashboard.",
            "--stream",
            "work-management",
            "--phase",
            "1",
            "--status",
            "ready",
            "--reference",
            "https://github.com/uwdata/mosaic",
        )
        require(add.returncode == 0, add.stderr)
        add_event = json.loads(add.stdout)
        require(add_event["event_type"] == "task.created", "add did not emit task.created")
        require(add_event["changed_fields"] == ["*"], "create event fields are not canonical")
        require(
            add_event["task"]["description"].startswith("Interactive work-item"),
            "create event omitted drilldown detail",
        )
        require(registry.read_bytes() == original_baseline, "add mutated baseline registry")

        projected = run(registry, events, "project", "--output", str(projection))
        require(projected.returncode == 0, projected.stderr)
        projected_records = [json.loads(line) for line in projection.read_text().splitlines()]
        projected_tasks = {item["id"]: item for item in projected_records if item["kind"] == "task"}
        require(projected_tasks["task.first"]["status"] == "in_progress", "projection missed update")
        require(projected_tasks["task.first"]["due_date"] == "2026-08-20", "projection missed second update")
        require(projected_tasks["task.dashboard"]["status"] == "ready", "projection missed create")
        require(registry.read_bytes() == original_baseline, "projection mutated baseline registry")
        snapshot = root / "work-dashboard.html"
        snapshot_result = subprocess.run(
            [
                sys.executable,
                str(SNAPSHOT_SCRIPT),
                "--file",
                str(registry),
                "--events",
                str(events),
                "--output",
                str(snapshot),
                "--title",
                "Status <Work>",
            ],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        require(snapshot_result.returncode == 0, snapshot_result.stderr)
        snapshot_html = snapshot.read_text()
        require("Status &lt;Work&gt;" in snapshot_html, "snapshot title was not escaped")
        require("Interactive work-item status" in snapshot_html, "snapshot omitted task description")
        require("gate.resume" in snapshot_html, "snapshot omitted task dependency")
        require("task.dashboard" in snapshot_html, "snapshot omitted event-created task")

        event_lines = events.read_text().splitlines()
        require(len(event_lines) == 3, f"expected three events, found {len(event_lines)}")

        before_stale = events.read_bytes()
        stale = run(
            registry,
            events,
            "update",
            "task.first",
            "--expect-status",
            "blocked",
            "--status",
            "done",
        )
        require(stale.returncode != 0, "stale status update succeeded")
        require("expected status" in stale.stderr, f"missing stale status error: {stale.stderr}")
        require(events.read_bytes() == before_stale, "stale update changed event journal")

        duplicate_add = run(
            registry,
            events,
            "add",
            "task.dashboard",
            "--title",
            "Duplicate",
            "--description",
            "Duplicate task must be rejected.",
            "--stream",
            "test",
            "--phase",
            "1",
        )
        require(duplicate_add.returncode != 0, "duplicate add succeeded")
        require(events.read_bytes() == before_stale, "duplicate add changed event journal")

        invalid_due = run(registry, events, "update", "task.second", "--due-date", "2026-02-30")
        require(invalid_due.returncode != 0, "invalid due date succeeded")
        require(events.read_bytes() == before_stale, "invalid due date changed event journal")

        conflicting = dict(second_event)
        conflicting["event_id"] = "conflicting-external-event"
        conflicting["task"] = dict(second_event["task"])
        conflicting["task"]["due_date"] = "2026-08-21"
        conflicting["due_date"] = "2026-08-21"
        with events.open("a", encoding="utf-8") as output:
            output.write(json.dumps(conflicting, separators=(",", ":")) + "\n")
        conflict_result = run(registry, events, "validate")
        require(conflict_result.returncode != 0, "duplicate revision conflict was accepted")
        require("reconciliation conflict" in conflict_result.stderr, conflict_result.stderr)

        events.unlink()
        case_duplicate = valid_records()
        case_duplicate.append(task("task.FIRST", "pending"))
        write_ndjson(registry, case_duplicate)
        duplicate_result = run(registry, events, "validate")
        require(duplicate_result.returncode != 0, "case-insensitive duplicate task id accepted")
        require("duplicate id" in duplicate_result.stderr, duplicate_result.stderr)

        missing_detail = valid_records()
        missing_detail[-1].pop("description")
        write_ndjson(registry, missing_detail)
        detail_result = run(registry, events, "validate")
        require(detail_result.returncode != 0, "task without drilldown detail accepted")
        require("drilldown detail" in detail_result.stderr, detail_result.stderr)

        missing_dependency = valid_records()
        missing_dependency[-1]["requires"] = ["task.missing"]
        write_ndjson(registry, missing_dependency)
        missing_result = run(registry, events, "validate")
        require(missing_result.returncode != 0, "missing dependency accepted")
        require("missing dependencies" in missing_result.stderr, missing_result.stderr)

        cyclic = valid_records()
        cyclic[2]["requires"] = ["task.second"]
        write_ndjson(registry, cyclic)
        cyclic_result = run(registry, events, "validate")
        require(cyclic_result.returncode != 0, "dependency cycle accepted")
        require("dependency cycle" in cyclic_result.stderr, cyclic_result.stderr)

    print("work-item event journal self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
