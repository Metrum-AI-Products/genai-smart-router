#!/usr/bin/env python3
"""Behavior tests for review-follow-up cascade closure."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

from close_review_followups import CascadeError, cascade_closed_rollup, parse_child_issues


class FakeIssueClient:
    def __init__(self, issues: dict[int, dict[str, Any]]) -> None:
        self.issues = issues
        self.comments: list[tuple[int, str]] = []
        self.closed: list[int] = []

    def get_issue(self, issue_number: int) -> dict[str, Any]:
        return self.issues[issue_number]

    def comment(self, issue_number: int, body: str) -> None:
        self.comments.append((issue_number, body))

    def close(self, issue_number: int) -> None:
        self.closed.append(issue_number)
        self.issues[issue_number]["state"] = "closed"


class MockGitHubHandler(BaseHTTPRequestHandler):
    requests: list[tuple[str, str, dict[str, Any] | None]] = []

    def respond(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def request_payload(self) -> dict[str, Any] | None:
        length = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(length)) if length else None

    def do_GET(self) -> None:
        self.requests.append(("GET", self.path, None))
        issue_number = int(self.path.rsplit("/", 1)[1])
        self.respond(200, {"number": issue_number, "state": "open" if issue_number == 714 else "closed"})

    def do_POST(self) -> None:
        payload = self.request_payload()
        self.requests.append(("POST", self.path, payload))
        self.respond(201, {"id": 1})

    def do_PATCH(self) -> None:
        payload = self.request_payload()
        self.requests.append(("PATCH", self.path, payload))
        self.respond(200, {"state": "closed"})

    def log_message(self, format: str, *args: Any) -> None:
        return


def require_error(function: Any, expected: str) -> None:
    try:
        function()
    except CascadeError as error:
        assert expected in str(error), error
    else:
        raise AssertionError(f"expected CascadeError containing {expected!r}")


def closed_event(body: str, *, label: bool = True) -> dict[str, Any]:
    labels = [{"name": "review-followup-rollup"}] if label else []
    return {
        "action": "closed",
        "issue": {"number": 774, "body": body, "labels": labels},
    }


def main() -> int:
    marker = "<!-- review-followup-rollup:v1 children=652,714,773 -->"
    assert parse_child_issues(marker, 774) == (652, 714, 773)
    require_error(lambda: parse_child_issues("no marker", 774), "exactly one")
    require_error(
        lambda: parse_child_issues(
            "<!-- review-followup-rollup:v1 children=714,714 -->",
            774,
        ),
        "unique",
    )
    require_error(
        lambda: parse_child_issues(
            "<!-- review-followup-rollup:v1 children=714,774 -->",
            774,
        ),
        "cannot list itself",
    )

    client = FakeIssueClient(
        {
            652: {"number": 652, "state": "closed"},
            714: {"number": 714, "state": "open"},
            773: {"number": 773, "state": "open"},
        }
    )
    result = cascade_closed_rollup(closed_event(marker), client)
    assert result is not None
    assert result.master_issue == 774
    assert result.children == (652, 714, 773)
    assert result.closed == (714, 773)
    assert result.already_closed == (652,)
    assert client.closed == [714, 773]
    assert client.comments == [
        (714, "Closed automatically because review-follow-up master #774 was completed."),
        (773, "Closed automatically because review-follow-up master #774 was completed."),
    ]

    ignored = FakeIssueClient({})
    assert cascade_closed_rollup({"action": "opened", "issue": {}}, ignored) is None
    assert cascade_closed_rollup(closed_event(marker, label=False), ignored) is None

    pull_client = FakeIssueClient({714: {"number": 714, "state": "open", "pull_request": {}}})
    require_error(
        lambda: cascade_closed_rollup(
            closed_event("<!-- review-followup-rollup:v1 children=714 -->"),
            pull_client,
        ),
        "pull request",
    )
    assert not pull_client.comments and not pull_client.closed

    MockGitHubHandler.requests = []
    server = ThreadingHTTPServer(("127.0.0.1", 0), MockGitHubHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    try:
        with tempfile.TemporaryDirectory() as temporary_directory:
            event_path = Path(temporary_directory) / "event.json"
            event_path.write_text(
                json.dumps(
                    closed_event(
                        "<!-- review-followup-rollup:v1 children=714,773 -->"
                    )
                ),
                encoding="utf-8",
            )
            environment = dict(os.environ, TEST_GITHUB_TOKEN="test-token-must-not-print")
            smoke = subprocess.run(
                [
                    sys.executable,
                    str(Path(__file__).with_name("close_review_followups.py")),
                    "--event-path",
                    str(event_path),
                    "--repository",
                    "owner/repository",
                    "--token-env",
                    "TEST_GITHUB_TOKEN",
                    "--api-url",
                    f"http://127.0.0.1:{server.server_port}",
                ],
                text=True,
                capture_output=True,
                env=environment,
                timeout=30,
            )
        assert smoke.returncode == 0, smoke.stderr
        assert "closed=[714] already_closed=[773]" in smoke.stdout
        assert "test-token-must-not-print" not in smoke.stdout + smoke.stderr
        assert MockGitHubHandler.requests == [
            ("GET", "/repos/owner/repository/issues/714", None),
            (
                "POST",
                "/repos/owner/repository/issues/714/comments",
                {"body": "Closed automatically because review-follow-up master #774 was completed."},
            ),
            (
                "PATCH",
                "/repos/owner/repository/issues/714",
                {"state": "closed", "state_reason": "completed"},
            ),
            ("GET", "/repos/owner/repository/issues/773", None),
        ]
    finally:
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)

    print("review follow-up cascade tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
