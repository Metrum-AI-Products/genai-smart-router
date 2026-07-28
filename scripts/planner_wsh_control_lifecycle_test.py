#!/usr/bin/env python3
"""Live, local-only WSH/tmux completed-worker regression for issue #573.

This deliberately uses a temporary fake `codex` that accepts `--help` and
exits immediately. It exercises the actual adapter/session/window lifecycle
without creating an unbounded Codex task or using a provider credential.
"""

from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import uuid


ROOT = Path(__file__).resolve().parent.parent
CONTROL = ROOT / "scripts" / "planner_wsh_control.py"


def run(argv: list[str], *, env: dict[str, str], check: bool = True, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(argv, cwd=cwd or ROOT, env=env, text=True, capture_output=True, check=False, timeout=30)
    if check and result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(argv)}; stderr={result.stderr.strip()}")
    return result


def main() -> int:
    if not shutil.which("wsh") or not shutil.which("tmux"):
        print("SKIP: wsh and tmux are required for the live lifecycle regression")
        return 0
    marker = uuid.uuid4().hex[:12]
    session = f"issue573live{marker}"
    branch = f"issue-573-live-{marker}"
    with tempfile.TemporaryDirectory(prefix="planner-wsh-live-") as temporary:
        base = Path(temporary)
        repository = base / "repo"
        worktree = base / "worktree"
        state = base / "state"
        fake_bin = base / "bin"
        fake_bin.mkdir()
        fake_codex = fake_bin / "codex"
        fake_codex.write_text(
            "#!/usr/bin/env bash\n"
            "if [[ ${1:-} == --help ]]; then echo 'Usage: codex [OPTIONS] [PROMPT]'; exit 0; fi\n"
            "if [[ ${1:-} == --cd && ${3:-} == *sole\\ sprint_planner* ]]; then exec sleep 300; fi\n"
            "if [[ ${1:-} == --cd ]]; then exit 0; fi\n"
            "exec sleep 300\n",
            encoding="utf-8",
        )
        fake_codex.chmod(0o755)
        # Isolate the Git common directory as well as state: a failed lifecycle
        # test must never leave a drain record in the developer's repository.
        bootstrap_env = os.environ.copy()
        run(["git", "init", str(repository)], env=bootstrap_env)
        run(["git", "-C", str(repository), "config", "user.email", "planner-wsh-test@example.invalid"], env=bootstrap_env)
        run(["git", "-C", str(repository), "config", "user.name", "Planner WSH Test"], env=bootstrap_env)
        run(["git", "-C", str(repository), "commit", "--allow-empty", "-m", "test root"], env=bootstrap_env)
        (repository / "scripts").symlink_to(CONTROL.parent, target_is_directory=True)
        profile = base / "profile.json"
        profile.write_text(
            json.dumps(
                {
                    "profile_id": f"issue573-{marker}",
                    "planner_owner": "sprint_planner",
                    "tmux_session": session,
                    "wsh_server_name": session,
                    "planner_wsh_session_id": f"planner{marker}",
                    "state_directory": str(state),
                    "worker_session_prefix": f"worker{marker}",
                    "base_ref": None,
                    "lease_ttl_seconds": 3600,
                    "status_timeout_seconds": 2,
                    "status_output_bytes": 16384,
                    "status_field_bytes": 256,
                    "forbidden_environment_variables": ["WSH_SESSION_ID"],
                    "commands": {
                        "wsh_server": ["wsh", "-L", "{wsh_server_name}", "server"],
                        "planner": [
                            "scripts/planner_wsh_planner.sh", "{wsh_server_name}", "{planner_wsh_session_id}",
                            "planner-{profile_id}", "{repository_root}", "{planner_assignment_prompt}",
                        ],
                        "worker": [
                            "scripts/planner_wsh_worker.sh", "{wsh_server_name}", "{wsh_session_id}",
                            "assignment-{assignment_id}", "lease-{lease_id}", "{worktree}", "{assignment_prompt}",
                        ],
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
        profile.chmod(0o600)
        env = dict(os.environ, PATH=f"{fake_bin}{os.pathsep}{os.environ.get('PATH', '')}")
        run(["git", "worktree", "add", "-b", branch, str(worktree), "HEAD"], env=env, cwd=repository)
        (worktree / "scripts").symlink_to(CONTROL.parent, target_is_directory=True)
        try:
            run(["python3", str(CONTROL), "--profile", str(profile), "bootstrap"], env=env, cwd=repository)
            preflight = run(["python3", str(CONTROL), "--profile", str(profile), "preflight"], env=env, cwd=repository)
            if json.loads(preflight.stdout) != {"codex": "accepted", "planner_wsh": "present"}:
                raise RuntimeError("bounded preflight did not accept local WSH/Codex surfaces")
            run(
                ["python3", str(CONTROL), "--profile", str(profile), "launch-worker", "--issue", "573", "--role", "software_engineer", "--worktree", str(worktree)],
                env=env, cwd=repository,
            )
            worker = f"worker{marker}-573-software_engineer"
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                listed = run(["wsh", "-L", session, "list"], env=env, check=False, cwd=repository)
                windows = run(["tmux", "list-windows", "-t", session, "-F", "#{window_name}"], env=env, check=False, cwd=repository)
                if worker not in listed.stdout and "agent-573-software_engineer" not in windows.stdout:
                    break
                time.sleep(0.1)
            else:
                raise RuntimeError("completed fake Codex worker did not remove its exact WSH session and tmux window")
            released = run(
                ["python3", str(CONTROL), "--profile", str(profile), "release-worker", "--issue", "573", "--role", "software_engineer", "--outcome", "merged", "--cleanup-authorized"],
                env=env, cwd=repository,
            )
            if json.loads(released.stdout)["workers"]:
                raise RuntimeError("completed worker lease was not reclaimed")
            run(["python3", str(CONTROL), "--profile", str(profile), "shutdown", "--confirm-shutdown"], env=env, cwd=repository)
            print("PASS: completed fake Codex worker session/window were verified absent and lease released")
            return 0
        finally:
            run(["tmux", "kill-session", "-t", session], env=env, check=False, cwd=repository)
            run(["git", "worktree", "remove", "--force", str(worktree)], env=env, check=False, cwd=repository)
            run(["git", "branch", "-D", branch], env=env, check=False, cwd=repository)


if __name__ == "__main__":
    raise SystemExit(main())
