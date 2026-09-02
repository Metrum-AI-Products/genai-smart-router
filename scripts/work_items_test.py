#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

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
        status_reason=fields.pop("status_reason", f"{status} because the fixture is in that state"),
        next_steps=fields.pop(
            "next_steps",
            [] if status in {"done", "cancelled"} else [f"Advance {record_id}"],
        ),
        human_actions=fields.pop(
            "human_actions",
            [f"What must be answered to unblock {record_id}?"] if status == "blocked" else [],
        ),
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
        missing_context_registry = root / "missing-context.ndjson"
        missing_context_records = valid_records()
        missing_context_records[2].pop("status_reason")
        write_ndjson(missing_context_registry, missing_context_records)
        missing_context = run(missing_context_registry, root / "missing-context-events.ndjson", "validate")
        require(missing_context.returncode != 0, "task without status reason was accepted")
        require("status_reason is required" in missing_context.stderr, missing_context.stderr)

        missing_blocker_questions_registry = root / "missing-blocker-questions.ndjson"
        missing_blocker_questions_records = valid_records()
        missing_blocker_questions_records[2]["human_actions"] = []
        write_ndjson(missing_blocker_questions_registry, missing_blocker_questions_records)
        missing_blocker_questions = run(
            missing_blocker_questions_registry,
            root / "missing-blocker-questions-events.ndjson",
            "validate",
        )
        require(missing_blocker_questions.returncode != 0, "blocked task without unblock questions was accepted")
        require(
            "blocked task requires at least one human_actions unblock question" in missing_blocker_questions.stderr,
            missing_blocker_questions.stderr,
        )

        vague_blocker_question_registry = root / "vague-blocker-question.ndjson"
        vague_blocker_question_records = valid_records()
        vague_blocker_question_records[2]["human_actions"] = ["Approve this task."]
        write_ndjson(vague_blocker_question_registry, vague_blocker_question_records)
        vague_blocker_question = run(
            vague_blocker_question_registry,
            root / "vague-blocker-question-events.ndjson",
            "validate",
        )
        require(vague_blocker_question.returncode != 0, "blocked task with a non-question human action was accepted")
        require(
            "blocked task human_actions must be explicit questions ending in '?'" in vague_blocker_question.stderr,
            vague_blocker_question.stderr,
        )

        missing_transition_context = run(
            registry,
            events,
            "update",
            "task.first",
            "--expect-status",
            "blocked",
            "--status",
            "in_progress",
        )
        require(missing_transition_context.returncode != 0, "status transition without context succeeded")
        require(
            "changing status requires --status-reason" in missing_transition_context.stderr,
            missing_transition_context.stderr,
        )
        require(not events.exists(), "invalid status transition created an event journal")


        update = run(
            registry,
            events,
            "update",
            "task.first",
            "--expect-status",
            "blocked",
            "--title",
            "Deliver consolidated task scope",
            "--description",
            "One task now covers the deduplicated implementation contract.",
            "--status",
            "in_progress",
            "--status-reason",
            "Implementation work is actively running.",
            "--replace-next-steps",
            "Complete the implementation checks.",
            "--replace-human-actions",
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
        require(
            update_event["task"]["title"] == "Deliver consolidated task scope",
            "task update omitted replacement title",
        )
        require(
            update_event["task"]["description"]
            == "One task now covers the deduplicated implementation contract.",
            "task update omitted replacement description",
        )
        require(
            update_event["task"]["status_reason"] == "Implementation work is actively running.",
            "status transition omitted its reason",
        )
        require(
            update_event["task"]["next_steps"] == ["Complete the implementation checks."],
            "status transition omitted next steps",
        )
        require(update_event["task"]["human_actions"] == [], "status transition omitted human actions")
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
            "--status-reason",
            "The dashboard task is ready for implementation.",
            "--next-step",
            "Build and verify the dashboard.",
            "--reference",
            "https://github.com/uwdata/mosaic",
            "--action",
            "Reconcile both sources in the browser.",
            "--acceptance",
            "The dashboard refreshes without a rebuild.",
        )
        require(add.returncode == 0, add.stderr)
        add_event = json.loads(add.stdout)
        require(add_event["event_type"] == "task.created", "add did not emit task.created")
        require(add_event["changed_fields"] == ["*"], "create event fields are not canonical")
        require(
            add_event["task"]["description"].startswith("Interactive work-item"),
            "create event omitted drilldown detail",
        )
        require(add_event["task"]["actions"] == ["Reconcile both sources in the browser."], "create event omitted actions")
        require(
            add_event["task"]["acceptance"] == ["The dashboard refreshes without a rebuild."],
            "create event omitted acceptance",
        )
        require(
            add_event["task"]["status_reason"] == "The dashboard task is ready for implementation.",
            "create event omitted status reason",
        )
        require(
            add_event["task"]["next_steps"] == ["Build and verify the dashboard."],
            "create event omitted next steps",
        )
        require(add_event["task"]["human_actions"] == [], "create event omitted human actions")
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
            "task.dashboard",
            "--expect-status",
            "pending",
            "--replace-actions",
            "Stale replacement must not be written.",
        )
        require(stale.returncode != 0, "stale status update succeeded")
        require("expected status" in stale.stderr, f"missing stale status error: {stale.stderr}")
        require(events.read_bytes() == before_stale, "stale update changed event journal")

        replace_requires = run(
            registry,
            events,
            "update",
            "task.second",
            "--expect-status",
            "pending",
            "--replace-requires",
            "gate.resume",
        )
        require(replace_requires.returncode == 0, replace_requires.stderr)
        replace_event = json.loads(replace_requires.stdout)
        require(replace_event["task"]["requires"] == ["gate.resume"], "dependencies were not replaced")
        require(
            "requires" in replace_event["changed_fields"]
            and set(replace_event["changed_fields"]) <= {"requires", "updated_at"},
            f"unexpected dependency update fields: {replace_event['changed_fields']}",
        )
        require(registry.read_bytes() == original_baseline, "dependency update mutated baseline registry")

        clear_requires = run(registry, events, "update", "task.first", "--replace-requires")
        require(clear_requires.returncode == 0, clear_requires.stderr)
        clear_event = json.loads(clear_requires.stdout)
        require(clear_event["task"]["requires"] == [], "dependencies were not cleared")
        require(
            "requires" in clear_event["changed_fields"]
            and set(clear_event["changed_fields"]) <= {"requires", "updated_at"},
            f"unexpected dependency clear fields: {clear_event['changed_fields']}",
        )

        replace_details = run(
            registry,
            events,
            "update",
            "task.dashboard",
            "--expect-status",
            "ready",
            "--replace-actions",
            "Reconcile both sources in the browser.",
            "Retain immutable event history.",
            "--replace-acceptance",
            "The dashboard refreshes from /data/version without a rebuild.",
            "--replace-evidence",
            "Sanitized event counts.",
            "--replace-commands",
            "rtk python3 scripts/work_items.py validate",
            "--replace-references",
            "https://github.com/uwdata/mosaic",
            "task.first",
        )
        require(replace_details.returncode == 0, replace_details.stderr)
        detail_event = json.loads(replace_details.stdout)
        require(
            detail_event["task"]["actions"]
            == ["Reconcile both sources in the browser.", "Retain immutable event history."],
            "actions were not replaced",
        )
        require(
            detail_event["task"]["acceptance"]
            == ["The dashboard refreshes from /data/version without a rebuild."],
            "acceptance was not replaced",
        )
        require(detail_event["task"]["evidence"] == ["Sanitized event counts."], "evidence was not replaced")
        require(
            detail_event["task"]["commands"] == ["rtk python3 scripts/work_items.py validate"],
            "commands were not replaced",
        )
        require(
            detail_event["task"]["references"]
            == ["https://github.com/uwdata/mosaic", "task.first"],
            "references were not replaced",
        )
        expected_detail_fields = {"actions", "acceptance", "evidence", "commands", "references"}
        require(
            expected_detail_fields <= set(detail_event["changed_fields"])
            and set(detail_event["changed_fields"]) <= expected_detail_fields | {"updated_at"},
            f"unexpected structured replacement fields: {detail_event['changed_fields']}",
        )

        clear_details = run(
            registry,
            events,
            "update",
            "task.dashboard",
            "--replace-actions",
            "--replace-acceptance",
            "--replace-evidence",
            "--replace-commands",
            "--replace-references",
        )
        require(clear_details.returncode == 0, clear_details.stderr)
        clear_detail_event = json.loads(clear_details.stdout)
        for field in ("actions", "acceptance", "evidence", "commands", "references"):
            require(clear_detail_event["task"][field] == [], f"{field} was not cleared")
        require(
            expected_detail_fields <= set(clear_detail_event["changed_fields"])
            and set(clear_detail_event["changed_fields"]) <= expected_detail_fields | {"updated_at"},
            f"unexpected structured clear fields: {clear_detail_event['changed_fields']}",
        )

        before_invalid_structured_replacement = events.read_bytes()
        invalid_structured_replacement = run(
            registry,
            events,
            "update",
            "task.dashboard",
            "--replace-actions",
            "",
        )
        require(
            invalid_structured_replacement.returncode != 0,
            "empty structured replacement value was accepted",
        )
        require(
            "actions must be a string or array of non-empty strings"
            in invalid_structured_replacement.stderr,
            invalid_structured_replacement.stderr,
        )
        require(
            events.read_bytes() == before_invalid_structured_replacement,
            "invalid structured replacement changed event journal",
        )

        before_invalid_requires = events.read_bytes()
        missing_requires = run(
            registry,
            events,
            "update",
            "task.second",
            "--replace-requires",
            "task.missing",
        )
        require(missing_requires.returncode != 0, "missing replacement dependency accepted")
        require("missing dependencies" in missing_requires.stderr, missing_requires.stderr)
        require(events.read_bytes() == before_invalid_requires, "missing dependency changed event journal")

        cyclic_requires = run(
            registry,
            events,
            "update",
            "task.first",
            "--replace-requires",
            "task.first",
        )
        require(cyclic_requires.returncode != 0, "cyclic replacement dependency accepted")
        require("dependency cycle" in cyclic_requires.stderr, cyclic_requires.stderr)
        require(events.read_bytes() == before_invalid_requires, "cyclic dependency changed event journal")

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
        require(events.read_bytes() == before_invalid_requires, "duplicate add changed event journal")

        invalid_due = run(registry, events, "update", "task.second", "--due-date", "2026-02-30")
        require(invalid_due.returncode != 0, "invalid due date succeeded")
        require(events.read_bytes() == before_invalid_requires, "invalid due date changed event journal")

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
