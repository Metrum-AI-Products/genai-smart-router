#!/usr/bin/env python3
"""Process-level launch-worker versus repository-drain proof for issue #576."""

from __future__ import annotations

import json
import multiprocessing
from pathlib import Path
import tempfile
import unittest

from planner_wsh_control import ControlPlane, ControlPlaneError, Profile, RunResult


class ConcurrentRunner:
    """Per-process tmux/WSH model; the real shared surface is the Git registry."""

    def __init__(self, root: Path, worktree: Path, profile: dict[str, object]):
        self.root = root
        self.worktree = worktree
        self.profile = profile
        self.tmux_exists = True
        self.windows = {"wsh-server", "sprint-planner"}
        self.sessions = {str(profile["planner_wsh_session_id"]): f"planner-{profile['profile_id']}"}

    def run(self, argv: list[str], *, check: bool = True, **_: object) -> RunResult:
        if argv[0] == "git":
            cwd = Path(argv[2]); args = argv[3:]
            if args == ["rev-parse", "--show-toplevel"]:
                return RunResult(0, str(cwd) + "\n")
            if args == ["rev-parse", "--path-format=absolute", "--git-common-dir"]:
                return RunResult(0, str(self.root / ".git") + "\n")
            if args == ["worktree", "list", "--porcelain"]:
                return RunResult(0, f"worktree {self.root}\n\nworktree {self.worktree}\nbranch refs/heads/issue-576\n\n")
            if args == ["branch", "--show-current"]:
                return RunResult(0, "issue-576-lifecycle-recovery\n")
            if args[:1] == ["merge-base"]:
                return RunResult(0, "base\n")
            raise AssertionError(argv)
        if argv[:2] == ["tmux", "has-session"]:
            return RunResult(0 if self.tmux_exists else 1)
        if argv[:2] == ["tmux", "list-windows"]:
            return RunResult(0 if self.tmux_exists else 1, "\n".join(sorted(self.windows)) + "\n")
        if argv[:2] == ["tmux", "new-window"]:
            self.windows.add(argv[argv.index("-n") + 1]); return RunResult(0)
        if argv[:2] == ["tmux", "kill-window"]:
            self.windows.discard(argv[-1].split(":", 1)[1]); return RunResult(0)
        if argv[:2] == ["tmux", "kill-session"]:
            self.tmux_exists = False; self.windows.clear(); return RunResult(0)
        if argv[0] == "wsh" and argv[-2:] == ["identity", "--json"]:
            # The race fixture models the required authoritative handshake;
            # it never contacts a WSH server or uses runtime credentials.
            return RunResult(0, json.dumps({"server_identity": self.profile["wsh_server_identity"]}))
        if argv[0] == "wsh" and argv[-1] == "list":
            return RunResult(0, "\n".join(f"{name}\nTAGS {tag}" for name, tag in self.sessions.items()))
        if argv[0] == "wsh" and "tag" in argv:
            return RunResult(0, f"Session '{argv[-1]}': {self.sessions.get(argv[-1], '')}")
        if argv[0] == "wsh" and "kill" in argv:
            self.sessions.pop(argv[-1], None); return RunResult(0)
        if argv == ["codex", "--help"]:
            return RunResult(0, "Usage: codex [PROMPT]\nCommands:\n exec\n")
        raise AssertionError(argv)


class BarrierControlPlane(ControlPlane):
    def __init__(self, *args: object, barrier: multiprocessing.synchronize.Barrier, **kwargs: object):
        super().__init__(*args, **kwargs)
        self.barrier = barrier

    def _reserve_shared_lease(self, worktree_id: str, assignment_id: str) -> tuple[str, int]:
        self.barrier.wait(timeout=10)
        return super()._reserve_shared_lease(worktree_id, assignment_id)

    def _begin_repository_shutdown(self) -> None:
        self.barrier.wait(timeout=10)
        super()._begin_repository_shutdown()


def _launch_target(profile_path: str, root: str, worktree: str, raw_profile: dict[str, object], barrier: multiprocessing.synchronize.Barrier, queue: multiprocessing.queues.Queue[tuple[str, str, bool]]) -> None:
    runner = ConcurrentRunner(Path(root), Path(worktree), raw_profile)
    control = BarrierControlPlane(Profile.load(Path(profile_path)), runner, Path(root), barrier=barrier)
    try:
        control.launch_worker("576", "software_engineer", Path(worktree))
        queue.put(("launch", "ok", "agent-576-software_engineer" in runner.windows))
    except Exception as exc:  # Result is asserted by the parent process.
        queue.put(("launch", str(exc), "agent-576-software_engineer" in runner.windows))


