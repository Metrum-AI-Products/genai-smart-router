#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Gate release package builds on a clean git tree unless explicitly overridden."""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path


def run_git(repo: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=Path.cwd(), type=Path)
    args = parser.parse_args()

    if os.environ.get("ALLOW_DIRTY_PACKAGE") == "1":
        print("release clean-tree validation skipped because ALLOW_DIRTY_PACKAGE=1")
        return 0

    repo = args.repo.resolve()
    version = os.environ.get("VERSION", "")
    errors: list[str] = []

    if "-dirty" in version:
        errors.append("VERSION contains -dirty; set ALLOW_DIRTY_PACKAGE=1 only for local development packages")

    inside = run_git(repo, "rev-parse", "--is-inside-work-tree")
    if inside.returncode != 0 or inside.stdout.strip() != "true":
        errors.append(f"{repo} is not a git work tree")
    else:
        status = run_git(repo, "status", "--porcelain=v1", "--untracked-files=all")
        if status.returncode != 0:
            errors.append("git status failed while checking release package cleanliness")
        elif status.stdout.strip():
            errors.append(
                "working tree has uncommitted changes; commit them before release packaging "
                "or set ALLOW_DIRTY_PACKAGE=1 for a local development package"
            )

    if errors:
        print("release clean-tree validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 2

    print("release clean-tree validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
