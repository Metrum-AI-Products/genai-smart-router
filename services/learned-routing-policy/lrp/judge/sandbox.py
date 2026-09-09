# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Fail-closed verifier execution using a minimal operator-owned root filesystem."""

from __future__ import annotations

import os
import selectors
import shutil
import signal
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from ..collect import DataError, canonical, protected_path, strict_json
from .contracts import normalize_verifier


def worker_source() -> str:
    """Allowlisted plugin runtime plus worker, executed only inside bubblewrap."""
    runtime = Path(__file__).with_name("plugins_runtime.py").read_text()
    worker = Path(__file__).with_name("worker.py").read_text()
    # Strip the runtime module's unused future import noise is fine; keep source exact
    # for cache identity. Worker calls run_plugin from the concatenated namespace.
    return runtime + "\n" + worker


@dataclass(frozen=True)
class Sandbox:
    """Rootfs contains /usr/bin/python3, pytest/jsonschema and empty /work.

    It must contain no operator data, credentials, host mounts or sockets. No host
    root, home, project or environment is mounted. Linux user/network/PID/mount
    namespace support and bubblewrap are prerequisites, never silently bypassed.
    """

    rootfs: Path
    timeout_s: float = 30

    def command(self) -> list[str]:
        if not 0 < self.timeout_s <= 30:
            raise DataError("invalid_verifier_timeout")
        root = protected_path(self.rootfs)
        if root == Path("/") or not root.is_dir() or root.stat().st_mode & 0o022:
            raise DataError("invalid_sandbox_root")
        executable = shutil.which("bwrap")
        if not executable:
            raise DataError("sandbox_unavailable")
        worker = worker_source()
        return [
            executable,
            "--unshare-all",
            "--unshare-user",
            "--disable-userns",
            "--die-with-parent",
            "--new-session",
            "--uid",
            "65534",
            "--gid",
            "65534",
            "--cap-drop",
            "ALL",
            "--clearenv",
            "--ro-bind",
            str(root),
            "/",
            "--proc",
            "/proc",
            "--dev",
            "/dev",
            "--size",
            "16777216",
            "--perms",
            "0700",
            "--tmpfs",
            "/work",
            "--chdir",
            "/work",
            "--setenv",
            "PATH",
            "/usr/bin",
            "/usr/bin/python3",
            "-I",
            "-c",
            worker,
        ]

    def verify(
        self, verifier: dict[str, Any], content: str
    ) -> tuple[float | None, dict[str, Any]]:
        try:
            normalized = normalize_verifier(verifier)
            command = self.command()
        except DataError as exc:
            name = str(exc)
            if name in {
                "unsupported_verifier",
                "unsupported_verifier_version",
                "unsupported_verifier_contract",
                "unsupported_verifier_spec_field",
                "missing_verifier_spec",
                "unsupported_plugin_id",
                "invalid_plugin_params",
                "invalid_sql_result_expected",
                "invalid_sql_result_columns",
                "invalid_sql_result_order_flag",
            }:
                return None, {"unsupported": True, "error_class": name}
            return None, {"unsupported": True, "error_class": "sandbox_unavailable"}
        except OSError:
            return None, {"unsupported": True, "error_class": "sandbox_unavailable"}
        payload = (
            canonical(
                {
                    "kind": normalized["kind"],
                    "version": normalized["version"],
                    "spec": normalized.get("spec", {}),
                    "content": content,
                }
            )
            + "\n"
        ).encode()
        if len(payload) > 2 * 1024 * 1024:
            return None, {"unsupported": True, "error_class": "verifier_input_limit"}
        try:
            process = subprocess.Popen(
                command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
                env={},
                start_new_session=True,
            )
        except OSError:
            return None, {"unsupported": True, "error_class": "sandbox_unavailable"}
        assert process.stdin is not None and process.stdout is not None
        # Nonblocking input as well as output: a dead worker must not block the writer.
        os.set_blocking(process.stdin.fileno(), False)
        os.set_blocking(process.stdout.fileno(), False)
        output = bytearray()
        offset = 0
        deadline = time.monotonic() + self.timeout_s
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdin, selectors.EVENT_WRITE)
                selector.register(process.stdout, selectors.EVENT_READ)
                while selector.get_map():
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        return 0.0, {"timeout": True, "error_class": "verifier_timeout"}
                    for key, _ in selector.select(min(remaining, 0.1)):
                        if key.fileobj is process.stdin:
                            try:
                                offset += os.write(
                                    process.stdin.fileno(),
                                    payload[offset : offset + 4096],
                                )
                            except BrokenPipeError:
                                offset = len(payload)
                            if offset >= len(payload):
                                selector.unregister(process.stdin)
                                process.stdin.close()
                        else:
                            chunk = os.read(process.stdout.fileno(), 4096)
                            if not chunk:
                                selector.unregister(process.stdout)
                            output.extend(chunk)
                            if len(output) > 4096:
                                return 0.0, {"error_class": "verifier_output_limit"}
            try:
                exitcode = process.wait(timeout=max(0.01, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                return 0.0, {"timeout": True, "error_class": "verifier_timeout"}
            if exitcode != 0:
                return None, {"unsupported": True, "error_class": "sandbox_unavailable"}
            verdict = strict_json(bytes(output))
            if set(verdict) != {"passed"} or type(verdict["passed"]) is not bool:
                return None, {
                    "unsupported": True,
                    "error_class": "invalid_verifier_result",
                }
            return float(verdict["passed"]), {"passed": verdict["passed"]}
        except (OSError, ValueError):
            return None, {"unsupported": True, "error_class": "sandbox_unavailable"}
        finally:
            # Kill the whole launch group; bwrap's PID namespace also reaps descendants.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
            if not process.stdin.closed:
                process.stdin.close()
            process.stdout.close()