def _drain_target(profile_path: str, root: str, worktree: str, raw_profile: dict[str, object], barrier: multiprocessing.synchronize.Barrier, queue: multiprocessing.queues.Queue[tuple[str, str, bool]]) -> None:
    runner = ConcurrentRunner(Path(root), Path(worktree), raw_profile)
    control = BarrierControlPlane(Profile.load(Path(profile_path)), runner, Path(root), barrier=barrier)
    try:
        control.shutdown()
        queue.put(("drain", "ok", "agent-576-software_engineer" in runner.windows))
    except Exception as exc:
        queue.put(("drain", str(exc), "agent-576-software_engineer" in runner.windows))


class ConcurrentLifecycleTest(unittest.TestCase):
    def test_cross_profile_full_launch_vs_drain_has_one_linearized_outcome(self) -> None:
        context = multiprocessing.get_context("fork")
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary); root = base / "repo"; worktree = base / "issue-576"
            root.mkdir(); (root / ".git").mkdir(); worktree.mkdir()

            def profile(profile_id: str) -> dict[str, object]:
                return {
                    "profile_id": profile_id, "planner_owner": "sprint_planner",
                    "tmux_session": f"router-{profile_id}", "wsh_server_name": f"router-{profile_id}",
                    "wsh_server_identity": f"router-{profile_id}-server", "bootstrap_identity_wait_seconds": 1,
                    "planner_wsh_session_id": f"router-{profile_id}-planner", "state_directory": str(base / f"state-{profile_id}"),
                    "worker_session_prefix": f"router-{profile_id}-worker", "base_ref": "origin/main", "lease_ttl_seconds": 3600,
                    "forbidden_environment_variables": ["WSH_SESSION_ID"],
                    "commands": {
                        "wsh_server": ["wsh", "-L", "{wsh_server_name}", "server"],
                        "wsh_identity": ["wsh", "-L", "{wsh_server_name}", "identity", "--json"],
                        "planner": ["scripts/planner_wsh_planner.sh", "{wsh_server_name}", "{planner_wsh_session_id}", "planner-{profile_id}", "{repository_root}", "{planner_assignment_prompt}"],
                        "worker": ["scripts/planner_wsh_worker.sh", "{wsh_server_name}", "{wsh_session_id}", "assignment-{assignment_id}", "lease-{lease_id}", "{worktree}", "{assignment_prompt}"],
                        "codex_preflight": ["codex", "--help"], "wsh_stop": ["wsh", "-L", "{wsh_server_name}", "kill", "{wsh_session_id}"],
                        "planner_stop": ["wsh", "-L", "{wsh_server_name}", "kill", "{planner_wsh_session_id}"],
                        "planner_tag": ["wsh", "-L", "{wsh_server_name}", "tag", "{planner_wsh_session_id}"], "wsh_status": ["wsh", "-L", "{wsh_server_name}", "list"],
                    },
                }

            first, second = profile("first"), profile("second")
            first_path, second_path = base / "first.json", base / "second.json"
            for path, raw in ((first_path, first), (second_path, second)):
                path.write_text(json.dumps(raw), encoding="utf-8"); path.chmod(0o600)
            # Bootstrap independent profile state before starting the exact race.
            ControlPlane(Profile.load(first_path), ConcurrentRunner(root, worktree, first), root).bootstrap()
            ControlPlane(Profile.load(second_path), ConcurrentRunner(root, worktree, second), root).bootstrap()
            barrier, queue = context.Barrier(2), context.Queue()
            launch = context.Process(target=_launch_target, args=(str(first_path), str(root), str(worktree), first, barrier, queue))
            drain = context.Process(target=_drain_target, args=(str(second_path), str(root), str(worktree), second, barrier, queue))
            launch.start(); drain.start(); launch.join(15); drain.join(15)
            self.assertEqual(0, launch.exitcode); self.assertEqual(0, drain.exitcode)
            first_result, second_result = queue.get(timeout=2), queue.get(timeout=2)
            results = {first_result[0]: first_result, second_result[0]: second_result}
            inspect = ControlPlane(Profile.load(first_path), ConcurrentRunner(root, worktree, first), root)
            with inspect._locked_registry() as registry:
                leases = dict(registry["leases"]); drain_state = registry["drain"]
            if results["launch"][1] == "ok":
                self.assertIn("worker leases exist", results["drain"][1])
                self.assertTrue(results["launch"][2]); self.assertEqual(1, len(leases)); self.assertIsNone(drain_state)
                worker = json.loads(inspect.state_path.read_text(encoding="utf-8"))["workers"]["576-software_engineer"]
                inspect._release_shared_lease(worker["worktree_id"], worker["lease_id"], worker["registry_generation"])
            else:
                self.assertEqual("ok", results["drain"][1])
                self.assertIn("drain requires explicit recovery", results["launch"][1])
                self.assertFalse(results["launch"][2]); self.assertEqual({}, leases); self.assertIsNone(drain_state)


if __name__ == "__main__":
    unittest.main()
