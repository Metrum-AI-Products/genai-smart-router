#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Close review-follow-up child issues when their labeled master closes."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

ROLLUP_LABEL = "review-followup-rollup"
MARKER_RE = re.compile(
    r"<!--\s*review-followup-rollup:v1\s+children=([0-9,\s]+)\s*-->",
    re.IGNORECASE,
)


class CascadeError(RuntimeError):
    """Raised when the rollup contract or a child update is invalid."""


class IssueClient(Protocol):
    def get_issue(self, issue_number: int) -> dict[str, Any]: ...

    def comment(self, issue_number: int, body: str) -> None: ...

    def close(self, issue_number: int) -> None: ...


@dataclass(frozen=True)
class CascadeResult:
    master_issue: int
    children: tuple[int, ...]
    closed: tuple[int, ...]
    already_closed: tuple[int, ...]


def parse_child_issues(body: str, master_issue: int) -> tuple[int, ...]:
    matches = MARKER_RE.findall(body)
    if len(matches) != 1:
        raise CascadeError("rollup issue must contain exactly one v1 child marker")
    values = [value.strip() for value in matches[0].split(",")]
    if not values or any(not value for value in values):
        raise CascadeError("rollup child marker contains an empty issue number")
    children = tuple(int(value) for value in values)
    if any(child <= 0 for child in children):
        raise CascadeError("rollup child issue numbers must be positive")
    if master_issue in children:
        raise CascadeError("rollup master cannot list itself as a child")
    if len(set(children)) != len(children):
        raise CascadeError("rollup child issue numbers must be unique")
    if len(children) > 100:
        raise CascadeError("rollup cannot close more than 100 child issues")
    return children


def event_labels(issue: dict[str, Any]) -> set[str]:
    labels: set[str] = set()
    for label in issue.get("labels", []):
        if isinstance(label, str):
            labels.add(label)
        elif isinstance(label, dict) and isinstance(label.get("name"), str):
            labels.add(label["name"])
    return labels


def cascade_closed_rollup(event: dict[str, Any], client: IssueClient) -> CascadeResult | None:
    if event.get("action") != "closed":
        return None
    master = event.get("issue")
    if not isinstance(master, dict) or ROLLUP_LABEL not in event_labels(master):
        return None
    if master.get("state_reason") != "completed":
        return None
    master_issue = master.get("number")
    body = master.get("body")
    if not isinstance(master_issue, int) or not isinstance(body, str):
        raise CascadeError("rollup close event lacks an issue number or body")

    children = parse_child_issues(body, master_issue)
    closed: list[int] = []
    already_closed: list[int] = []
    failures: list[str] = []
    for child in children:
        try:
            issue = client.get_issue(child)
            if "pull_request" in issue:
                raise CascadeError(f"#{child} is a pull request, not an issue")
            if issue.get("state") == "closed":
                already_closed.append(child)
                continue
            if issue.get("state") != "open":
                raise CascadeError(f"#{child} has unsupported state {issue.get('state')!r}")
            client.comment(
                child,
                f"Closed automatically because review-follow-up master #{master_issue} was completed.",
            )
            client.close(child)
            closed.append(child)
        except Exception as error:  # Continue so one child cannot prevent attempts on the rest.
            failures.append(f"#{child}: {error}")
    if failures:
        raise CascadeError("; ".join(failures))
    return CascadeResult(master_issue, children, tuple(closed), tuple(already_closed))


class GitHubIssueClient:
    def __init__(self, repository: str, token: str, api_url: str = "https://api.github.com") -> None:
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
            raise CascadeError("repository must use owner/name syntax")
        self.repository = repository
        self.token = token
        self.api_url = api_url.rstrip("/")

    def request(self, method: str, path: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        request = urllib.request.Request(
            f"{self.api_url}{path}",
            data=data,
            method=method,
            headers={
                "Accept": "application/vnd.github+json",
                "Authorization": f"Bearer {self.token}",
                "Content-Type": "application/json",
                "User-Agent": "genai-smart-router-review-followup-rollup",
                "X-GitHub-Api-Version": "2022-11-28",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                raw = response.read()
        except urllib.error.HTTPError as error:
            raise CascadeError(f"GitHub API {method} {path} returned HTTP {error.code}") from error
        except urllib.error.URLError as error:
            raise CascadeError(f"GitHub API {method} {path} failed: {error.reason}") from error
        return json.loads(raw) if raw else {}

    def get_issue(self, issue_number: int) -> dict[str, Any]:
        return self.request("GET", f"/repos/{self.repository}/issues/{issue_number}")

    def comment(self, issue_number: int, body: str) -> None:
        self.request(
            "POST",
            f"/repos/{self.repository}/issues/{issue_number}/comments",
            {"body": body},
        )

    def close(self, issue_number: int) -> None:
        self.request(
            "PATCH",
            f"/repos/{self.repository}/issues/{issue_number}",
            {"state": "closed", "state_reason": "completed"},
        )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--event-path", type=Path, required=True)
    parser.add_argument("--repository", required=True, help="GitHub owner/name")
    parser.add_argument("--token-env", default="GH_TOKEN")
    parser.add_argument("--api-url", default="https://api.github.com")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    token = os.environ.get(args.token_env)
    if not token:
        print(f"missing GitHub token environment variable: {args.token_env}", file=sys.stderr)
        return 2
    try:
        event = json.loads(args.event_path.read_text(encoding="utf-8"))
        result = cascade_closed_rollup(
            event,
            GitHubIssueClient(args.repository, token, api_url=args.api_url),
        )
    except (CascadeError, OSError, json.JSONDecodeError) as error:
        print(f"review follow-up cascade failed: {error}", file=sys.stderr)
        return 1
    if result is None:
        print("review follow-up cascade skipped: event is not a labeled rollup closure")
        return 0
    print(
        f"review follow-up cascade complete: master=#{result.master_issue} "
        f"closed={list(result.closed)} already_closed={list(result.already_closed)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
