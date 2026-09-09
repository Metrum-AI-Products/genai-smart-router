#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Render the reconciled work-item registry as self-contained printable HTML."""

from __future__ import annotations

import argparse
from collections import Counter
from html import escape
import re
from pathlib import Path
from typing import Any

from work_items import (
    DEFAULT_EVENTS,
    DEFAULT_REGISTRY,
    PRIORITIES,
    RegistryError,
    TASK_STATUSES,
    current_registry,
    task_records,
)

GITHUB_REPOSITORY = "https://github.com/sysadmin-metrum-ai/genai-smart-router"
LINK_PATTERN = re.compile(r"https?://[^\s]+|PR#\d+|#\d+", re.IGNORECASE)


def text(value: Any) -> str:
    return escape("" if value is None else str(value), quote=True)


def values(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, list):
        return [str(item) for item in value]
    return [str(value)]

def reference_href(value: str) -> str | None:
    if re.fullmatch(r"https?://[^\s]+", value, re.IGNORECASE):
        return value
    pull_request = re.fullmatch(r"PR#(\d+)", value, re.IGNORECASE)
    if pull_request:
        return f"{GITHUB_REPOSITORY}/pull/{pull_request.group(1)}"
    issue = re.fullmatch(r"#(\d+)", value)
    if issue:
        return f"{GITHUB_REPOSITORY}/issues/{issue.group(1)}"
    return None


def linked_text(value: Any) -> str:
    source = "" if value is None else str(value)
    rendered: list[str] = []
    cursor = 0
    for match in LINK_PATTERN.finditer(source):
        rendered.append(text(source[cursor : match.start()]))
        token = match.group(0)
        suffix = ""
        if token.lower().startswith(("http://", "https://")):
            trailing = re.search(r"[.,;:!?]+$", token)
            if trailing:
                suffix = trailing.group(0)
                token = token[: -len(suffix)]
        href = reference_href(token)
        if href:
            rendered.append(
                f'<a href="{text(href)}" target="_blank" rel="noopener noreferrer">{text(token)}</a>'
            )
        else:
            rendered.append(text(token))
        rendered.append(text(suffix))
        cursor = match.end()
    rendered.append(text(source[cursor:]))
    return "".join(rendered)


def status_list(label: str, value: Any, empty_state: str) -> str:
    items = values(value)
    if items:
        content = "<ul>" + "".join(f"<li>{linked_text(item)}</li>" for item in items) + "</ul>"
    else:
        content = f'<p class="empty">{text(empty_state)}</p>'
    return f'<section class="status-section"><strong>{text(label)}</strong>{content}</section>'


def status_context(task: dict[str, Any]) -> str:
    reason = task.get("status_reason") or "No status reason has been recorded."
    closed = task["status"] in {"done", "cancelled"}
    next_empty = (
        "No next step is required; this task is closed."
        if closed
        else "No next step has been recorded."
    )
    return f"""<div class="status-context">
      <section class="status-section status-reason"><strong>Why this status?</strong><p>{linked_text(reason)}</p></section>
      {status_list("What happens next?", task.get("next_steps"), next_empty)}
      {status_list("What can a human do?", task.get("human_actions"), "No direct human action is currently required.")}
    </div>"""


def details(label: str, value: Any) -> str:
    items = values(value)
    if not items:
        return ""
    rendered = "".join(f"<li>{linked_text(item)}</li>" for item in items)
    return f"<div class=\"detail\"><strong>{text(label)}</strong><ul>{rendered}</ul></div>"


def render_html(registry_path: Path, events_path: Path, title: str) -> str:
    records, revisions, event_count = current_registry(registry_path, events_path)
    tasks = sorted(
        task_records(records),
        key=lambda task: (
            TASK_STATUSES.index(task["status"]),
            PRIORITIES.index(task["priority"]),
            task["due_date"] is None,
            task["due_date"] or "",
            task["id"],
        ),
    )
    status_counts = Counter(task["status"] for task in tasks)
    largest_status = max(status_counts.values(), default=1)
    snapshot_at = max((task["updated_at"] for task in tasks), default="n/a")

    status_cards = "".join(
        f"""<section class="metric">
          <span>{text(status.replace("_", " "))}</span>
          <strong>{status_counts[status]}</strong>
          <i style="width:{status_counts[status] * 100 / largest_status:.1f}%"></i>
        </section>"""
        for status in TASK_STATUSES
    )
    task_rows = "".join(
        f"""<tr>
          <td><strong>{text(task["title"])}</strong><small>{text(task["id"])}</small></td>
          <td>{text(task["status"])}</td>
          <td>{text(task["stream"])}</td>
          <td>{task["phase"]}</td>
          <td>{text(task["priority"])}</td>
          <td>{text(task["due_date"] or "Unscheduled")}</td>
          <td>{text(task["assignee"] or "Unassigned")}</td>
          <td>{revisions.get(task["id"], 0)}</td>
        </tr>"""
        for task in tasks
    )
    task_details = "".join(
        f"""<article class="task">
          <header><div><h3>{text(task["title"])}</h3><code>{text(task["id"])}</code></div>
          <span>{text(task["status"])}</span></header>
          {status_context(task)}
          {f'<p>{text(task.get("description"))}</p>' if task.get("description") else ''}
          <div class="detail-grid">
            {details("Actions", task.get("actions"))}
            {details("Acceptance", task.get("acceptance"))}
            {details("Evidence", task.get("evidence"))}
            {details("Commands", task.get("commands"))}
            {details("Dependencies", task.get("requires"))}
            {details("References", task.get("references"))}
          </div>
          <footer>Priority: {text(task["priority"])} · Due: {text(task["due_date"] or "unscheduled")} · Revision: {revisions.get(task["id"], 0)} · Updated: {text(task["updated_at"])}</footer>
        </article>"""
        for task in tasks
    )

    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{text(title)}</title>
