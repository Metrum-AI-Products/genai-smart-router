#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate, inspect, append events to, and reconcile the work-item registry."""

from __future__ import annotations

import argparse
import fcntl
import json
import os
import sys
import tempfile
import uuid
from datetime import date, datetime, timezone
from pathlib import Path
from typing import Any, TextIO


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_REGISTRY = REPO_ROOT / "work-items.ndjson"
DEFAULT_EVENTS = REPO_ROOT / "work-item-events.ndjson"
REGISTRY_SCHEMA = "smart-llmrouter.work-items/v1"
EVENT_SCHEMA = "smart-llmrouter.work-item-events/v1"
TASK_STATUSES = ("pending", "ready", "in_progress", "blocked", "done", "cancelled")
ACTIVE_TASK_STATUSES = ("pending", "ready", "in_progress", "blocked")
PRIORITIES = ("critical", "high", "normal", "low")
EVENT_TYPES = ("task.created", "task.updated")


class RegistryError(ValueError):
    """A registry or event record violates the work-item contract."""


def load_ndjson(path: Path, *, label: str, missing_ok: bool = False) -> list[dict[str, Any]]:
    try:
        with path.open(encoding="utf-8") as source:
            return load_ndjson_stream(source, label=label)
    except FileNotFoundError as error:
        if missing_ok:
            return []
        raise RegistryError(f"{label} not found: {path}") from error


