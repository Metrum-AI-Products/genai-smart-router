#!/usr/bin/env python3
"""Self-test release clean-tree validation."""

from __future__ import annotations

import os
import subprocess
import sys
import tempfile
from pathlib import Path


SCRIPT = Path(__file__).resolve().with_name("validate_release_clean.py")


def run(repo: Path, *, version: str = "v1.0.0", allow_dirty: bool = False) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env["VERSION"] = version
    if allow_dirty:
        env["ALLOW_DIRTY_PACKAGE"] = "1"
    else:
        env.pop("ALLOW_DIRTY_PACKAGE", None)
    return subprocess.run(
        [sys.executable, str(SCRIPT), "--repo", str(repo)],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        check=False,
    )


def git(repo: Path, *args: str) -> None:
    completed = subprocess.run(["git", "-C", str(repo), *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if completed.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed:\n{completed.stdout}\n{completed.stderr}")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    with tempfile.TemporaryDirectory() as temp:
        repo = Path(temp)
        git(repo, "init")
        git(repo, "config", "user.email", "test@example.com")
        git(repo, "config", "user.name", "Package Test")
        (repo / "README.md").write_text("package test\n", encoding="utf-8")
        git(repo, "add", "README.md")
        git(repo, "commit", "-m", "initial")

        clean = run(repo)
        require(clean.returncode == 0, f"clean repo rejected:\n{clean.stdout}\n{clean.stderr}")

        dirty_version = run(repo, version="v1.0.0-dirty")
        require(dirty_version.returncode != 0, "dirty VERSION accepted")
        require("VERSION contains -dirty" in dirty_version.stderr, f"missing dirty VERSION error: {dirty_version.stderr}")

        (repo / "README.md").write_text("changed\n", encoding="utf-8")
        dirty_tree = run(repo)
        require(dirty_tree.returncode != 0, "dirty tree accepted")
        require("working tree has uncommitted changes" in dirty_tree.stderr, f"missing dirty tree error: {dirty_tree.stderr}")

        git(repo, "checkout", "--", "README.md")
        git(repo, "config", "status.showUntrackedFiles", "no")
        (repo / "untracked.txt").write_text("untracked input\n", encoding="utf-8")
        hidden_untracked = run(repo)
        require(hidden_untracked.returncode != 0, "untracked file hidden by git config accepted")
        require(
            "working tree has uncommitted changes" in hidden_untracked.stderr,
            f"missing hidden untracked error: {hidden_untracked.stderr}",
        )

        override = run(repo, version="v1.0.0-dirty", allow_dirty=True)
        require(override.returncode == 0, f"ALLOW_DIRTY_PACKAGE override rejected:\n{override.stdout}\n{override.stderr}")

    print("release clean-tree validation self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
