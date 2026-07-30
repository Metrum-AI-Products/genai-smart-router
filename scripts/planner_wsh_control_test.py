#!/usr/bin/env python3
"""Deterministic contract tests for the planner-owned tmux/WSH launcher."""

from __future__ import annotations

import json
import os
import fcntl
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import sys
import time
import subprocess
import signal
import shutil
import shlex

from planner_wsh_control import AtomicWriteError, ControlPlane, ControlPlaneError, Profile, RunResult, Runner, _durable_json_replace


class FakeRunner:
    def __init__(self, root: Path, worktree: Path):
        self.root = root
        self.worktree = worktree
        self.tmux_exists = False
        self.windows: set[str] = set()
        self.calls: list[list[str]] = []
        self.call_kwargs: list[dict[str, object]] = []
        self.status_payload: object | None = None
        self.fail_new_window = False
        self.fail_stop = False
        self.status_result: RunResult | None = None
        self.status_hook = None
        self.wsh_sessions: dict[str, str] = {}
        self.fail_status_spawn = False
        self.fail_list_windows = False
        self.fail_codex_preflight = False
        self.tag_payload: str | None = None
        self.server_identity = "router-planner-server"
        self.server_identity_payload: str | None = None
        self.server_identity_result: RunResult | None = None
        self.server_identity_responses: list[RunResult] = []

    def run(self, argv: list[str], *, check: bool = True, **kwargs: object) -> RunResult:
        self.calls.append(argv)
        self.call_kwargs.append(kwargs)
        if argv[0] == "git":
            cwd = Path(argv[2])
            args = argv[3:]
            if args == ["rev-parse", "--show-toplevel"]:
                return RunResult(0, str(cwd) + "\n")
            if args == ["rev-parse", "--path-format=absolute", "--git-common-dir"]:
                return RunResult(0, str(self.root / ".git") + "\n")
            if args == ["worktree", "list", "--porcelain"]:
                return RunResult(0, f"worktree {self.root}\n\nworktree {self.worktree}\nbranch refs/heads/issue-573\n\n")
            if args == ["branch", "--show-current"]:
                return RunResult(0, "issue-573-planner-wsh\n")
            if args[:1] == ["merge-base"]:
                return RunResult(0, "base-sha\n")
            raise AssertionError(f"unexpected git command: {argv}")
        if argv[:2] == ["tmux", "has-session"]:
            return RunResult(0 if self.tmux_exists else 1)
        if argv[:2] == ["tmux", "list-windows"]:
            if self.fail_list_windows:
                return RunResult(1)
            return RunResult(0 if self.tmux_exists else 1, "\n".join(sorted(self.windows)) + "\n")
        if argv[:2] == ["tmux", "new-session"]:
            self.tmux_exists = True
            self.windows.add(argv[argv.index("-n") + 1])
            return RunResult(0)
        if argv[:2] == ["tmux", "new-window"]:
            if self.fail_new_window:
                raise ControlPlaneError("simulated tmux failure")
            window = argv[argv.index("-n") + 1]
            self.windows.add(window)
            if window == "sprint-planner":
                planner_command = shlex.split(argv[-1])
                adapter = planner_command.index("scripts/planner_wsh_planner.sh")
                self.wsh_sessions[planner_command[adapter + 2]] = planner_command[adapter + 3]
            if window.startswith("agent-"):
                self.wsh_sessions["router-worker-" + window.removeprefix("agent-")] = ""
            return RunResult(0)
        if argv[:2] == ["tmux", "kill-window"]:
            self.windows.remove(argv[-1].split(":", 1)[1])
            return RunResult(0)
        if argv[:2] == ["tmux", "kill-session"]:
            self.tmux_exists = False
            self.windows.clear()
            return RunResult(0)
        if argv[:2] == ["tmux", "attach-session"]:
            return RunResult(0)
        if argv[0] == "wsh" and argv[-1] == "list":
            if self.fail_status_spawn:
                raise ControlPlaneError("unable to start command")
            if self.status_hook:
                self.status_hook()
            if self.status_result:
                return self.status_result
            payload = self.status_payload if self.status_payload is not None else "\n".join(
                f"{session}\nTAGS {tag}" for session, tag in sorted(self.wsh_sessions.items())
            )
            return RunResult(0, json.dumps(payload) if isinstance(payload, dict) else str(payload))
        if argv == ["wsh", "-L", "router-planner", "identity", "--json"]:
            if self.server_identity_responses:
                return self.server_identity_responses.pop(0)
            if self.server_identity_result:
                return self.server_identity_result
            if self.server_identity_payload is not None:
                return RunResult(0, self.server_identity_payload)
            return RunResult(0, json.dumps({"server_identity": self.server_identity}))
        if argv[0] == "wsh" and "kill" in argv:
            if self.fail_stop:
                raise ControlPlaneError("simulated WSH stop failure")
            self.wsh_sessions.pop(argv[-1], None)
            return RunResult(0)
        if argv[0] == "wsh" and "tag" in argv:
            if self.tag_payload is not None:
                return RunResult(0, f"Session '{argv[-1]}': {self.tag_payload}")
            return RunResult(0, f"Session '{argv[-1]}': {self.wsh_sessions.get(argv[-1], '')}")
        if argv == ["codex", "--help"]:
            if self.fail_codex_preflight:
                return RunResult(1)
            return RunResult(0, "Usage: codex [OPTIONS] [PROMPT]\n\nCommands:\n  exec\n")
        raise AssertionError(f"unexpected command: {argv}")


class PlannerWSHControlTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        base = Path(self.tmp.name)
        self.root = base / "repo"
        self.worktree = base / "issue-573"
        self.root.mkdir()
        (self.root / ".git").mkdir()
        self.worktree.mkdir()
        self.profile_path = base / "profile.json"
        self.profile_path.write_text(
            json.dumps(
                {
                    "profile_id": "local-planner",
                    "planner_owner": "sprint_planner",
                    "tmux_session": "router-planner",
                    "wsh_server_name": "router-planner",
                    "wsh_server_identity": "router-planner-server",
                    "bootstrap_identity_wait_seconds": 1,
                    "planner_wsh_session_id": "router-planner-session",
                    "state_directory": str(base / "state"),
                    "worker_session_prefix": "router-worker",
                    "base_ref": "origin/main",
                    "lease_ttl_seconds": 3600,
                    "forbidden_environment_variables": ["WSH_SESSION_ID"],
                    "commands": {
                        "wsh_server": ["wsh", "-L", "{wsh_server_name}", "server"],
                        "wsh_identity": ["wsh", "-L", "{wsh_server_name}", "identity", "--json"],
                        "planner": ["scripts/planner_wsh_planner.sh", "{wsh_server_name}", "{planner_wsh_session_id}", "planner-{profile_id}", "{repository_root}", "{planner_assignment_prompt}"],
                        "worker": ["scripts/planner_wsh_worker.sh", "{wsh_server_name}", "{wsh_session_id}", "assignment-{assignment_id}", "lease-{lease_id}", "{worktree}", "{assignment_prompt}"],
                        "codex_preflight": ["codex", "--help"],
                        "wsh_stop": ["wsh", "-L", "{wsh_server_name}", "kill", "{wsh_session_id}"],
                        "planner_stop": ["wsh", "-L", "{wsh_server_name}", "kill", "{planner_wsh_session_id}"],
                        "planner_tag": ["wsh", "-L", "{wsh_server_name}", "tag", "{planner_wsh_session_id}"],
                        "wsh_status": ["wsh", "-L", "{wsh_server_name}", "list"],
                    },
                }
            ),
            encoding="utf-8",
        )
        self.profile_path.chmod(0o600)
        self.runner = FakeRunner(self.root, self.worktree)
        self.control = ControlPlane(Profile.load(self.profile_path), self.runner, self.root)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def calls(self, command: str) -> list[list[str]]:
        return [call for call in self.runner.calls if call[:2] == ["tmux", command]]

    def test_bootstrap_is_idempotent_and_separates_server_and_planner(self) -> None:
        first = self.control.bootstrap()
        second = self.control.bootstrap()
        self.assertEqual({"wsh-server", "sprint-planner"}, set(first["windows"]))
        self.assertEqual(first["windows"], second["windows"])
        self.assertEqual(1, len(self.calls("new-session")))
        self.assertEqual(1, len([call for call in self.calls("new-window") if "sprint-planner" in call]))
        self.assertEqual("present", first["planner_wsh"])
        self.assertEqual("router-planner-session", first["planner_wsh_session_id"])
        planner_window = [call for call in self.calls("new-window") if "sprint-planner" in call][0]
        self.assertIn("planner_wsh_planner.sh", planner_window[-1])

    def test_bootstrap_refuses_a_foreign_control_window_before_creating_planner_state(self) -> None:
        self.runner.tmux_exists = True
        self.runner.windows.add("foreign-window")
        with self.assertRaisesRegex(ControlPlaneError, "unexpected window"):
            self.control.bootstrap()
        self.assertEqual({"foreign-window"}, self.runner.windows)
        self.assertFalse(self.control.state_path.exists())

    def test_bootstrap_waits_only_for_unavailable_identity_then_revalidates_before_creation(self) -> None:
        self.runner.server_identity_responses = [
            RunResult(1),
            RunResult(0, '{"server_identity":"router-planner-server"}'),
            RunResult(0, '{"server_identity":"router-planner-server"}'),
        ]
        self.control.bootstrap()
        planner_call = next(index for index, call in enumerate(self.runner.calls) if call[:2] == ["tmux", "new-window"] and "sprint-planner" in call)
        identity_calls = [
            (index, call) for index, call in enumerate(self.runner.calls)
            if call == ["wsh", "-L", "router-planner", "identity", "--json"]
        ]
        self.assertEqual(3, len([call for index, call in identity_calls if index < planner_call]))
        readiness_calls = self.runner.calls[identity_calls[0][0] : planner_call]
        self.assertTrue(all(call[0] != "wsh" or call[-2:] == ["identity", "--json"] for call in readiness_calls))
        audit = self.control.audit_path.read_text(encoding="utf-8")
        self.assertIn('"action":"server-identity-readiness"', audit)
        self.assertIn('"outcome":"matched"', audit)
        self.assertIn('"disposition":"bootstrap"', audit)

    def test_bootstrap_unavailable_expiry_never_creates_planner_or_state(self) -> None:
        self.runner.server_identity_responses = [RunResult(1)]
        with mock.patch("planner_wsh_control.time.monotonic", side_effect=[0.0, 0.0, 1.0]):
            with self.assertRaisesRegex(ControlPlaneError, "server identity is unavailable"):
                self.control.bootstrap()
        self.assertEqual({"wsh-server"}, self.runner.windows)
        self.assertFalse(self.control.state_path.exists())
        self.assertFalse(any(call[:2] == ["tmux", "new-window"] and "sprint-planner" in call for call in self.runner.calls))
        self.assertTrue(all(call[0] != "wsh" or call[-2:] == ["identity", "--json"] for call in self.runner.calls))
        audit = self.control.audit_path.read_text(encoding="utf-8")
        event = json.loads(audit.strip())
        self.assertEqual({"action", "at", "event_id", "outcome", "disposition"}, set(event))
        self.assertEqual(("server-identity-readiness", "unavailable", "bootstrap"), (event["action"], event["outcome"], event["disposition"]))
        self.assertNotIn("router-planner-server", audit)
        self.assertNotIn("another-server", audit)

    def test_bootstrap_terminal_identity_outcomes_do_not_poll_or_mutate(self) -> None:
        cases = {
            "missing": "{}",
            "mismatch": '{"server_identity":"another-server"}',
            "malformed": "not-json",
            "ambiguous": '{"server_identity":"router-planner-server","server_identity":"another-server"}',
        }
        for expected, payload in cases.items():
            with self.subTest(expected=expected):
                self.runner.tmux_exists = False
                self.runner.windows.clear()
                self.runner.calls.clear()
                self.runner.call_kwargs.clear()
                self.runner.server_identity_responses.clear()
                self.runner.server_identity_responses = [RunResult(0, payload)]
                with self.assertRaisesRegex(ControlPlaneError, f"server identity is {expected}"):
                    self.control.bootstrap()
                identity_calls = [call for call in self.runner.calls if call[0] == "wsh"]
                self.assertEqual([["wsh", "-L", "router-planner", "identity", "--json"]], identity_calls)
                self.assertEqual({"wsh-server"}, self.runner.windows)
                self.assertFalse(self.control.state_path.exists())

    def test_bootstrap_identity_timeout_is_capped_by_the_remaining_deadline(self) -> None:
        self.runner.server_identity_responses = [RunResult(0, "{}")]
        with mock.patch("planner_wsh_control.time.monotonic", side_effect=[0.0, 0.9]):
            with self.assertRaisesRegex(ControlPlaneError, "server identity is missing"):
                self.control.bootstrap()
        identity_kwargs = next(
            kwargs for call, kwargs in zip(self.runner.calls, self.runner.call_kwargs)
            if call == ["wsh", "-L", "router-planner", "identity", "--json"]
        )
        self.assertLessEqual(float(identity_kwargs["timeout_seconds"]), 0.1)

    def test_bootstrap_final_creation_guard_rejects_a_later_mismatch(self) -> None:
        self.runner.server_identity_responses = [
            RunResult(0, '{"server_identity":"router-planner-server"}'),
            RunResult(0, '{"server_identity":"another-server"}'),
        ]
        with self.assertRaisesRegex(ControlPlaneError, "planner session creation rejected: profile WSH server identity is mismatch"):
            self.control.bootstrap()
        self.assertEqual({"wsh-server"}, self.runner.windows)
        self.assertFalse(self.control.state_path.exists())

    def test_durable_json_replace_distinguishes_pre_and_post_replace_failures(self) -> None:
        target = self.root.parent / "durable.json"
        target.write_text('{"old":true}\n', encoding="utf-8")
        with mock.patch("planner_wsh_control.os.replace", side_effect=OSError("replace failed")):
            with self.assertRaises(AtomicWriteError) as before:
                _durable_json_replace(target, {"new": True}, prefix=".test-")
        self.assertFalse(before.exception.after_replace)
        self.assertEqual('{"old":true}\n', target.read_text(encoding="utf-8"))
        with mock.patch("planner_wsh_control._fsync_parent_directory", side_effect=OSError("directory fsync failed")):
            with self.assertRaises(AtomicWriteError) as after:
                _durable_json_replace(target, {"new": True}, prefix=".test-")
        self.assertTrue(after.exception.after_replace)
        self.assertEqual({"new": True}, json.loads(target.read_text(encoding="utf-8")))

    def test_durable_json_replace_allows_explicit_unsupported_directory_fsync(self) -> None:
        target = self.root.parent / "unsupported-directory-fsync.json"
        with mock.patch("planner_wsh_control._fsync_parent_directory", return_value=False) as fsync_parent:
            self.assertFalse(_durable_json_replace(target, {"ok": True}, prefix=".test-"))
        fsync_parent.assert_called_once_with(target.parent)
        self.assertEqual({"ok": True}, json.loads(target.read_text(encoding="utf-8")))

    def test_worker_allocation_requires_one_fresh_tagged_planner_identity(self) -> None:
        self.control.bootstrap()
        self.runner.wsh_sessions["router-planner-session"] = "wrong-tag"
        with self.assertRaisesRegex(ControlPlaneError, "wrong-identity"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.status_payload = "router-planner-session\nrouter-planner-session\nTAGS planner-local-planner"
        with self.assertRaisesRegex(ControlPlaneError, "duplicate"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.status_payload = "router-planner-session\nTAGS unrelated\nother-session\nTAGS planner-local-planner"
        self.runner.tag_payload = "unrelated"
        with self.assertRaisesRegex(ControlPlaneError, "wrong-identity"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.status_payload = None
        self.runner.tag_payload = None
        self.runner.wsh_sessions["router-planner-session"] = "planner-local-planner"
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertTrue(any(call[0] == "wsh" and "tag" in call and call[-1] == "router-planner-session" for call in self.runner.calls))

    def test_server_identity_handshake_fails_closed_before_worker_mutation(self) -> None:
        self.control.bootstrap()
        cases = {
            "missing": "{}",
            "unavailable": None,
            "mismatch": '{"server_identity":"another-server"}',
            "malformed": "not-json",
            "ambiguous": '{"server_identity":"router-planner-server","server_identity":"another-server"}',
        }
        for expected, payload in cases.items():
            with self.subTest(expected=expected):
                self.runner.server_identity_payload = payload
                self.runner.server_identity_result = RunResult(1) if expected == "unavailable" else None
                before = len(self.runner.calls)
                with self.assertRaisesRegex(ControlPlaneError, f"server identity is {expected}"):
                    self.control.launch_worker("573", "software_engineer", self.worktree)
                calls = [call for call in self.runner.calls[before:] if call[0] == "wsh"]
                self.assertTrue(calls and all(call[-2:] == ["identity", "--json"] for call in calls))
                self.assertNotIn("agent-573-software_engineer", self.runner.windows)
                self.assertEqual({}, json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        self.runner.server_identity_payload = None
        self.runner.server_identity_result = None

    def test_later_invalid_server_identity_cannot_reuse_a_prior_binding_for_stop(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.server_identity_payload = '{"server_identity":"another-server"}'
        before = len(self.runner.calls)
        with self.assertRaisesRegex(ControlPlaneError, "server identity is mismatch"):
            self.control.release_worker("573", "software_engineer", outcome="abandoned", cleanup_authorized=True)
        calls = [call for call in self.runner.calls[before:] if call[0] == "wsh"]
        self.assertTrue(calls and all(call[-2:] == ["identity", "--json"] for call in calls))
        self.assertIn("router-worker-573-software_engineer", self.runner.wsh_sessions)

    def test_server_identity_audit_uses_only_safe_outcome_class(self) -> None:
        self.control.bootstrap()
        self.runner.server_identity_payload = '{"server_identity":"another-server"}'
        with self.assertRaisesRegex(ControlPlaneError, "server identity is mismatch"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        audit = self.control.audit_path.read_text(encoding="utf-8")
        self.assertIn('"action":"server-identity-validation"', audit)
        self.assertIn('"outcome":"mismatch"', audit)
        self.assertIn('"disposition":"worker-allocation"', audit)
        self.assertNotIn("another-server", audit)

    def test_tag_query_does_not_authorize_a_session_name_that_equals_the_expected_tag(self) -> None:
        raw = json.loads(self.profile_path.read_text(encoding="utf-8"))
        raw["planner_wsh_session_id"] = "planner-local-planner"
        self.profile_path.write_text(json.dumps(raw), encoding="utf-8")
        self.profile_path.chmod(0o600)
        self.control = ControlPlane(Profile.load(self.profile_path), self.runner, self.root)
        self.control.bootstrap()
        self.runner.wsh_sessions["planner-local-planner"] = "wrong-tag"
        with self.assertRaisesRegex(ControlPlaneError, "wrong-identity"):
            self.control.launch_worker("573", "software_engineer", self.worktree)

    def test_worker_has_one_window_one_session_and_status_is_metadata_only(self) -> None:
        self.control.bootstrap()
        first = self.control.launch_worker("573", "software_engineer", self.worktree)
        second = self.control.launch_worker("573", "software_engineer", self.worktree)
        worker_windows = [call for call in self.calls("new-window") if "agent-573-software_engineer" in call]
        self.assertEqual(1, len(worker_windows))
        self.assertEqual("router-worker-573-software_engineer", first["workers"][0]["wsh_session_id"])
        self.assertEqual(first["workers"], second["workers"])
        self.assertNotIn("screen", first["workers"][0]["wsh"])
        worker_command = worker_windows[0][-1]
        self.assertIn("-c", worker_windows[0])
        self.assertEqual(str(self.worktree), worker_windows[0][worker_windows[0].index("-c") + 1])
        self.assertIn("planner_wsh_worker.sh", worker_command)
        self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        self.assertNotIn("agent-573-software_engineer", self.runner.windows)
        self.assertTrue(any(call[0] == "wsh" and "kill" in call for call in self.runner.calls))

    def test_rejects_unbootstrapped_primary_duplicate_and_nested_launches_before_window_creation(self) -> None:
        with self.assertRaisesRegex(ControlPlaneError, "not bootstrapped"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertFalse(self.calls("new-window"))
        self.control.bootstrap()
        with self.assertRaisesRegex(ControlPlaneError, "primary checkout"):
            self.control.launch_worker("573", "software_engineer", self.root)
        self.control.launch_worker("573", "software_engineer", self.worktree)
        with self.assertRaisesRegex(ControlPlaneError, "lease"):
            self.control.launch_worker("573", "quality_engineer", self.worktree)
        before = len(self.calls("new-window"))
        old = os.environ.get("WSH_SESSION_ID")
        os.environ["WSH_SESSION_ID"] = "nested"
        try:
            with self.assertRaisesRegex(ControlPlaneError, "existing WSH"):
                self.control.launch_worker("574", "software_engineer", self.worktree)
        finally:
            if old is None:
                del os.environ["WSH_SESSION_ID"]
            else:
                os.environ["WSH_SESSION_ID"] = old
        self.assertEqual(before, len(self.calls("new-window")))

    def test_launch_refuses_foreign_window_without_allocating_a_shared_lease(self) -> None:
        self.control.bootstrap()
        self.runner.windows.add("foreign-window")
        with self.assertRaisesRegex(ControlPlaneError, "unexpected window"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertNotIn("agent-573-software_engineer", self.runner.windows)
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_post_reservation_local_validation_failure_releases_shared_lease(self) -> None:
        self.control.bootstrap()

        def introduce_conflicting_window() -> None:
            self.runner.windows.add("agent-573-software_engineer")

        self.runner.status_hook = introduce_conflicting_window
        with self.assertRaisesRegex(ControlPlaneError, "worker window already exists"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertNotIn("573-software_engineer", json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_post_reservation_state_write_failure_releases_shared_lease(self) -> None:
        self.control.bootstrap()
        write_state = self.control._write_state

        def fail_worker_persistence(state: dict[str, object]) -> None:
            if "573-software_engineer" in state["workers"]:
                raise OSError("simulated state persistence failure")
            write_state(state)

        self.control._write_state = fail_worker_persistence  # type: ignore[method-assign]
        with self.assertRaisesRegex(ControlPlaneError, "reservation could not be persisted"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertNotIn("573-software_engineer", json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_post_replace_reservation_uncertainty_retains_matching_lease_for_retry(self) -> None:
        self.control.bootstrap()
        write_state = self.control._write_state
        failed = False

        def uncertain_once(state: dict[str, object]) -> None:
            nonlocal failed
            if not failed and "573-software_engineer" in state["workers"]:
                failed = True
                raise AtomicWriteError(self.control.state_path, after_replace=True, cause=OSError("directory fsync failed"))
            write_state(state)

        self.control._write_state = uncertain_once  # type: ignore[method-assign]
        with self.assertRaisesRegex(ControlPlaneError, "persistence is uncertain"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        with self.control._locked_registry() as registry:
            self.assertIn(self.control._discover_worktree(self.worktree)[2], registry["leases"])
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.control.release_worker("573", "software_engineer", outcome="abandoned", cleanup_authorized=True)

    def test_template_and_tmux_enumeration_failures_after_reservation_release_exact_lease(self) -> None:
        self.control.bootstrap()
        original_worker = self.control.profile.commands["worker"]
        self.control.profile.commands["worker"] = ["bad-template-{unknown}"]
        with self.assertRaisesRegex(ControlPlaneError, "template"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.control.profile.commands["worker"] = original_worker
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

        def fail_later_tmux_enumeration() -> None:
            self.runner.fail_list_windows = True

        self.runner.status_hook = fail_later_tmux_enumeration
        with self.assertRaisesRegex(ControlPlaneError, "cannot be enumerated"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertNotIn("573-software_engineer", json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_tmux_create_failure_retains_start_uncertain_until_exact_release(self) -> None:
        self.control.bootstrap()
        self.runner.fail_new_window = True
        with self.assertRaisesRegex(ControlPlaneError, "simulated tmux"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        state = json.loads(self.control.state_path.read_text(encoding="utf-8"))
        self.assertEqual("start-uncertain", state["workers"]["573-software_engineer"]["state"])
        with self.control._locked_registry() as registry:
            self.assertTrue(registry["leases"])
        self.runner.fail_new_window = False
        self.control.release_worker("573", "software_engineer", outcome="abandoned", cleanup_authorized=True)
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_failed_shutdown_requires_authorized_recovery_before_new_launch(self) -> None:
        self.control.bootstrap()
        self.runner.fail_stop = True
        with self.assertRaisesRegex(ControlPlaneError, "simulated WSH"):
            self.control.shutdown()
        with self.control._locked_registry() as registry:
            self.assertEqual("draining", registry["drain"]["state"])
        with self.assertRaisesRegex(ControlPlaneError, "drain requires explicit recovery"):
            self.control._reserve_shared_lease("another-worktree", "573-software_engineer")
        self.runner.fail_stop = False
        self.control.shutdown(recovery_authorized=True)
        with self.control._locked_registry() as registry:
            self.assertIsNone(registry["drain"])

    def test_proof_backed_recovery_permits_only_unavailable_identity(self) -> None:
        self.control.bootstrap()
        self.control._begin_repository_shutdown()
        self.control._record_planner_absence_proof()
        self.runner.status_payload = "router-planner-session\nrouter-planner-session"
        with self.assertRaisesRegex(ControlPlaneError, "planner WSH identity is duplicate"):
            self.control.shutdown(recovery_authorized=True)
        self.runner.status_payload = None
        self.runner.wsh_sessions = {"router-planner-session": "wrong-tag"}
        with self.assertRaisesRegex(ControlPlaneError, "planner WSH identity is wrong-identity"):
            self.control.shutdown(recovery_authorized=True)
        self.runner.wsh_sessions = {}
        self.runner.status_result = RunResult(1)
        self.runner.windows.remove("wsh-server")
        self.control.shutdown(recovery_authorized=True)

    def test_absence_proof_requires_exact_owner_profile_generation_and_session(self) -> None:
        self.control._begin_repository_shutdown()
        self.control._record_planner_absence_proof()
        with self.control._locked_registry() as registry:
            registry["drain"]["planner_absence_proof"]["planner_wsh_session_id"] = "wrong-session"
        self.assertFalse(self.control._has_planner_absence_proof())
        with self.control._locked_registry() as registry:
            registry["drain"]["planner_absence_proof"]["planner_wsh_session_id"] = "router-planner-session"
            registry["drain"]["generation"] += 1
        self.assertFalse(self.control._has_planner_absence_proof())

    def test_existing_assignment_with_a_different_worktree_never_reserves_shared_lease(self) -> None:
        self.control.bootstrap()
        state = json.loads(self.control.state_path.read_text(encoding="utf-8"))
        state["workers"]["573-software_engineer"] = {"worktree_id": "another-worktree"}
        self.control.state_path.write_text(json.dumps(state), encoding="utf-8")
        with self.assertRaisesRegex(ControlPlaneError, "different immutable worktree lease"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        with self.control._locked_registry() as registry:
            self.assertEqual({}, registry["leases"])

    def test_shutdown_refuses_leases_and_unexpected_windows_then_cleans_exact_windows(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        with self.assertRaisesRegex(ControlPlaneError, "worker leases"):
            self.control.shutdown()
        self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        self.runner.windows.add("unrelated")
        with self.assertRaisesRegex(ControlPlaneError, "unexpected window"):
            self.control.shutdown()
        self.runner.windows.remove("unrelated")
        self.control.shutdown()
        self.assertFalse(self.runner.tmux_exists)

    def test_expired_lease_is_visible_but_is_not_automatically_reused(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        state_path = self.control.state_path
        state = json.loads(state_path.read_text(encoding="utf-8"))
        state["workers"]["573-software_engineer"]["expires_at"] = "2000-01-01T00:00:00+00:00"
        state_path.write_text(json.dumps(state), encoding="utf-8")
        status = self.control.status()
        self.assertEqual("expired", status["workers"][0]["lifecycle"])
        with self.assertRaisesRegex(ControlPlaneError, "lease"):
            self.control.launch_worker("573", "quality_engineer", self.worktree)

    def test_profile_rejects_missing_runtime_safety_configuration(self) -> None:
        raw = json.loads(self.profile_path.read_text(encoding="utf-8"))
        raw["state_directory"] = "relative-state"
        self.profile_path.write_text(json.dumps(raw), encoding="utf-8")
        with self.assertRaisesRegex(ControlPlaneError, "must be absolute"):
            Profile.load(self.profile_path)

    def test_help_verified_wsh_cli_template_grammar(self) -> None:
        commands = self.control.profile.commands
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "server"], commands["wsh_server"])
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "identity", "--json"], commands["wsh_identity"])
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "list"], commands["wsh_status"])
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "kill", "{wsh_session_id}"], commands["wsh_stop"])
        self.assertEqual(["scripts/planner_wsh_planner.sh", "{wsh_server_name}", "{planner_wsh_session_id}", "planner-{profile_id}", "{repository_root}", "{planner_assignment_prompt}"], commands["planner"])
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "kill", "{planner_wsh_session_id}"], commands["planner_stop"])
        self.assertEqual(["wsh", "-L", "{wsh_server_name}", "tag", "{planner_wsh_session_id}"], commands["planner_tag"])
        self.assertEqual(["codex", "--help"], commands["codex_preflight"])
        self.assertEqual(["scripts/planner_wsh_worker.sh", "{wsh_server_name}", "{wsh_session_id}", "assignment-{assignment_id}", "lease-{lease_id}", "{worktree}", "{assignment_prompt}"], commands["worker"])
        worker_adapter = Path(__file__).with_name("planner_wsh_worker.sh").read_text(encoding="utf-8")
        self.assertIn("codex --cd", worker_adapter)
        self.assertIn(" attach ", worker_adapter)
        self.assertNotIn("--agent", worker_adapter)

    def test_actual_codex_help_uses_supported_interactive_and_exec_grammar(self) -> None:
        codex = shutil.which("codex")
        if not codex:
            self.skipTest("Codex CLI is not installed in this test environment")
        result = subprocess.run([codex, "--help"], capture_output=True, text=True, check=False, timeout=10)
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("[PROMPT]", result.stdout)
        self.assertIn("exec", result.stdout)
        self.assertNotIn("--agent", result.stdout)

    def test_planner_created_assignment_prompt_makes_worker_and_qa_controls_effective(self) -> None:
        prompt = self.control._assignment_prompt("573", "quality_engineer")
        for required in (
            "Accept task intake only from sprint_planner",
            "assigned external worktree",
            "Do not launch, attach to, inspect, or steer tmux or WSH sessions",
            "do not use WSH MCP",
            "Report progress, evidence, blockers, and every required decision to sprint_planner as MANAGER ATTENTION NEEDED",
            "independently leased QA worktree",
            "never share an implementation worktree",
            "Create a focused PR linked to this issue",
            "monitor PR and issue feedback",
            "action every actionable review or issue comment",
            "linked out-of-scope follow-up issue only with authority",
            "otherwise escalate it to sprint_planner as MANAGER ATTENTION NEEDED",
            "required checks, CODEOWNERS review where applicable, no unresolved blocker, and rollback review",
            "explicitly authorize the exact PR head and named target",
            "binding issue comment with PR, head, checks, QA, rollback, and merge authority evidence",
            "Do not remove or delete the worktree or its local branch after merge",
            "GitHub confirms the authorized head merged into the named target",
            "planner-controlled cleanup path",
            "separately recorded explicit cleanup authority",
            "clean, unpushed, unleased, and not needed",
            "server-identity validation fails",
            "Only the planner uses WSH",
        ):
            self.assertIn(required, prompt)

    def test_closeout_contract_is_consistent_in_effective_prompt_and_role_policies(self) -> None:
        repository = Path(__file__).resolve().parent.parent
        sources = {
            "effective worker prompt": self.control._assignment_prompt("573", "software_engineer"),
            "effective planner prompt": self.control._planner_assignment_prompt(),
            "software engineer policy": (repository / ".codex" / "agents" / "software_engineer.toml").read_text(encoding="utf-8"),
            "sprint planner policy": (repository / ".codex" / "agents" / "sprint_planner.toml").read_text(encoding="utf-8"),
            "repository policy": (repository / "AGENTS.md").read_text(encoding="utf-8"),
            "control-plane runbook": (repository / "docs" / "PLANNER_WSH_CONTROL_PLANE.md").read_text(encoding="utf-8"),
        }
        for name, source in sources.items():
            self.assertIn("MANAGER ATTENTION NEEDED", source, name)
            self.assertIn("GitHub confirms", source, name)
            self.assertIn("local branch", source, name)
            self.assertIn("planner-controlled cleanup path", source, name)
            self.assertIn("explicit cleanup authority", source, name)
        self.assertIn("Do not remove or delete", sources["effective worker prompt"])

    def test_deterministic_names_survive_control_plane_recovery(self) -> None:
        self.control.bootstrap()
        first = self.control.launch_worker("573", "software_engineer", self.worktree)
        recovered = ControlPlane(Profile.load(self.profile_path), self.runner, self.root).status()
        self.assertEqual("router-planner", recovered["tmux_session"])
        self.assertEqual("router-planner-session", recovered["planner_wsh_session_id"])
        self.assertEqual("router-worker-573-software_engineer", first["workers"][0]["wsh_session_id"])
        self.assertEqual(first["workers"][0]["wsh_session_id"], recovered["workers"][0]["wsh_session_id"])

    def test_repository_common_registry_rejects_cross_profile_and_aba_release(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        raw = json.loads(self.profile_path.read_text(encoding="utf-8"))
        raw["profile_id"] = "second-planner"
        raw["state_directory"] = str(self.root.parent / "second-state")
        second_path = self.root.parent / "second-profile.json"
        second_path.write_text(json.dumps(raw), encoding="utf-8"); second_path.chmod(0o600)
        second = ControlPlane(Profile.load(second_path), self.runner, self.root)
        worktree_id = self.control.status()["workers"][0]["worktree_id"]
        with self.assertRaisesRegex(ControlPlaneError, "repository-common lease"):
            second._reserve_shared_lease(worktree_id, "573-quality_engineer")
        worker = json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"]["573-software_engineer"]
        with self.assertRaisesRegex(ControlPlaneError, "ownership is ambiguous"):
            second._release_shared_lease(worktree_id, worker["lease_id"], worker["registry_generation"])

    def test_repository_shutdown_stopping_serializes_a_concurrent_allocation(self) -> None:
        self.control.bootstrap()
        self.control._begin_repository_shutdown()
        with self.assertRaisesRegex(ControlPlaneError, "drain requires explicit recovery"):
            self.control._reserve_shared_lease("worktree-race", "573-software_engineer")
        self.control._finish_repository_shutdown()

    def test_status_is_bounded_validates_lease_and_never_runs_under_state_lock(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        lock_acquired = []

        def acquire_lock() -> None:
            with self.control.lock_path.open("a+", encoding="utf-8") as handle:
                fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                lock_acquired.append(True)
                fcntl.flock(handle.fileno(), fcntl.LOCK_UN)

        self.runner.status_hook = acquire_lock
        self.runner.status_payload = {"session_id": "wrong", "assignment_id": "573-software_engineer", "state": "idle"}
        status = self.control.status()
        self.assertEqual({"state": "status-unavailable"}, status["workers"][0]["wsh"])
        self.assertTrue(lock_acquired)
        self.assertEqual(2, [kwargs["timeout_seconds"] for call, kwargs in zip(self.runner.calls, self.runner.call_kwargs) if call[0] == "wsh" and call[-1] == "list"][-1])
        self.assertEqual(16384, [kwargs["max_output_bytes"] for call, kwargs in zip(self.runner.calls, self.runner.call_kwargs) if call[0] == "wsh" and call[-1] == "list"][-1])
        self.runner.status_payload = {"session_id": "router-worker-573-software_engineer", "assignment_id": "573-software_engineer", "state": "x" * 257}
        self.assertEqual("status-unavailable", self.control.status()["workers"][0]["wsh"]["state"])
        self.runner.status_result = RunResult(124)
        self.assertEqual("status-unavailable", self.control.status()["workers"][0]["wsh"]["state"])
        self.runner.status_result = RunResult(125)
        self.assertEqual("status-unavailable", self.control.status()["workers"][0]["wsh"]["state"])

    def test_runner_enforces_timeout_and_output_caps(self) -> None:
        self.assertEqual(124, Runner().run([sys.executable, "-c", "import time; time.sleep(2)"], check=False, timeout_seconds=1).returncode)
        self.assertEqual(125, Runner().run([sys.executable, "-c", "print('x' * 4096)"], check=False, max_output_bytes=256).returncode)
        self.assertEqual(125, Runner().run([sys.executable, "-c", "import sys; print('x' * 200); print('y' * 200, file=sys.stderr)"], check=False, max_output_bytes=300).returncode)
        started = time.monotonic()
        result = Runner().run(
            [
                sys.executable,
                "-c",
                "import subprocess, sys, time; subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(10)']); time.sleep(10)",
            ],
            check=False,
            timeout_seconds=1,
        )
        self.assertEqual(124, result.returncode)
        self.assertLess(time.monotonic() - started, 2.5, "inherited pipes must not extend the timeout")
        started = time.monotonic()
        result = Runner().run(
            [
                sys.executable,
                "-c",
                "import subprocess, sys; subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(10)'])",
            ],
            check=False,
            timeout_seconds=1,
        )
        self.assertEqual(124, result.returncode)
        self.assertLess(time.monotonic() - started, 2.5, "parent exit must not wait for a descendant-held pipe")
        escaped_pid_file = self.root.parent / "escaped-child.pid"
        runner_dir = Path(__file__).parent
        parent_code = (
            "import pathlib, subprocess, sys; "
            "child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(10)'], start_new_session=True); "
            f"pathlib.Path({str(escaped_pid_file)!r}).write_text(str(child.pid))"
        )
        outer_code = (
            "import sys; "
            f"sys.path.insert(0, {str(runner_dir)!r}); "
            "from planner_wsh_control import Runner; "
            f"print(Runner().run([sys.executable, '-c', {parent_code!r}], check=False, timeout_seconds=1).returncode)"
        )
        started = time.monotonic()
        outer = subprocess.run([sys.executable, "-c", outer_code], capture_output=True, text=True, timeout=2.5, check=False)
        self.assertEqual(0, outer.returncode)
        self.assertEqual("124", outer.stdout.strip())
        self.assertLess(time.monotonic() - started, 2.5, "outer interpreter must not wait for escaped pipe holders")
        if escaped_pid_file.exists():
            escaped_pid = int(escaped_pid_file.read_text(encoding="utf-8"))
            try:
                os.killpg(escaped_pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    def test_audit_is_append_only_across_failed_lifecycle_and_shutdown(self) -> None:
        self.control.bootstrap()
        self.runner.fail_new_window = True
        with self.assertRaisesRegex(ControlPlaneError, "simulated tmux"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.fail_new_window = False
        # A create-call failure is intentionally start-uncertain; prove its
        # exact absence through normal release before retrying the assignment.
        self.control.release_worker("573", "software_engineer", outcome="abandoned", cleanup_authorized=True)
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.fail_stop = True
        with self.assertRaisesRegex(ControlPlaneError, "simulated WSH"):
            self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        events_before = [json.loads(line) for line in self.control.audit_path.read_text(encoding="utf-8").splitlines()]
        self.assertTrue(any(event["action"] == "worker-started" and event["outcome"] == "uncertain" for event in events_before))
        self.assertTrue(any(event["action"] == "worker-released" and event["outcome"] == "failed" for event in events_before))
        self.runner.fail_stop = False
        self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        self.control.shutdown()
        events_after = [json.loads(line) for line in self.control.audit_path.read_text(encoding="utf-8").splitlines()]
        self.assertGreater(len(events_after), len(events_before))
        self.assertEqual(events_before, events_after[:len(events_before)])
        self.assertTrue(all({"event_id", "at", "action", "outcome"} <= set(event) for event in events_after))

    def test_failed_wsh_preflight_releases_reservation_before_terminal_creation(self) -> None:
        self.control.bootstrap()
        self.runner.status_result = RunResult(1)
        with self.assertRaisesRegex(ControlPlaneError, "planner WSH identity is list-unavailable"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertNotIn("agent-573-software_engineer", self.runner.windows)
        state = json.loads(self.control.state_path.read_text(encoding="utf-8"))
        self.assertEqual({}, state["workers"])
        self.assertFalse(self.control.audit_path.exists() and "worker-preflight" in self.control.audit_path.read_text(encoding="utf-8"))

    def test_failed_codex_preflight_and_spawn_error_release_reservation_before_terminal_creation(self) -> None:
        self.control.bootstrap()
        self.runner.fail_codex_preflight = True
        with self.assertRaisesRegex(ControlPlaneError, "Codex invocation preflight"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertEqual({}, json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        self.runner.fail_codex_preflight = False
        self.runner.fail_status_spawn = True
        with self.assertRaisesRegex(ControlPlaneError, "planner WSH identity is list-unavailable"):
            self.control.launch_worker("573", "software_engineer", self.worktree)
        self.assertEqual({}, json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        events = [json.loads(line) for line in self.control.audit_path.read_text(encoding="utf-8").splitlines()]
        self.assertTrue(any(event["action"] == "worker-codex-preflight" and event["outcome"] == "failed" for event in events))

    def test_runner_normalizes_missing_executable(self) -> None:
        with self.assertRaisesRegex(ControlPlaneError, "unable to start command"):
            Runner().run(["definitely-not-an-installed-command-573"], check=False)

    def test_completed_worker_is_released_only_after_exact_session_and_window_are_absent(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.wsh_sessions = {"router-planner-session": "planner-local-planner"}
        self.runner.windows.remove("agent-573-software_engineer")
        self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        self.assertEqual({}, json.loads(self.control.state_path.read_text(encoding="utf-8"))["workers"])
        self.control.shutdown()

    def test_release_retains_lease_when_completed_session_is_ambiguous_or_window_remains(self) -> None:
        self.control.bootstrap()
        self.control.launch_worker("573", "software_engineer", self.worktree)
        self.runner.wsh_sessions = {"router-planner-session": "planner-local-planner"}
        with self.assertRaisesRegex(ControlPlaneError, "tmux worker window remains"):
            self.control.release_worker("573", "software_engineer", outcome="merged", cleanup_authorized=True)
        state = json.loads(self.control.state_path.read_text(encoding="utf-8"))
        self.assertEqual("release-failed", state["workers"]["573-software_engineer"]["state"])

    def test_profile_rejects_symlink_permissions_shell_nested_tmux_non_wsh_and_network_templates(self) -> None:
        raw = json.loads(self.profile_path.read_text(encoding="utf-8"))
        missing_identity = json.loads(json.dumps(raw))
        missing_identity.pop("wsh_server_identity")
        self.profile_path.write_text(json.dumps(missing_identity), encoding="utf-8")
        self.profile_path.chmod(0o600)
        with self.assertRaisesRegex(ControlPlaneError, "wsh_server_identity"):
            Profile.load(self.profile_path)
        for value in (None, True, 0, 11):
            changed = json.loads(json.dumps(raw))
            if value is None:
                changed.pop("bootstrap_identity_wait_seconds")
            else:
                changed["bootstrap_identity_wait_seconds"] = value
            self.profile_path.write_text(json.dumps(changed), encoding="utf-8")
            self.profile_path.chmod(0o600)
            with self.assertRaisesRegex(ControlPlaneError, "bootstrap_identity_wait_seconds"):
                Profile.load(self.profile_path)
        self.profile_path.write_text(json.dumps(raw), encoding="utf-8")
        self.profile_path.chmod(0o644)
        with self.assertRaisesRegex(ControlPlaneError, "mode 0600"):
            Profile.load(self.profile_path)
        self.profile_path.chmod(0o600)
        linked = self.profile_path.with_name("profile-link.json")
        linked.symlink_to(self.profile_path)
        with self.assertRaisesRegex(ControlPlaneError, "non-symlink"):
            Profile.load(linked)
        for command, expected in (
            (["sh", "-c", "unsafe-command"], "shell wrapper"),
            (["tmux", "new-session"], "nested tmux"),
            (["not-wsh", "server"], "wsh server"),
            (["wsh", "server", "serve", "--listen", "0.0.0.0"], "network-exposed"),
            (["wsh", "session", "run", "--name", "not-a-server"], "wsh server"),
        ):
            changed = json.loads(json.dumps(raw))
            changed["commands"]["wsh_server"] = command
            self.profile_path.write_text(json.dumps(changed), encoding="utf-8")
            self.profile_path.chmod(0o600)
            with self.assertRaisesRegex(ControlPlaneError, expected):
                Profile.load(self.profile_path)
        changed = json.loads(json.dumps(raw))
        changed["commands"]["wsh_identity"] = ["wsh", "-L", "{wsh_server_name}", "list"]
        self.profile_path.write_text(json.dumps(changed), encoding="utf-8")
        self.profile_path.chmod(0o600)
        with self.assertRaisesRegex(ControlPlaneError, "authoritative JSON identity"):
            Profile.load(self.profile_path)
        changed = json.loads(json.dumps(raw))
        changed["commands"]["worker"][0] = "not-an-approved-adapter"
        self.profile_path.write_text(json.dumps(changed), encoding="utf-8")
        self.profile_path.chmod(0o600)
        with self.assertRaisesRegex(ControlPlaneError, "approved WSH-to-Codex"):
            Profile.load(self.profile_path)

    def test_launcher_contains_no_network_or_socket_locator(self) -> None:
        source = Path(__file__).with_name("planner_wsh_control.py").read_text(encoding="utf-8")
        self.assertNotIn("localhost", source)
        self.assertNotIn(".sock", source)

    def test_all_role_contracts_require_one_planner_identity_and_no_codex_agent_flag(self) -> None:
        repository = Path(__file__).resolve().parent.parent
        role_configs = (
            "sprint_planner.toml",
            "software_engineer.toml",
            "quality_engineer.toml",
            "system_architect.toml",
            "product-manager.toml",
            "interaction_designer.toml",
            "eks_operations.toml",
        )
        for name in role_configs:
            source = (repository / ".codex" / "agents" / name).read_text(encoding="utf-8")
            self.assertIn("WSH assignment identity:", source, name)
            self.assertIn("duplicate in-process planner", source, name)
            self.assertIn("bounded WSH CLI/REST", source, name)
            self.assertIn("Codex CLI selector", source, name)
            self.assertIn("codex --agent", source, name)
            self.assertIn("merge", source, name)
            self.assertIn("cleanup", source, name)
        instructions = (repository / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("WSH assignment identity:", instructions)
        self.assertIn("exactly one authoritative WSH-hosted `sprint_planner` identity", instructions)
        self.assertIn("cooperative and mutually trusted", instructions)
        self.assertIn("accidental-access hygiene only", instructions)
        self.assertIn("separate authenticated OS or service identity", instructions)


if __name__ == "__main__":
    unittest.main()
