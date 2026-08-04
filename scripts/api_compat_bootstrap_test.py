#!/usr/bin/env python3
"""Regression coverage for API-compatibility dependency provisioning."""

from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
API_COMPAT_ROOT = ROOT / "tests" / "api_compat"
REPOSITORY_RESIDUE = (
    API_COMPAT_ROOT / ".venv",
    API_COMPAT_ROOT / ".pytest_cache",
    API_COMPAT_ROOT / "tests" / "__pycache__",
)


def run_make(target: str, env: dict[str, str], *, expected: int = 0) -> str:
    completed = subprocess.run(
        ["make", target],
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if completed.returncode != expected:
        raise AssertionError(
            f"make {target} returned {completed.returncode}, expected {expected}\\n"
            f"{completed.stdout}"
        )
    return completed.stdout


def isolated_environment(root: Path) -> dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "UV_CACHE_DIR": str(root / "uv-cache"),
            "UV_PROJECT_ENVIRONMENT": str(root / "venv"),
            "GOMODCACHE": str(root / "go-mod-cache"),
            "GOCACHE": str(root / "go-build-cache"),
        }
    )
    # These must not accidentally make a clean-cache bootstrap pass through a
    # caller's already-offline environment. The Make bootstrap target supplies
    # its approved Go module settings explicitly.
    env.pop("UV_OFFLINE", None)
    env.pop("GOPROXY", None)
    env.pop("GOSUMDB", None)
    return env


def assert_no_repository_residue() -> None:
    residue = [str(path.relative_to(ROOT)) for path in REPOSITORY_RESIDUE if path.exists()]
    if residue:
        raise AssertionError(f"API compatibility run left repository-local residue: {', '.join(residue)}")


def main() -> int:
    assert_no_repository_residue()
    with tempfile.TemporaryDirectory(prefix="api-compat-bootstrap-") as temporary:
        cache_root = Path(temporary)
        env = isolated_environment(cache_root)

        # An isolated empty cache proves that the conformance phase cannot
        # silently download a missing Python or Go prerequisite.
        missing_output = run_make("api-compat-mock-offline", env, expected=2)
        if "offline" not in missing_output.lower():
            raise AssertionError("clean-cache offline failure did not report offline dependency resolution")

        run_make("api-compat-bootstrap", env)
        if not any((cache_root / "uv-cache").iterdir()):
            raise AssertionError("locked Python bootstrap did not populate its isolated cache")
        if not any((cache_root / "go-mod-cache").iterdir()):
            raise AssertionError("Go bootstrap did not populate its isolated module cache")

        offline_env = env | {"UV_OFFLINE": "1", "GOPROXY": "off", "GOSUMDB": "off"}
        run_make("api-compat-mock-offline", offline_env)

        # The test uses only disposable caches and must leave the worktree
        # unchanged by Python environments, pytest cache, or bytecode cache.
        assert_no_repository_residue()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