def load_ndjson_stream(source: TextIO, *, label: str) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for line_number, line in enumerate(source, start=1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as error:
            raise RegistryError(f"{label} line {line_number}: invalid JSON: {error.msg}") from error
        if not isinstance(record, dict):
            raise RegistryError(f"{label} line {line_number}: record must be an object")
        records.append(record)
    return records


def load_registry(path: Path) -> list[dict[str, Any]]:
    records = load_ndjson(path, label="registry")
    if not records:
        raise RegistryError("registry has no records")
    return records


def load_events(path: Path) -> list[dict[str, Any]]:
    return load_ndjson(path, label="event journal", missing_ok=True)


def parse_due_date(value: Any, *, record_id: str) -> None:
    if value is None:
        return
    if not isinstance(value, str):
        raise RegistryError(f"{record_id}: due_date must be YYYY-MM-DD or null")
    try:
        parsed = date.fromisoformat(value)
    except ValueError as error:
        raise RegistryError(f"{record_id}: invalid due_date {value!r}; expected YYYY-MM-DD") from error
    if parsed.isoformat() != value:
        raise RegistryError(f"{record_id}: due_date must use canonical YYYY-MM-DD form")


def parse_timestamp(value: Any, *, field: str, record_id: str) -> None:
    if not isinstance(value, str) or not value.endswith("Z"):
        raise RegistryError(f"{record_id}: {field} must be a UTC RFC3339 timestamp")
    try:
        datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError as error:
        raise RegistryError(f"{record_id}: invalid {field}") from error


def validate_string_list(record: dict[str, Any], field: str, *, record_id: str) -> list[str] | None:
    values = record.get(field)
    if values is None:
        return None
    if not isinstance(values, list) or not all(isinstance(item, str) and item.strip() for item in values):
        raise RegistryError(f"{record_id}: {field} must be an array of non-empty strings")
    return values


def validate_status_context(record: dict[str, Any], *, required: bool) -> None:
    record_id = record["id"]
    status_reason = record.get("status_reason")
    if status_reason is not None and (not isinstance(status_reason, str) or not status_reason.strip()):
        raise RegistryError(f"{record_id}: status_reason must be a non-empty string")
    next_steps = validate_string_list(record, "next_steps", record_id=record_id)
    human_actions = validate_string_list(record, "human_actions", record_id=record_id)
    if not required:
        return
    if status_reason is None:
        raise RegistryError(f"{record_id}: status_reason is required")
    if next_steps is None:
        raise RegistryError(f"{record_id}: next_steps is required")
    if human_actions is None:
        raise RegistryError(f"{record_id}: human_actions is required")
    if record["status"] in ACTIVE_TASK_STATUSES and not next_steps:
        raise RegistryError(f"{record_id}: active task status requires at least one next_steps item")
    if record["status"] == "blocked":
        if not human_actions:
            raise RegistryError(f"{record_id}: blocked task requires at least one human_actions unblock question")
        if any(not question.rstrip().endswith("?") for question in human_actions):
            raise RegistryError(f"{record_id}: blocked task human_actions must be explicit questions ending in '?'")


def validate_task(record: dict[str, Any], *, require_status_context: bool = False) -> None:
    record_id = record["id"]
    if not record_id.startswith("task."):
        raise RegistryError(f"{record_id}: task id must start with 'task.'")
    if record.get("status") not in TASK_STATUSES:
        raise RegistryError(f"{record_id}: task status must be one of {', '.join(TASK_STATUSES)}")
    if not isinstance(record.get("title"), str) or not record["title"]:
        raise RegistryError(f"{record_id}: task title is required")
    if not isinstance(record.get("stream"), str) or not record["stream"]:
        raise RegistryError(f"{record_id}: stream must be a non-empty string")
    if not isinstance(record.get("phase"), int) or record["phase"] < 1:
        raise RegistryError(f"{record_id}: phase must be a positive integer")
    if record.get("priority") not in PRIORITIES:
        raise RegistryError(f"{record_id}: priority must be one of {', '.join(PRIORITIES)}")
    requires = record.get("requires", [])
    if not isinstance(requires, list) or not all(isinstance(item, str) and item for item in requires):
        raise RegistryError(f"{record_id}: requires must be an array of record IDs")
    references = record.get("references", [])
    if not isinstance(references, list) or not all(isinstance(item, str) and item for item in references):
        raise RegistryError(f"{record_id}: references must be an array of strings")
    description = record.get("description")
    if description is not None and (not isinstance(description, str) or not description.strip()):
        raise RegistryError(f"{record_id}: description must be a non-empty string when present")
    detail_fields = ("actions", "acceptance", "evidence")
    has_structured_detail = False
    for field in detail_fields:
        values = record.get(field, [])
        if isinstance(values, str):
            if not values.strip():
                raise RegistryError(f"{record_id}: {field} must not be empty")
            has_structured_detail = True
        elif isinstance(values, list) and all(isinstance(item, str) and item.strip() for item in values):
            has_structured_detail = has_structured_detail or bool(values)
        else:
            raise RegistryError(f"{record_id}: {field} must be a string or array of non-empty strings")
    if not (description and description.strip()) and not has_structured_detail:
        raise RegistryError(
            f"{record_id}: drilldown detail requires description, actions, acceptance, or evidence"
        )
    validate_status_context(record, required=require_status_context)


def validate_registry(
    records: list[dict[str, Any]], *, require_status_context: bool = False
) -> None:
    ids: set[str] = set()
    normalized_ids: dict[str, str] = {}
    task_ids: set[str] = set()
    plans = 0

    for index, record in enumerate(records, start=1):
        record_id = record.get("id")
        if not isinstance(record_id, str) or not record_id:
            raise RegistryError(f"registry line {index}: id must be a non-empty string")
        normalized_id = record_id.casefold()
        if normalized_id in normalized_ids:
            raise RegistryError(
                f"duplicate id: {record_id} conflicts with {normalized_ids[normalized_id]}"
            )
        ids.add(record_id)
        normalized_ids[normalized_id] = record_id

        kind = record.get("kind")
        if not isinstance(kind, str) or not kind:
            raise RegistryError(f"{record_id}: kind must be a non-empty string")
        status = record.get("status")
        if not isinstance(status, str) or not status:
            raise RegistryError(f"{record_id}: status must be a non-empty string")
        if "due_date" not in record:
            raise RegistryError(f"{record_id}: due_date column is required")
        parse_due_date(record["due_date"], record_id=record_id)
        parse_timestamp(record.get("created_at"), field="created_at", record_id=record_id)
        parse_timestamp(record.get("updated_at"), field="updated_at", record_id=record_id)

        if kind == "plan":
            plans += 1
            if record.get("schema") != REGISTRY_SCHEMA:
                raise RegistryError(f"{record_id}: unsupported registry schema")
        if kind == "task":
            task_ids.add(record_id)
            validate_task(record, require_status_context=require_status_context)

    if plans != 1:
        raise RegistryError(f"registry must contain exactly one plan record; found {plans}")

    graph: dict[str, list[str]] = {}
    for record in records:
        if record["kind"] != "task":
            continue
        dependencies = record.get("requires", [])
        missing = [dependency for dependency in dependencies if dependency not in ids]
        if missing:
            raise RegistryError(f"{record['id']}: missing dependencies: {', '.join(missing)}")
        graph[record["id"]] = [dependency for dependency in dependencies if dependency in task_ids]

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(task_id: str) -> None:
        if task_id in visiting:
            raise RegistryError(f"task dependency cycle includes {task_id}")
        if task_id in visited:
            return
        visiting.add(task_id)
        for dependency in graph[task_id]:
            visit(dependency)
        visiting.remove(task_id)
        visited.add(task_id)

    for task_id in graph:
        visit(task_id)


def changed_fields(previous: dict[str, Any], current: dict[str, Any]) -> list[str]:
    return sorted(key for key in set(previous) | set(current) if previous.get(key) != current.get(key))


def validate_event_shape(event: dict[str, Any], *, index: int) -> None:
    event_id = event.get("event_id")
    if not isinstance(event_id, str) or not event_id:
        raise RegistryError(f"event {index}: event_id must be a non-empty string")
    if event.get("kind") != "work_item_event" or event.get("schema") != EVENT_SCHEMA:
        raise RegistryError(f"{event_id}: unsupported event schema")
    if event.get("event_type") not in EVENT_TYPES:
        raise RegistryError(f"{event_id}: unsupported event_type")
    task_id = event.get("task_id")
    if not isinstance(task_id, str) or not task_id:
        raise RegistryError(f"{event_id}: task_id must be a non-empty string")
    revision = event.get("revision")
    base_revision = event.get("base_revision")
    if not isinstance(revision, int) or revision < 1:
        raise RegistryError(f"{event_id}: revision must be a positive integer")
    if not isinstance(base_revision, int) or base_revision < 0 or revision != base_revision + 1:
        raise RegistryError(f"{event_id}: revision must equal base_revision + 1")
    parse_timestamp(event.get("occurred_at"), field="occurred_at", record_id=event_id)
    if not isinstance(event.get("actor"), str) or not event["actor"]:
        raise RegistryError(f"{event_id}: actor must be a non-empty string")
    fields = event.get("changed_fields")
    if not isinstance(fields, list) or not all(isinstance(field, str) and field for field in fields):
        raise RegistryError(f"{event_id}: changed_fields must be an array of strings")
    task = event.get("task")
    if not isinstance(task, dict) or task.get("kind") != "task" or task.get("id") != task_id:
        raise RegistryError(f"{event_id}: task must be a full snapshot for {task_id}")
    validate_task(task)
    if event.get("status") != task.get("status") or event.get("due_date") != task.get("due_date"):
        raise RegistryError(f"{event_id}: top-level status/due_date must match the task snapshot")


def project_registry(
    baseline: list[dict[str, Any]], events: list[dict[str, Any]]
) -> tuple[list[dict[str, Any]], dict[str, int]]:
    validate_registry(baseline)
    task_by_id = {record["id"]: dict(record) for record in baseline if record["kind"] == "task"}
    task_order = [record["id"] for record in baseline if record["kind"] == "task"]
    revisions = {task_id: 0 for task_id in task_by_id}
    events_by_task: dict[str, list[dict[str, Any]]] = {}
    event_ids: set[str] = set()
    revision_keys: set[tuple[str, int]] = set()

    for index, event in enumerate(events, start=1):
        validate_event_shape(event, index=index)
        event_id = event["event_id"]
        if event_id in event_ids:
            raise RegistryError(f"duplicate event_id: {event_id}")
        event_ids.add(event_id)
        revision_key = (event["task_id"], event["revision"])
        if revision_key in revision_keys:
            raise RegistryError(
                f"reconciliation conflict: {event['task_id']} has multiple events at revision {event['revision']}"
            )
        revision_keys.add(revision_key)
        events_by_task.setdefault(event["task_id"], []).append(event)

    for task_id, task_events in events_by_task.items():
        current = task_by_id.get(task_id)
        revision = 0
        for event in sorted(task_events, key=lambda item: item["revision"]):
            event_id = event["event_id"]
            if event["base_revision"] != revision or event["revision"] != revision + 1:
                raise RegistryError(
                    f"{event_id}: revision gap for {task_id}; expected base {revision} and revision {revision + 1}"
                )
            if event["event_type"] == "task.created":
                if current is not None or revision != 0:
                    raise RegistryError(f"{event_id}: task.created conflicts with existing task {task_id}")
                if event["changed_fields"] != ["*"]:
                    raise RegistryError(f"{event_id}: task.created changed_fields must be ['*']")
                task_order.append(task_id)
            else:
                if current is None:
                    raise RegistryError(f"{event_id}: task.updated has no existing task {task_id}")
                expected_fields = changed_fields(current, event["task"])
                if event["changed_fields"] != expected_fields:
                    raise RegistryError(
                        f"{event_id}: changed_fields do not match the full task snapshot; expected {expected_fields}"
                    )
                if event["task"].get("created_at") != current.get("created_at"):
                    raise RegistryError(f"{event_id}: task.updated cannot change created_at")
            current = dict(event["task"])
            revision = event["revision"]
        if current is not None:
            task_by_id[task_id] = current
            revisions[task_id] = revision

    projected = [dict(record) for record in baseline if record["kind"] != "task"]
    projected.extend(task_by_id[task_id] for task_id in task_order)
    validate_registry(projected, require_status_context=True)
    return projected, revisions


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def write_registry(path: Path, records: list[dict[str, Any]]) -> None:
    validate_registry(records)
    path.parent.mkdir(parents=True, exist_ok=True)
    mode = path.stat().st_mode & 0o777 if path.exists() else 0o644
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as output:
            for record in records:
                output.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")))
                output.write("\n")
            output.flush()
            os.fsync(output.fileno())
        os.chmod(temporary, mode)
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        temporary.unlink(missing_ok=True)


def task_records(records: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [record for record in records if record["kind"] == "task"]


def event_for_task(
    *,
    event_type: str,
    task: dict[str, Any],
    revision: int,
    actor: str,
    previous: dict[str, Any] | None,
) -> dict[str, Any]:
    return {
        "kind": "work_item_event",
        "schema": EVENT_SCHEMA,
        "event_id": str(uuid.uuid4()),
        "event_type": event_type,
        "task_id": task["id"],
        "revision": revision,
        "base_revision": revision - 1,
        "occurred_at": utc_now(),
        "actor": actor,
        "status": task["status"],
        "due_date": task["due_date"],
        "changed_fields": ["*"] if previous is None else changed_fields(previous, task),
        "task": task,
    }


def append_event(
    events_path: Path,
    baseline: list[dict[str, Any]],
    event: dict[str, Any],
) -> None:
    events_path.parent.mkdir(parents=True, exist_ok=True)
    with events_path.open("a+", encoding="utf-8") as journal:
        fcntl.flock(journal.fileno(), fcntl.LOCK_EX)
        try:
            journal.seek(0)
            current_events = load_ndjson_stream(journal, label="event journal")
            projected, revisions = project_registry(baseline, current_events)
            current_tasks = {task["id"]: task for task in task_records(projected)}
            current_revision = revisions.get(event["task_id"], 0)
            if event["event_type"] == "task.created" and event["task_id"] in current_tasks:
                raise RegistryError(f"duplicate id: {event['task_id']}")
            if event["event_type"] == "task.updated" and event["task_id"] not in current_tasks:
                raise RegistryError(f"unknown work item: {event['task_id']}")
            if event["base_revision"] != current_revision:
                raise RegistryError(
                    f"{event['task_id']}: expected revision {event['base_revision']}, found {current_revision}; event was not appended"
                )
            project_registry(baseline, [*current_events, event])
            journal.seek(0, os.SEEK_END)
            journal.write(json.dumps(event, ensure_ascii=False, separators=(",", ":")))
            journal.write("\n")
            journal.flush()
            os.fsync(journal.fileno())
        finally:
            fcntl.flock(journal.fileno(), fcntl.LOCK_UN)


def current_registry(registry_path: Path, events_path: Path) -> tuple[list[dict[str, Any]], dict[str, int], int]:
    baseline = load_registry(registry_path)
    events = load_events(events_path)
    projected, revisions = project_registry(baseline, events)
    return projected, revisions, len(events)


def command_validate(registry_path: Path, events_path: Path) -> int:
    projected, _, event_count = current_registry(registry_path, events_path)
    baseline_count = len(load_registry(registry_path))
    print(
        f"work-item registry valid: {baseline_count} baseline records, {event_count} events, "
        f"{len(task_records(projected))} current tasks"
    )
    return 0


def command_list(
    registry_path: Path,
    events_path: Path,
    *,
    status: str | None,
    stream: str | None,
    due_before: str | None,
    as_json: bool,
) -> int:
    records, _, _ = current_registry(registry_path, events_path)
    cutoff: date | None = None
    if due_before:
        try:
            cutoff = date.fromisoformat(due_before)
        except ValueError as error:
            raise RegistryError("--due-before must use YYYY-MM-DD") from error

    tasks = task_records(records)
    if status:
        tasks = [task for task in tasks if task["status"] == status]
    if stream:
        tasks = [task for task in tasks if task["stream"] == stream]
    if cutoff:
        tasks = [
            task
            for task in tasks
            if task["due_date"] is not None and date.fromisoformat(task["due_date"]) <= cutoff
        ]
    tasks.sort(key=lambda task: (task["due_date"] is None, task["due_date"] or "", task["phase"], task["id"]))

    if as_json:
        for task in tasks:
            print(json.dumps(task, ensure_ascii=False, separators=(",", ":")))
        return 0

    print("ID\tSTREAM\tSTATUS\tDUE_DATE\tPRIORITY\tTITLE")
    for task in tasks:
        print(
            f"{task['id']}\t{task['stream']}\t{task['status']}\t{task['due_date'] or '-'}\t"
            f"{task['priority']}\t{task['title']}"
        )
    return 0


def optional_value(value: str | None) -> str | None:
    if value is None or value.lower() != "none":
        return value
    return None


def command_add(
    registry_path: Path,
    events_path: Path,
    *,
    task_id: str,
    title: str,
    description: str | None,
    stream: str,
    phase: int,
    priority: str,
    status: str,
    status_reason: str,
    due_date: str | None,
    assignee: str | None,
    requires: list[str],
    references: list[str],
    actions: list[str],
    acceptance: list[str],
    evidence: list[str],
    next_steps: list[str],
    human_actions: list[str],
    commands: list[str],
    actor: str,
) -> int:
    baseline = load_registry(registry_path)
    projected, revisions = project_registry(baseline, load_events(events_path))
    duplicate = next(
        (record["id"] for record in projected if record["id"].casefold() == task_id.casefold()),
        None,
    )
    if duplicate is not None:
        raise RegistryError(f"duplicate id: {task_id} conflicts with {duplicate}")

    normalized_due_date = optional_value(due_date)
    parse_due_date(normalized_due_date, record_id=task_id)
    now = utc_now()
    task: dict[str, Any] = {
        "kind": "task",
        "id": task_id,
        "stream": stream,
        "phase": phase,
        "title": title,
        "description": description,
        "requires": requires,
        "references": references,
        "actions": actions,
        "acceptance": acceptance,
        "evidence": evidence,
        "status": status,
        "status_reason": status_reason,
        "next_steps": next_steps,
        "human_actions": human_actions,
        "due_date": normalized_due_date,
        "priority": priority,
        "assignee": optional_value(assignee),
        "created_at": now,
        "updated_at": now,
        "started_at": now if status == "in_progress" else None,
        "completed_at": now if status == "done" else None,
    }
    if commands:
        task["commands"] = commands
    validate_task(task, require_status_context=True)
    event = event_for_task(
        event_type="task.created",
        task=task,
        revision=revisions.get(task_id, 0) + 1,
        actor=actor,
        previous=None,
    )
    append_event(events_path, baseline, event)
    print(json.dumps(event, ensure_ascii=False, separators=(",", ":")))
    return 0


def command_update(
    registry_path: Path,
    events_path: Path,
    *,
    task_id: str,
    title: str | None,
    description: str | None,
    status: str | None,
    status_reason: str | None,
    due_date: str | None,
    assignee: str | None,
    replace_requires: list[str] | None,
    replace_actions: list[str] | None,
    replace_acceptance: list[str] | None,
    replace_evidence: list[str] | None,
    replace_commands: list[str] | None,
    replace_references: list[str] | None,
    replace_next_steps: list[str] | None,
    replace_human_actions: list[str] | None,
    expect_status: str | None,
    actor: str,
) -> int:
    replacements = {
        "requires": replace_requires,
        "actions": replace_actions,
        "acceptance": replace_acceptance,
        "evidence": replace_evidence,
        "commands": replace_commands,
        "references": replace_references,
        "next_steps": replace_next_steps,
        "human_actions": replace_human_actions,
    }
    if (
        title is None
        and description is None
        and status is None
        and status_reason is None
        and due_date is None
        and assignee is None
        and all(value is None for value in replacements.values())
    ):
        raise RegistryError(
            "update requires a title, description, status, status reason, due date, assignee, "
            "or structured-list replacement option"
        )

    baseline = load_registry(registry_path)
    projected, revisions = project_registry(baseline, load_events(events_path))
    previous = next((record for record in projected if record["id"] == task_id), None)
    if previous is None:
        raise RegistryError(f"unknown work item: {task_id}")
    if previous["kind"] != "task":
        raise RegistryError(f"{task_id}: only task records can be updated")
    if expect_status is not None and previous["status"] != expect_status:
        raise RegistryError(
            f"{task_id}: expected status {expect_status!r}, found {previous['status']!r}; event was not appended"
        )
    status_changed = status is not None and status != previous["status"]
    if status_changed and (
        status_reason is None or replace_next_steps is None or replace_human_actions is None
    ):
        raise RegistryError(
            f"{task_id}: changing status requires --status-reason, --replace-next-steps, "
            "and --replace-human-actions"
        )

    task = dict(previous)
    now = utc_now()
    if title is not None:
        task["title"] = title
    if description is not None:
        task["description"] = description
    if status is not None:
        task["status"] = status
        if status == "in_progress" and task.get("started_at") is None:
            task["started_at"] = now
        if status == "done":
            task["completed_at"] = now
        elif task.get("completed_at") is not None:
            task["completed_at"] = None
    if status_reason is not None:
        task["status_reason"] = status_reason
    if due_date is not None:
        task["due_date"] = optional_value(due_date)
        parse_due_date(task["due_date"], record_id=task_id)
    if assignee is not None:
        task["assignee"] = optional_value(assignee)
    for field, value in replacements.items():
        if value is not None:
            task[field] = value
    validate_task(task, require_status_context=True)
    task["updated_at"] = now

    event = event_for_task(
        event_type="task.updated",
        task=task,
        revision=revisions.get(task_id, 0) + 1,
        actor=actor,
        previous=previous,
    )
    append_event(events_path, baseline, event)
    print(json.dumps(event, ensure_ascii=False, separators=(",", ":")))
    return 0


def command_project(registry_path: Path, events_path: Path, *, output: str) -> int:
    projected, _, _ = current_registry(registry_path, events_path)
    if output == "-":
        for record in projected:
            print(json.dumps(record, ensure_ascii=False, separators=(",", ":")))
        return 0
    destination = Path(output)
    if destination.resolve() in {registry_path.resolve(), events_path.resolve()}:
        raise RegistryError("project output must not overwrite the baseline or event journal")
    write_registry(destination, projected)
    print(f"projected registry written: {destination}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", type=Path, default=DEFAULT_REGISTRY, help="baseline NDJSON registry path")
    parser.add_argument("--events", type=Path, default=DEFAULT_EVENTS, help="append-only NDJSON event journal")
    parser.add_argument("--actor", default="local-cli", help="non-secret reconciliation actor identifier")
    subparsers = parser.add_subparsers(dest="command", required=True)

    subparsers.add_parser("validate", help="validate baseline, events, revisions, dates, and dependencies")

    list_parser = subparsers.add_parser("list", help="list reconciled task records")
    list_parser.add_argument("--status", choices=TASK_STATUSES)
    list_parser.add_argument("--stream")
    list_parser.add_argument("--due-before")
    list_parser.add_argument("--json", action="store_true")

    add_parser = subparsers.add_parser("add", help="append a task.created event")
    add_parser.add_argument("id")
    add_parser.add_argument("--title", required=True)
    add_parser.add_argument("--description", required=True)
    add_parser.add_argument("--stream", required=True)
    add_parser.add_argument("--phase", required=True, type=int)
    add_parser.add_argument("--priority", choices=PRIORITIES, default="normal")
    add_parser.add_argument("--status", choices=TASK_STATUSES, default="pending")
    add_parser.add_argument("--status-reason", required=True, help="why the task currently has this status")
    add_parser.add_argument("--due-date", help="YYYY-MM-DD or none")
    add_parser.add_argument("--assignee", help="assignee identifier or none")
    add_parser.add_argument("--requires", action="append", default=[])
    add_parser.add_argument("--reference", action="append", default=[])
    add_parser.add_argument("--action", action="append", default=[])
    add_parser.add_argument("--acceptance", action="append", default=[])
    add_parser.add_argument("--evidence", action="append", default=[])
    add_parser.add_argument("--command", dest="commands", action="append", default=[])
    add_parser.add_argument("--next-step", action="append", default=[])
    add_parser.add_argument("--human-action", action="append", default=[])

    update_parser = subparsers.add_parser("update", help="append a task.updated event")
    update_parser.add_argument("id")
    update_parser.add_argument("--title", help="replace the task title")
    update_parser.add_argument("--description", help="replace the task description")
    update_parser.add_argument("--status", choices=TASK_STATUSES)
    update_parser.add_argument("--status-reason", help="why the task currently has this status")
    update_parser.add_argument("--due-date", help="YYYY-MM-DD or none")
    update_parser.add_argument("--assignee", help="assignee identifier or none")
    update_parser.add_argument(
        "--replace-requires",
        nargs="*",
        default=None,
        metavar="RECORD_ID",
        help="replace the complete dependency list; omit RECORD_ID to clear it",
    )
    for option, destination, label in (
        ("--replace-actions", "replace_actions", "action"),
        ("--replace-acceptance", "replace_acceptance", "acceptance item"),
        ("--replace-evidence", "replace_evidence", "evidence item"),
        ("--replace-commands", "replace_commands", "command"),
        ("--replace-references", "replace_references", "reference"),
        ("--replace-next-steps", "replace_next_steps", "next step"),
        ("--replace-human-actions", "replace_human_actions", "human action"),
    ):
        update_parser.add_argument(
            option,
            dest=destination,
            nargs="*",
            default=None,
            metavar=label.upper().replace(" ", "_"),
            help=f"replace the complete {label} list; omit values to clear it",
        )
    update_parser.add_argument("--expect-status", choices=TASK_STATUSES)

    project_parser = subparsers.add_parser("project", help="materialize reconciled state without changing source files")
    project_parser.add_argument("--output", default="-", help="output path or - for stdout")
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        if args.command == "validate":
            return command_validate(args.file, args.events)
        if args.command == "list":
            return command_list(
                args.file,
                args.events,
                status=args.status,
                stream=args.stream,
                due_before=args.due_before,
                as_json=args.json,
            )
        if args.command == "add":
            return command_add(
                args.file,
                args.events,
                task_id=args.id,
                title=args.title,
                description=args.description,
                stream=args.stream,
                phase=args.phase,
                priority=args.priority,
                status=args.status,
                status_reason=args.status_reason,
                due_date=args.due_date,
                assignee=args.assignee,
                requires=args.requires,
                references=args.reference,
                actions=args.action,
                acceptance=args.acceptance,
                evidence=args.evidence,
                commands=args.commands,
                next_steps=args.next_step,
                human_actions=args.human_action,
                actor=args.actor,
            )
        if args.command == "update":
            return command_update(
                args.file,
                args.events,
                task_id=args.id,
                title=args.title,
                description=args.description,
                status=args.status,
                status_reason=args.status_reason,
                due_date=args.due_date,
                assignee=args.assignee,
                replace_requires=args.replace_requires,
                replace_actions=args.replace_actions,
                replace_acceptance=args.replace_acceptance,
                replace_evidence=args.replace_evidence,
                replace_commands=args.replace_commands,
                replace_references=args.replace_references,
                replace_next_steps=args.replace_next_steps,
                replace_human_actions=args.replace_human_actions,
                expect_status=args.expect_status,
                actor=args.actor,
            )
        if args.command == "project":
            return command_project(args.file, args.events, output=args.output)
    except RegistryError as error:
        print(f"work-item registry error: {error}", file=sys.stderr)
        return 2
    parser.error(f"unsupported command: {args.command}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