<style>
@page {{ size: A4 landscape; margin: 12mm; @bottom-right {{ content: "Page " counter(page) " of " counter(pages); }} }}
* {{ box-sizing: border-box; }}
body {{ color: #172033; font: 9pt/1.35 sans-serif; margin: 0; }}
h1, h2, h3, p {{ margin-top: 0; }}
h1 {{ font-size: 22pt; margin-bottom: 2mm; }}
h2 {{ border-bottom: 1px solid #b7c1d3; font-size: 14pt; margin-top: 8mm; padding-bottom: 2mm; }}
.meta {{ color: #536079; margin-bottom: 5mm; }}
.metrics {{ display: grid; grid-template-columns: repeat(6, 1fr); gap: 3mm; }}
.metric {{ background: #f0f3f8; border: 1px solid #d7deea; padding: 3mm; position: relative; }}
.metric span {{ display: block; font-size: 7pt; text-transform: uppercase; }}
.metric strong {{ font-size: 17pt; }}
.metric i {{ background: #3157c8; bottom: 0; height: 1mm; left: 0; position: absolute; }}
table {{ border-collapse: collapse; font-size: 7.5pt; width: 100%; }}
th, td {{ border-bottom: 1px solid #d7deea; padding: 1.8mm; text-align: left; vertical-align: top; }}
th {{ background: #172033; color: white; text-transform: uppercase; }}
td small {{ color: #68758d; display: block; font-family: monospace; }}
.task {{ border: 1px solid #cbd4e2; break-inside: avoid; margin-bottom: 4mm; padding: 4mm; }}
.task header {{ align-items: start; display: flex; justify-content: space-between; }}
.task h3 {{ font-size: 11pt; margin-bottom: 1mm; }}
.task header > span {{ background: #e9eef9; border-radius: 8px; padding: 1mm 2mm; }}
.task code {{ color: #536079; font-size: 7.5pt; }}
.task p {{ margin: 3mm 0; }}
.detail-grid {{ display: grid; grid-template-columns: repeat(2, 1fr); gap: 2mm 5mm; }}
.detail strong {{ font-size: 7.5pt; text-transform: uppercase; }}
.detail ul {{ margin: 1mm 0 0; padding-left: 5mm; }}
.status-context {{ background: #f6f8fb; border-left: 1mm solid #3157c8; display: grid; grid-template-columns: repeat(2, 1fr); gap: 2mm 5mm; margin: 3mm 0; padding: 3mm; }}
.status-section strong {{ font-size: 7.5pt; text-transform: uppercase; }}
.status-section p, .status-section ul {{ margin: 1mm 0 0; }}
.status-reason {{ grid-column: 1 / -1; }}
.empty {{ color: #68758d; font-style: italic; }}
.task a {{ color: #2449b8; text-decoration: underline; }}
.task footer {{ border-top: 1px solid #e0e5ed; color: #68758d; font-size: 7pt; margin-top: 3mm; padding-top: 2mm; }}
</style>
</head>
<body>
<h1>{text(title)}</h1>
<p class="meta">Tracked sources: {text(registry_path.name)} + {text(events_path.name)} · {len(tasks)} tasks · {event_count} events · Snapshot state updated {text(snapshot_at)}</p>
<div class="metrics">{status_cards}</div>
<h2>Reconciled task register</h2>
<table>
<thead><tr><th>Task</th><th>Status</th><th>Stream</th><th>Phase</th><th>Priority</th><th>Due</th><th>Assignee</th><th>Rev</th></tr></thead>
<tbody>{task_rows}</tbody>
</table>
<h2>Task drilldown</h2>
{task_details}
</body>
</html>
"""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", type=Path, default=DEFAULT_REGISTRY)
    parser.add_argument("--events", type=Path, default=DEFAULT_EVENTS)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--title", default="GenAI Smart Router Work Dashboard")
    args = parser.parse_args()
    try:
        rendered = render_html(args.file, args.events, args.title)
    except RegistryError as error:
        parser.error(str(error))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(rendered, encoding="utf-8")
    print(f"work dashboard HTML written: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
