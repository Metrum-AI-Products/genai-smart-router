#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression coverage for API-compatibility dependency provisioning."""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
API_COMPAT_ROOT = ROOT / "tests" / "api_compat"
REPOSITORY_RESIDUE = (
    API_COMPAT_ROOT / ".venv",
    API_COMPAT_ROOT / ".pytest_cache",
)


def run_make(
    target: str,
    env: dict[str, str],
    *,
    expected: int = 0,
    variables: dict[str, str] | None = None,
) -> str:
    command = ["make", target]
    command.extend(f"{name}={value}" for name, value in (variables or {}).items())
    completed = subprocess.run(
        command,
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
    residue = [str(path.relative_to(ROOT)) for path in repository_residue_paths()]
    if residue:
        raise AssertionError(f"API compatibility run left repository-local residue: {', '.join(residue)}")


def repository_residue_paths() -> tuple[Path, ...]:
    return tuple(
        sorted(
            {path for path in (*REPOSITORY_RESIDUE, *API_COMPAT_ROOT.rglob("__pycache__")) if path.exists()},
            key=lambda path: str(path),
        )
    )


def remove_repository_residue() -> None:
    """Clear only generated API-compatibility artifacts before the bootstrap proof."""
    for path in repository_residue_paths():
        if path.is_symlink():
            path.unlink()
        elif path.is_dir():
            shutil.rmtree(path)


def write_executable(path: Path, body: str) -> None:
    path.write_text(f"#!/bin/sh\n{body}\n", encoding="utf-8")
    path.chmod(0o755)


def assert_provisioning_failures_stop_immediately() -> None:
    """Prove failed bootstrap commands cannot be hidden by later successes."""
    with tempfile.TemporaryDirectory(prefix="api-compat-fail-closed-") as temporary:
        temporary_root = Path(temporary)
        command_bin = temporary_root / "bin"
        command_bin.mkdir()
        command_log = temporary_root / "commands.log"

        # A failing locked Python provision must prevent the Go download from
        # running even when that subsequent command would succeed.
        write_executable(command_bin / "uv", "exit 71")
        write_executable(
            command_bin / "go",
            f"if [ \"$1\" = mod ] && [ \"$2\" = download ]; then printf 'go-download\\n' >> '{command_log}'; fi\nexit 0",
        )
        bootstrap_env = os.environ.copy() | {"PATH": f"{command_bin}:{os.environ['PATH']}"}
        bootstrap_output = run_make("api-compat-bootstrap", bootstrap_env, expected=2)
        if "Error 71" not in bootstrap_output:
            raise AssertionError("bootstrap did not expose the locked Python provisioning failure")
        if command_log.exists():
            raise AssertionError("bootstrap ran Go dependency provisioning after locked Python provisioning failed")

        # The normal target delegates bootstrap and offline work to recursive
        # make. A failing bootstrap must stop before the offline phase, even if
        # that later phase would report success.
        fake_make = temporary_root / "fake-make"
        write_executable(
            fake_make,
            "case \"$1\" in\n"
            f"  api-compat-bootstrap) printf 'bootstrap\\n' >> '{command_log}'; exit 71 ;;\n"
            f"  api-compat-mock-offline) printf 'offline\\n' >> '{command_log}'; exit 0 ;;\n"
            "  *) exit 2 ;;\n"
            "esac",
        )
        mock_output = run_make(
            "api-compat-mock",
            os.environ.copy(),
            expected=2,
            variables={"MAKE": str(fake_make)},
        )
        if "Error 71" not in mock_output:
            raise AssertionError("normal orchestration did not expose the bootstrap failure")
        if command_log.read_text(encoding="utf-8").splitlines() != ["bootstrap"]:
            raise AssertionError("normal orchestration ran the offline phase after bootstrap failed")


def assert_mirror_values_are_go_scoped_shell_data() -> None:
    """CLI and environment mirror settings reach only Go as literal data."""
    with tempfile.TemporaryDirectory(prefix="api-compat-mirror-data-") as temporary:
        root = Path(temporary)
        tool_dir = root / "bin"
        tool_dir.mkdir()
        marker = root / "must-not-exist"
        captured = root / "captured-mirrors"
        uv_captured = root / "captured-uv-mirrors"
        offline_captured = root / "captured-offline-mirrors"

        write_executable(
            tool_dir / "uv",
            'printf "%s\\n%s\\n%s\\n%s\\n" "${API_COMPAT_BOOTSTRAP_GO_PROXY-}" "${API_COMPAT_BOOTSTRAP_GO_SUMDB-}" "${MAKEFLAGS-}" "${MAKEOVERRIDES-}" >"$API_COMPAT_CAPTURED_UV_MIRRORS"\nexit 0',
        )
        write_executable(
            tool_dir / "go",
            'printf "%s\\n%s\\n" "$GOPROXY" "$GOSUMDB" >"$API_COMPAT_CAPTURED_MIRRORS"\nexit 1',
        )

        command_line_proxy = f"https://mirror.invalid/$(shell touch {marker});'quoted'"
        command_line_sumdb = f"sumdb.invalid; touch {marker} #"
        environment_proxy = "https://mirror.invalid/environment-proxy"
        environment_sumdb = "sumdb.invalid/environment-sumdb"

        cases = (
            (
                "direct command-line bootstrap",
                "api-compat-bootstrap",
                {},
                {
                    "API_COMPAT_BOOTSTRAP_GO_PROXY": command_line_proxy,
                    "API_COMPAT_BOOTSTRAP_GO_SUMDB": command_line_sumdb,
                },
                command_line_proxy,
                command_line_sumdb,
            ),
            (
                "recursive environment bootstrap",
                "api-compat-mock",
                {
                    "API_COMPAT_BOOTSTRAP_GO_PROXY": environment_proxy,
                    "API_COMPAT_BOOTSTRAP_GO_SUMDB": environment_sumdb,
                },
                {},
                environment_proxy,
                environment_sumdb,
            ),
        )
        for description, target, mirrors, variables, expected_proxy, expected_sumdb in cases:
            captured.unlink(missing_ok=True)
            uv_captured.unlink(missing_ok=True)
            env = isolated_environment(root) | mirrors
            env.update(
                {
                    "PATH": f"{tool_dir}:{env['PATH']}",
                    "API_COMPAT_CAPTURED_MIRRORS": str(captured),
                    "API_COMPAT_CAPTURED_UV_MIRRORS": str(uv_captured),
                }
            )
            run_make(target, env, expected=2, variables=variables)
            python_transport = uv_captured.read_text(encoding="utf-8").splitlines()
            if python_transport != ["", "", "", ""]:
                raise AssertionError(
                    f"{description} let Python provisioning inherit bootstrap mirror configuration "
                    f"through standalone variables or Make transport"
                )
            if captured.read_text(encoding="utf-8").splitlines() != [expected_proxy, expected_sumdb]:
                raise AssertionError(f"{description} did not pass bootstrap mirror configuration to Go verbatim")

        # The offline recursive Make boundary must discard command-line
        # overrides as well as standalone environment variables. A fake
        # successful bootstrap reaches the offline `uv run` phase and records
        # the complete transport surface that previously reintroduced them.
        write_executable(
            tool_dir / "uv",
            'case "$1" in\n'
            '  sync) exit 0 ;;\n'
            '  run) printf "%s\\n%s\\n%s\\n%s\\n" "${API_COMPAT_BOOTSTRAP_GO_PROXY-}" "${API_COMPAT_BOOTSTRAP_GO_SUMDB-}" "${MAKEFLAGS-}" "${MAKEOVERRIDES-}" >"$API_COMPAT_CAPTURED_OFFLINE_MIRRORS"; exit 0 ;;\n'
            '  *) exit 70 ;;\n'
            'esac',
        )
        write_executable(tool_dir / "go", "exit 0")
        offline_captured.unlink(missing_ok=True)
        env = isolated_environment(root)
        env.update(
            {
                "PATH": f"{tool_dir}:{env['PATH']}",
                "API_COMPAT_CAPTURED_OFFLINE_MIRRORS": str(offline_captured),
            }
        )
        run_make(
            "api-compat-mock",
            env,
            variables={
                "API_COMPAT_BOOTSTRAP_GO_PROXY": command_line_proxy,
                "API_COMPAT_BOOTSTRAP_GO_SUMDB": command_line_sumdb,
            },
        )
        offline_transport = offline_captured.read_text(encoding="utf-8").splitlines()
        if offline_transport[:2] != ["", ""]:
            raise AssertionError("offline recursion inherited standalone bootstrap mirrors")
        # GNU Make is entitled to create its own non-sensitive MAKEFLAGS (for
        # example, `--no-print-directory`). It must not recreate either
        # bootstrap assignment in that transport.
        if any(
            value in "\n".join(offline_transport[2:])
            for value in (
                "API_COMPAT_BOOTSTRAP_GO_PROXY",
                "API_COMPAT_BOOTSTRAP_GO_SUMDB",
                command_line_proxy,
                command_line_sumdb,
            )
        ):
            raise AssertionError("offline recursion inherited bootstrap mirrors through Make override transport")
        if marker.exists():
            raise AssertionError("command-line bootstrap mirror configuration executed Make or shell syntax")


def main() -> int:
    remove_repository_residue()
    assert_no_repository_residue()
    assert_provisioning_failures_stop_immediately()
    assert_mirror_values_are_go_scoped_shell_data()
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
