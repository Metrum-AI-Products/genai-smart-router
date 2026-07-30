#!/usr/bin/env python3
"""Planner-owned local tmux/WSH control plane.

This is deliberately a thin, profile-driven launcher.  It does not expose a
general terminal API and it does not assume that a WSH server authorizes a
lease: the planner owns allocation, idempotency, and cleanup in its protected
state directory.  Command templates are administrative configuration, not
worker-supplied shell input.
"""

from __future__ import annotations

import argparse
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import shlex
import subprocess
import sys
import tempfile
import stat
import signal
import time
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Iterable


NAME_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{0,63}$")
STATE_VERSION = 1
DEFAULT_STATUS_TIMEOUT_SECONDS = 2
DEFAULT_STATUS_OUTPUT_BYTES = 16 * 1024
DEFAULT_STATUS_FIELD_BYTES = 256
DEFAULT_COMMAND_OUTPUT_BYTES = 1024 * 1024
DEFAULT_COMMAND_TIMEOUT_SECONDS = 10
APPROVED_WORKER_ROLES = {"software_engineer", "quality_engineer", "system_architect", "product_manager", "interaction_designer", "eks_operations"}
SAFE_STATUS_FIELDS = {
    "session_id",
    "state",
    "assignment_id",
    "updated_at",
    "heartbeat_at",
    "exit_class",
}
TEMPLATE_FIELDS = {
    "profile_id",
    "tmux_session",
    "wsh_server_name",
    "planner_wsh_session_id",
    "window_name",
    "wsh_session_id",
    "assignment_id",
    "issue",
    "role",
    "worktree",
    "lease_id",
    "repository_root",
    "assignment_prompt",
    "planner_assignment_prompt",
}


class ControlPlaneError(RuntimeError):
    """A predictable, safe-to-display launcher failure."""


class AtomicWriteError(ControlPlaneError):
    """A durable write failed before or after its atomic visibility point."""

    def __init__(self, target: Path, *, after_replace: bool, cause: OSError):
        stage = "after atomic replace" if after_replace else "before atomic replace"
        super().__init__(f"durable write for {target.name} failed {stage}")
        self.target = target
        self.after_replace = after_replace
        self.__cause__ = cause


def _fsync_parent_directory(directory: Path) -> bool:
    """Persist a prior rename when the host filesystem supports directory fsync."""
    try:
        descriptor = os.open(directory, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    except OSError as exc:
        if exc.errno in {getattr(os, "EINVAL", 22), getattr(os, "ENOTSUP", 95), getattr(os, "EOPNOTSUPP", 95)}:
            return False
        raise
    try:
        try:
            os.fsync(descriptor)
        except OSError as exc:
            if exc.errno in {getattr(os, "EINVAL", 22), getattr(os, "ENOTSUP", 95), getattr(os, "EOPNOTSUPP", 95)}:
                return False
            raise
    finally:
        os.close(descriptor)
    return True


def _durable_json_replace(target: Path, value: dict[str, Any], *, prefix: str) -> bool:
    """Write JSON atomically and fsync its containing directory when supported.

    A failure before ``os.replace`` leaves the old file authoritative. A failure
    after it is intentionally reported as uncertain: callers must retain their
    matching lease/state rather than guessing which version survived a crash.
    """
    descriptor, temporary = tempfile.mkstemp(prefix=prefix, dir=target.parent)
    replaced = False
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(value, handle, sort_keys=True, separators=(",", ":"))
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, 0o600)
        os.replace(temporary, target)
        replaced = True
        return _fsync_parent_directory(target.parent)
    except OSError as exc:
        raise AtomicWriteError(target, after_replace=replaced, cause=exc) from exc
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


@dataclass(frozen=True)
class RunResult:
    returncode: int
    stdout: str = ""
    stderr: str = ""


class Runner:
    def run(
        self,
        argv: list[str],
        *,
        check: bool = True,
        timeout_seconds: int | None = None,
        max_output_bytes: int | None = None,
    ) -> RunResult:
        cap = max_output_bytes if max_output_bytes is not None else DEFAULT_COMMAND_OUTPUT_BYTES
        effective_timeout = timeout_seconds if timeout_seconds is not None else DEFAULT_COMMAND_TIMEOUT_SECONDS
        try:
            process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        except OSError as exc:
            raise ControlPlaneError("unable to start command") from exc
        deadline = time.monotonic() + effective_timeout

        def terminate_group() -> None:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=1)
            except subprocess.TimeoutExpired:
                pass

        selector = selectors.DefaultSelector()
        streams = {"stdout": process.stdout, "stderr": process.stderr}
        captured = {"stdout": bytearray(), "stderr": bytearray()}
        oversized = {"stdout": False, "stderr": False}
        try:
            for name, stream in streams.items():
                if stream is None:
                    continue
                os.set_blocking(stream.fileno(), False)
                selector.register(stream, selectors.EVENT_READ, name)
            while selector.get_map() or process.poll() is None:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    # Closing our local descriptors is what guarantees return
                    # even when an escaped descendant retains inherited pipes.
                    terminate_group()
                    return RunResult(124)
                select_timeout = remaining if selector.get_map() else min(remaining, 0.05)
                for key, _ in selector.select(select_timeout):
                    name = str(key.data)
                    try:
                        chunk = os.read(key.fileobj.fileno(), 4096)
                    except BlockingIOError:
                        continue
                    if not chunk:
                        selector.unregister(key.fileobj)
                        key.fileobj.close()
                        continue
                    available = cap - sum(len(value) for value in captured.values())
                    if available > 0:
                        captured[name].extend(chunk[:available])
                    if len(chunk) > available:
                        oversized[name] = True
            returncode = process.wait()
            if oversized["stdout"] or oversized["stderr"]:
                return RunResult(125)
            result = RunResult(
                returncode,
                bytes(captured["stdout"]).decode("utf-8", errors="replace"),
                bytes(captured["stderr"]).decode("utf-8", errors="replace"),
            )
        except OSError as exc:
            terminate_group()
            raise ControlPlaneError("unable to run command") from exc
        finally:
            selector.close()
            for stream in streams.values():
                if stream and not stream.closed:
                    stream.close()
        if result.returncode:
            if check:
                raise ControlPlaneError("command failed: " + shlex.join(argv))
        return result


def _utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def _fingerprint(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()[:20]


def _safe_name(value: str, label: str) -> str:
    if not NAME_RE.fullmatch(value):
        raise ControlPlaneError(f"{label} must be a lowercase safe name")
    return value


def _command(value: Any, label: str) -> list[str]:
    if not isinstance(value, list) or not value or not all(isinstance(part, str) and part for part in value):
        raise ControlPlaneError(f"profile commands.{label} must be a non-empty JSON string array")
    return list(value)


def _reject_unsafe_command(command: list[str], label: str) -> None:
    shell_wrappers = {"sh", "bash", "dash", "zsh", "fish", "cmd", "powershell", "pwsh", "env"}
    if Path(command[0]).name in shell_wrappers or "tmux" in {Path(part).name for part in command}:
        raise ControlPlaneError(f"profile commands.{label} must not use a shell wrapper or nested tmux")
    forbidden = ("$(`", "$(", "`", ";", "&&", "||", "\n", "\r", "\x00", "http://", "https://", "tcp://", "0.0.0.0", "--listen", "--host", "--port")
    if any(token in part for part in command for token in forbidden):
        raise ControlPlaneError(f"profile commands.{label} contains an unsafe or network-exposed template")


def _require_wsh_shape(command: list[str], label: str, verb: str) -> None:
    if Path(command[0]).name != "wsh" or len(command) < 3 or command[1] != "session" or command[2] != verb:
        raise ControlPlaneError(f"profile commands.{label} must invoke the approved WSH session {verb} shape")


def _require_option(command: list[str], option: str, expected: str, label: str) -> None:
    try:
        if command[command.index(option) + 1] != expected:
            raise ValueError
    except (ValueError, IndexError):
        raise ControlPlaneError(f"profile commands.{label} must bind {option} to the planner-issued template")


def _render_command(template: Iterable[str], values: dict[str, str]) -> list[str]:
    rendered: list[str] = []
    for part in template:
        fields = set(re.findall(r"\{([a-z_]+)\}", part))
        unknown = fields - TEMPLATE_FIELDS
        if unknown:
            raise ControlPlaneError("profile command contains an unsupported template field")
        try:
            result = part.format(**values)
        except KeyError as exc:
            raise ControlPlaneError("profile command contains an unsupported template field") from exc
        if "\x00" in result or "\n" in result or "\r" in result:
            raise ControlPlaneError("rendered command contains a control character")
        rendered.append(result)
    return rendered


@dataclass(frozen=True)
class Profile:
    path: Path
    profile_id: str
    planner_owner: str
    tmux_session: str
    wsh_server_name: str
    wsh_server_identity: str
    bootstrap_identity_wait_seconds: int
    planner_wsh_session_id: str
    state_directory: Path
    worker_session_prefix: str
    base_ref: str | None
    lease_ttl_seconds: int
    status_timeout_seconds: int
    status_output_bytes: int
    status_field_bytes: int
    forbidden_environment_variables: tuple[str, ...]
    commands: dict[str, list[str]]

    @classmethod
    def load(cls, path: Path) -> "Profile":
        try:
            profile_stat = path.lstat()
        except OSError as exc:
            raise ControlPlaneError("unable to inspect the runtime control profile") from exc
        if not stat.S_ISREG(profile_stat.st_mode) or stat.S_ISLNK(profile_stat.st_mode):
            raise ControlPlaneError("runtime control profile must be a regular non-symlink file")
        if profile_stat.st_uid != os.geteuid() or profile_stat.st_mode & 0o077:
            raise ControlPlaneError("runtime control profile must be owned by the planner and mode 0600")
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ControlPlaneError("unable to read the runtime control profile") from exc
        if not isinstance(raw, dict):
            raise ControlPlaneError("runtime control profile must be a JSON object")
        profile_id = _safe_name(str(raw.get("profile_id", "")), "profile_id")
        planner_owner = _safe_name(str(raw.get("planner_owner", "")), "planner_owner")
        tmux_session = _safe_name(str(raw.get("tmux_session", "")), "tmux_session")
        wsh_server_name = _safe_name(str(raw.get("wsh_server_name", "")), "wsh_server_name")
        wsh_server_identity = _safe_name(str(raw.get("wsh_server_identity", "")), "wsh_server_identity")
        bootstrap_identity_wait = raw.get("bootstrap_identity_wait_seconds")
        if (
            not isinstance(bootstrap_identity_wait, int)
            or isinstance(bootstrap_identity_wait, bool)
            or not 1 <= bootstrap_identity_wait <= 10
        ):
            raise ControlPlaneError("profile bootstrap_identity_wait_seconds must be an integer between 1 and 10")
        planner_wsh_session_id = _safe_name(str(raw.get("planner_wsh_session_id", "")), "planner_wsh_session_id")
        prefix = _safe_name(str(raw.get("worker_session_prefix", "")), "worker_session_prefix")
        state_value = raw.get("state_directory")
        if not isinstance(state_value, str) or not state_value:
            raise ControlPlaneError("profile state_directory is required")
        state_directory = Path(state_value).expanduser()
        if not state_directory.is_absolute():
            raise ControlPlaneError("profile state_directory must be absolute")
        env_names = raw.get("forbidden_environment_variables")
        if not isinstance(env_names, list) or not env_names or not all(
            isinstance(name, str) and re.fullmatch(r"[A-Z][A-Z0-9_]*", name) for name in env_names
        ):
            raise ControlPlaneError("profile forbidden_environment_variables must contain environment variable names")
        commands_raw = raw.get("commands")
        required = {"wsh_server", "wsh_identity", "planner", "worker", "wsh_stop", "planner_stop", "planner_tag", "wsh_status", "codex_preflight"}
        if not isinstance(commands_raw, dict) or set(commands_raw) != required:
            raise ControlPlaneError("profile commands must define exactly wsh_server, wsh_identity, planner, worker, wsh_stop, planner_stop, planner_tag, wsh_status, and codex_preflight")
        commands = {name: _command(commands_raw[name], name) for name in required}
        for name, command in commands.items():
            _reject_unsafe_command(command, name)
        server = commands["wsh_server"]
        if server != ["wsh", "-L", "{wsh_server_name}", "server"]:
            raise ControlPlaneError("profile commands.wsh_server must use the portable `wsh server` shape")
        if commands["wsh_identity"] != ["wsh", "-L", "{wsh_server_name}", "identity", "--json"]:
            raise ControlPlaneError("profile commands.wsh_identity must use the exact authoritative JSON identity handshake")
        if commands["wsh_stop"] != ["wsh", "-L", "{wsh_server_name}", "kill", "{wsh_session_id}"]:
            raise ControlPlaneError("profile commands.wsh_stop must use the portable `wsh kill <NAME>` shape")
        if commands["planner_stop"] != ["wsh", "-L", "{wsh_server_name}", "kill", "{planner_wsh_session_id}"]:
            raise ControlPlaneError("profile commands.planner_stop must use the exact planner WSH kill shape")
        if commands["planner_tag"] != ["wsh", "-L", "{wsh_server_name}", "tag", "{planner_wsh_session_id}"]:
            raise ControlPlaneError("profile commands.planner_tag must use the exact read-only planner WSH tag shape")
        if commands["wsh_status"] != ["wsh", "-L", "{wsh_server_name}", "list"]:
            raise ControlPlaneError("profile commands.wsh_status must use the bounded portable `wsh list` shape")
        worker = commands["worker"]
        if worker != [
            "scripts/planner_wsh_worker.sh",
            "{wsh_server_name}",
            "{wsh_session_id}",
            "assignment-{assignment_id}",
            "lease-{lease_id}",
            "{worktree}",
            "{assignment_prompt}",
        ]:
            raise ControlPlaneError("profile commands.worker must use the approved WSH-to-Codex worker adapter")
        if commands["planner"] != [
            "scripts/planner_wsh_planner.sh", "{wsh_server_name}", "{planner_wsh_session_id}",
            "planner-{profile_id}", "{repository_root}", "{planner_assignment_prompt}",
        ]:
            raise ControlPlaneError("profile commands.planner must use the approved named WSH-to-Codex planner adapter")
        if commands["codex_preflight"] != ["codex", "--help"]:
            raise ControlPlaneError("profile commands.codex_preflight must use the bounded supported `codex --help` shape")
        base_ref = raw.get("base_ref")
        if base_ref is not None and (not isinstance(base_ref, str) or not base_ref or "\n" in base_ref):
            raise ControlPlaneError("profile base_ref must be a non-empty ref when set")
        ttl = raw.get("lease_ttl_seconds")
        if not isinstance(ttl, int) or isinstance(ttl, bool) or not 60 <= ttl <= 86400:
            raise ControlPlaneError("profile lease_ttl_seconds must be between 60 and 86400")
        status_timeout = raw.get("status_timeout_seconds", DEFAULT_STATUS_TIMEOUT_SECONDS)
        status_output = raw.get("status_output_bytes", DEFAULT_STATUS_OUTPUT_BYTES)
        status_field = raw.get("status_field_bytes", DEFAULT_STATUS_FIELD_BYTES)
        for value, minimum, maximum, label in (
            (status_timeout, 1, 10, "status_timeout_seconds"),
            (status_output, 256, 65536, "status_output_bytes"),
            (status_field, 16, 1024, "status_field_bytes"),
        ):
            if not isinstance(value, int) or isinstance(value, bool) or not minimum <= value <= maximum:
                raise ControlPlaneError(f"profile {label} is outside the approved bound")
        return cls(
            path, profile_id, planner_owner, tmux_session, wsh_server_name, wsh_server_identity, bootstrap_identity_wait,
            planner_wsh_session_id, state_directory, prefix, base_ref, ttl,
            status_timeout, status_output, status_field, tuple(env_names), commands,
        )


class ControlPlane:
    def __init__(self, profile: Profile, runner: Runner | None = None, cwd: Path | None = None):
        self.profile = profile
        self.runner = runner or Runner()
        self.cwd = (cwd or Path.cwd()).resolve()

    def _git(self, *args: str, check: bool = True) -> RunResult:
        return self.runner.run(["git", "-C", str(self.cwd), *args], check=check)

    @property
    def repository_root(self) -> Path:
        root = self._git("rev-parse", "--show-toplevel").stdout.strip()
        if not root:
            raise ControlPlaneError("unable to derive repository root from Git")
        return Path(root).resolve()

    @property
    def state_path(self) -> Path:
        return self.profile.state_directory / "planner-wsh-control-state.json"

    @property
    def lock_path(self) -> Path:
        return self.profile.state_directory / "planner-wsh-control-state.lock"

    @property
    def audit_path(self) -> Path:
        return self.profile.state_directory / "planner-wsh-control-audit.jsonl"

    @property
    def registry_directory(self) -> Path:
        common = self._git("rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip()
        if not common:
            raise ControlPlaneError("unable to derive repository-common lease registry")
        return Path(common) / "planner-wsh-leases"

    @contextlib.contextmanager
    def _locked_registry(self) -> Any:
        directory = self.registry_directory
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        lock_path = directory / "registry.lock"
        state_path = directory / "registry.json"
        with lock_path.open("a+", encoding="utf-8") as lock:
            os.chmod(lock_path, 0o600)
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            try:
                if state_path.exists():
                    raw = json.loads(state_path.read_text(encoding="utf-8"))
                    if not isinstance(raw, dict) or not isinstance(raw.get("leases"), dict) or not isinstance(raw.get("generation"), int):
                        raise ControlPlaneError("repository-common lease registry is corrupt; refuse recovery")
                    # The old boolean cannot safely identify the owner or exact
                    # shutdown to resume. Preserve it as an explicit recovery
                    # requirement rather than silently clearing it.
                    if "drain" not in raw:
                        raw["drain"] = ({"state": "legacy-recovery-required", "owner_profile": None, "started_at": None} if raw.pop("stopping", False) else None)
                    if raw["drain"] is not None and (
                        not isinstance(raw["drain"], dict)
                        or raw["drain"].get("state") not in {"draining", "recovering", "legacy-recovery-required"}
                    ):
                        raise ControlPlaneError("repository-common drain metadata is corrupt; refuse recovery")
                else:
                    raw = {"generation": 0, "drain": None, "leases": {}}
                yield raw
                _durable_json_replace(state_path, raw, prefix=".registry-")
            finally:
                fcntl.flock(lock.fileno(), fcntl.LOCK_UN)

    def _reserve_shared_lease(self, worktree_id: str, assignment_id: str) -> tuple[str, int]:
        with self._locked_registry() as registry:
            if registry["drain"] is not None:
                raise ControlPlaneError("worker launch rejected: repository drain requires explicit recovery")
            if worktree_id in registry["leases"]:
                current = registry["leases"][worktree_id]
                if current.get("owner_profile") == self.profile.profile_id and current.get("assignment_id") == assignment_id:
                    return str(current["lease_id"]), int(current["generation"])
                raise ControlPlaneError("worker launch rejected: worktree already has a repository-common lease")
            registry["generation"] += 1
            generation = registry["generation"]
            lease_id = _fingerprint(f"{self.profile.profile_id}:{assignment_id}:{worktree_id}:{generation}")
            registry["leases"][worktree_id] = {"lease_id": lease_id, "generation": generation, "owner_profile": self.profile.profile_id, "assignment_id": assignment_id, "state": "allocated", "at": _utc_now()}
            return lease_id, generation

    def _release_shared_lease(self, worktree_id: str, lease_id: str, generation: int) -> None:
        with self._locked_registry() as registry:
            current = registry["leases"].get(worktree_id)
            if not isinstance(current, dict) or current.get("owner_profile") != self.profile.profile_id or current.get("lease_id") != lease_id or current.get("generation") != generation:
                raise ControlPlaneError("repository-common lease ownership is ambiguous; refuse release")
            del registry["leases"][worktree_id]

    def _begin_repository_shutdown(self) -> None:
        with self._locked_registry() as registry:
            if registry["leases"]:
                raise ControlPlaneError("shutdown refused while repository-common worker leases exist")
            if registry["drain"] is not None:
                raise ControlPlaneError("shutdown already requires explicit drain recovery")
            registry["drain"] = {
                "state": "draining",
                "owner_profile": self.profile.profile_id,
                "owner": self.profile.planner_owner,
                "started_at": _utc_now(),
                "generation": registry["generation"],
            }

    def _resume_repository_shutdown(self) -> None:
        with self._locked_registry() as registry:
            drain = registry["drain"]
            if not isinstance(drain, dict) or drain.get("state") == "legacy-recovery-required":
                raise ControlPlaneError("repository drain recovery is ambiguous; do not clear it manually")
            if drain.get("owner_profile") != self.profile.profile_id or drain.get("owner") != self.profile.planner_owner:
                raise ControlPlaneError("repository drain belongs to a different profile; refuse recovery")
            if registry["leases"]:
                raise ControlPlaneError("repository drain recovery refused while worker leases exist")
            drain["state"] = "recovering"
            drain["recovery_started_at"] = _utc_now()

    def _finish_repository_shutdown(self) -> None:
        with self._locked_registry() as registry:
            if registry["leases"]:
                raise ControlPlaneError("repository-common lease appeared during shutdown")
            drain = registry["drain"]
            if not isinstance(drain, dict) or drain.get("owner_profile") != self.profile.profile_id:
                raise ControlPlaneError("repository drain ownership is ambiguous; refuse completion")
            registry["drain"] = None

    def _record_planner_absence_proof(self) -> None:
        """Durably retain the exact-list proof before the WSH server is stopped."""
        with self._locked_registry() as registry:
            drain = registry["drain"]
            if not isinstance(drain, dict) or drain.get("owner_profile") != self.profile.profile_id:
                raise ControlPlaneError("repository drain ownership is ambiguous; refuse absence-proof recording")
            drain["planner_absence_proof"] = {
                "at": _utc_now(), "owner_profile": self.profile.profile_id,
                "owner": self.profile.planner_owner, "generation": registry["generation"],
                "planner_wsh_session_id": self.profile.planner_wsh_session_id,
            }

    def _has_planner_absence_proof(self) -> bool:
        with self._locked_registry() as registry:
            drain = registry["drain"]
            proof = drain.get("planner_absence_proof") if isinstance(drain, dict) else None
            return isinstance(proof, dict) and isinstance(drain, dict) and drain.get("generation") == registry["generation"] and proof == {
                "at": proof.get("at"), "owner_profile": self.profile.profile_id,
                "owner": self.profile.planner_owner, "generation": registry["generation"],
                "planner_wsh_session_id": self.profile.planner_wsh_session_id,
            } and isinstance(proof.get("at"), str)

    @contextlib.contextmanager
    def _locked_state(self) -> Any:
        self.profile.state_directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        directory_stat = self.profile.state_directory.lstat()
        if (
            not stat.S_ISDIR(directory_stat.st_mode)
            or stat.S_ISLNK(directory_stat.st_mode)
            or directory_stat.st_uid != os.geteuid()
            or directory_stat.st_mode & 0o077
        ):
            raise ControlPlaneError("control-plane state directory must be planner-owned, mode 0700, and not a symlink")
        with self.lock_path.open("a+", encoding="utf-8") as lock:
            os.chmod(self.lock_path, 0o600)
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            state = self._read_state()
            try:
                yield state
            finally:
                fcntl.flock(lock.fileno(), fcntl.LOCK_UN)

    def _read_state(self) -> dict[str, Any]:
        if not self.state_path.exists():
            return self._new_state()
        try:
            state = json.loads(self.state_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ControlPlaneError("control-plane state is unreadable; recover it with the documented procedure") from exc
        if not isinstance(state, dict) or state.get("version") != STATE_VERSION:
            raise ControlPlaneError("control-plane state has an unsupported version")
        if state.get("profile_id") != self.profile.profile_id:
            raise ControlPlaneError("control-plane state belongs to a different profile")
        if state.get("planner_owner") != self.profile.planner_owner:
            raise ControlPlaneError("control-plane state belongs to a different planner owner")
        if state.get("planner_wsh_session_id") != self.profile.planner_wsh_session_id:
            raise ControlPlaneError("control-plane state belongs to a different planner WSH identity")
        if state.get("repository_id") != _fingerprint(str(self.repository_root)):
            raise ControlPlaneError("control-plane state belongs to a different repository")
        if not isinstance(state.get("workers"), dict):
            raise ControlPlaneError("control-plane state is malformed")
        return state

    def _new_state(self) -> dict[str, Any]:
        return {
            "version": STATE_VERSION,
            "profile_id": self.profile.profile_id,
            "planner_owner": self.profile.planner_owner,
            "repository_id": _fingerprint(str(self.repository_root)),
            "tmux_session": self.profile.tmux_session,
            "workers": {},
            "planner_wsh_session_id": self.profile.planner_wsh_session_id,
            "planner_identity_tag": f"planner-{self.profile.profile_id}",
            "updated_at": _utc_now(),
        }

    def _write_state(self, state: dict[str, Any]) -> None:
        state["updated_at"] = _utc_now()
        _durable_json_replace(self.state_path, state, prefix=".planner-wsh-")

    def _audit(
        self, action: str, outcome: str, worker: dict[str, Any] | None = None, *, disposition: str | None = None
    ) -> None:
        """Append safe lifecycle metadata. This file is never reset or pruned by shutdown."""
        event: dict[str, str] = {"event_id": str(uuid.uuid4()), "at": _utc_now(), "action": action, "outcome": outcome}
        if worker:
            for source, target in (
                ("assignment_id", "assignment_id"),
                ("lease_id", "lease_id"),
                ("worktree_id", "worktree_id"),
            ):
                if isinstance(worker.get(source), str):
                    event[target] = worker[source]
        if disposition is not None:
            safe_disposition = re.sub(r"[^a-z0-9]+", "-", disposition.lower()).strip("-")
            if not safe_disposition:
                raise ControlPlaneError("audit disposition must be a safe non-empty class")
            event["disposition"] = safe_disposition
        encoded = (json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
        fd = os.open(self.audit_path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        try:
            os.write(fd, encoded)
            os.fsync(fd)
        finally:
            os.close(fd)

    def _server_identity_state(self, timeout_seconds: float | None = None) -> str:
        """Return only a safe class for the authoritative server handshake.

        The handshake is the sole runtime binding between this protected profile
        and a WSH server.  It intentionally accepts one exact JSON field and
        never returns, logs, or otherwise exposes the observed value.
        """
        try:
            result = self.runner.run(
                _render_command(self.profile.commands["wsh_identity"], self._template_values()),
                check=False,
                timeout_seconds=timeout_seconds if timeout_seconds is not None else self.profile.status_timeout_seconds,
                max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            return "unavailable"
        if result.returncode:
            return "unavailable"
        try:
            pairs = json.loads(result.stdout, object_pairs_hook=lambda values: values)
        except json.JSONDecodeError:
            return "malformed"
        if not isinstance(pairs, list) or any(not isinstance(pair, tuple) or len(pair) != 2 for pair in pairs):
            return "malformed"
        identities = [value for key, value in pairs if key == "server_identity"]
        if len(identities) != 1:
            return "missing" if not identities else "ambiguous"
        if len(pairs) != 1 or not isinstance(identities[0], str) or not NAME_RE.fullmatch(identities[0]):
            return "malformed"
        return "matched" if identities[0] == self.profile.wsh_server_identity else "mismatch"

    def _assert_server_identity(self, disposition: str) -> None:
        outcome = self._server_identity_state()
        with self._locked_state():
            self._audit("server-identity-validation", outcome, disposition=disposition)
        if outcome != "matched":
            raise ControlPlaneError(f"{disposition} rejected: profile WSH server identity is {outcome}")

    def _await_bootstrap_server_identity(self) -> None:
        """Bound only cold bootstrap's unavailable identity observation.

        The identity command remains the sole WSH call in this interval.  A
        successful observation is intentionally not reusable: the ordinary
        creation-time assertion below still binds the pending mutation.
        """
        deadline = time.monotonic() + self.profile.bootstrap_identity_wait_seconds
        outcome = "unavailable"
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            outcome = self._server_identity_state(min(self.profile.status_timeout_seconds, remaining))
            if outcome != "unavailable":
                break
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            time.sleep(min(0.1, remaining))
        with self._locked_state():
            self._audit("server-identity-readiness", outcome, disposition="bootstrap")
        if outcome != "matched":
            raise ControlPlaneError(f"bootstrap rejected: profile WSH server identity is {outcome}")

    def _expire_leases(self, state: dict[str, Any]) -> bool:
        changed = False
        now = datetime.now(timezone.utc)
        for assignment_id, worker in state["workers"].items():
            if worker.get("state") in {"released", "expired"}:
                continue
            try:
                expiry = datetime.fromisoformat(str(worker["expires_at"]))
            except (KeyError, ValueError):
                raise ControlPlaneError("worker lease is missing a valid expiry")
            if expiry <= now:
                worker["state"] = "expired"
                self._audit("lease-expired", "recorded", worker)
                changed = True
        return changed

    def _tmux_has_session(self) -> bool:
        return self.runner.run(["tmux", "has-session", "-t", self.profile.tmux_session], check=False).returncode == 0

    def _tmux_windows(self) -> set[str]:
        result = self.runner.run(
            ["tmux", "list-windows", "-t", self.profile.tmux_session, "-F", "#{window_name}"], check=False
        )
        if result.returncode:
            raise ControlPlaneError("unable to enumerate tmux control windows")
        return {line.strip() for line in result.stdout.splitlines() if line.strip()}

    def _template_values(self, **extra: str) -> dict[str, str]:
        values = {
            "profile_id": self.profile.profile_id,
            "tmux_session": self.profile.tmux_session,
            "wsh_server_name": self.profile.wsh_server_name,
            "planner_wsh_session_id": self.profile.planner_wsh_session_id,
            "repository_root": str(self.repository_root),
            "window_name": "",
            "wsh_session_id": "",
            "assignment_id": "",
            "issue": "",
            "role": "",
            "worktree": "",
            "lease_id": "",
            "assignment_prompt": "",
            "planner_assignment_prompt": "",
        }
        values.update(extra)
        return values

    @staticmethod
    def _assignment_prompt(issue: str, role: str) -> str:
        """Return bounded planner instruction context for an interactive Codex TUI.

        This is deliberately not a named-agent selector. Codex discovers
        repository instructions from AGENTS.md; the role is planner assignment
        context only and the text contains no worker-supplied prompt material.
        """
        return (
            f"Work only on GitHub issue {issue} as the assigned {role}. "
            "Accept task intake only from sprint_planner through this planner-provisioned assignment. "
            "Use only this assigned external worktree and read and follow AGENTS.md. "
            "Do not launch, attach to, inspect, or steer tmux or WSH sessions, and do not use WSH MCP. "
            "Only the planner uses WSH. If server-identity validation fails, do not retry, select another profile, or attempt cleanup; report MANAGER ATTENTION NEEDED for human-supervised recovery. "
            "Only cold bootstrap may wait briefly for an unavailable authoritative identity; every other identity failure is immediately fail-closed. "
            "Report progress, evidence, blockers, and every required decision to sprint_planner as MANAGER ATTENTION NEEDED. "
            "If assigned the quality_engineer role, use only an independently leased QA worktree and never share an implementation worktree."
            " Create a focused PR linked to this issue, monitor PR and issue feedback, and action every actionable review or issue comment through sprint_planner. "
            "Create a linked out-of-scope follow-up issue only with authority; otherwise escalate it to sprint_planner as MANAGER ATTENTION NEEDED. "
            "QA must validate independently with required checks, CODEOWNERS review where applicable, no unresolved blocker, and rollback review. "
            "Never merge: a human must explicitly authorize the exact PR head and named target. Before issue closeout, post the binding issue comment with PR, head, checks, QA, rollback, and merge authority evidence. "
            "Do not remove or delete the worktree or its local branch after merge. Only after GitHub confirms the authorized head merged into the named target may the planner use its planner-controlled cleanup path, with separately recorded explicit cleanup authority and proof that the worktree/session is clean, unpushed, unleased, and not needed."
        )

    @staticmethod
    def _planner_assignment_prompt() -> str:
        return (
            "Act as the sole sprint_planner for this protected runtime profile. "
            "Read and follow AGENTS.md, allocate only approved issue work through the control plane, "
            "and report evidence and blockers to the human supervisor."
            " Cold bootstrap alone may use the profile's bounded unavailable-only identity readiness wait after starting its local server; every other WSH session inventory, creation, relay, stop, or cleanup requires one fresh exact authoritative server-identity handshake. On missing, mismatch, unavailable, malformed, or ambiguous identity, fail closed without an alternate profile or stale client and escalate to the human supervisor; only this planner uses WSH."
            " Require focused issue-linked PRs, monitored/actioned feedback, MANAGER ATTENTION NEEDED escalation for unauthorized out-of-scope follow-ups or any required decision, independent QA, required checks/CODEOWNERS/no unresolved blocker, and rollback review. "
            "Require explicit human authorization of the exact PR head and named target before merge, binding issue-comment closeout evidence, and only after GitHub confirms the authorized head merged into the named target authorize the planner-controlled cleanup path to remove only that merged issue's leased worktree and local branch with separately recorded explicit cleanup authority plus clean/unpushed/unleased/not-needed proof. Any ambiguity is MANAGER ATTENTION NEEDED and leaves files and branches intact."
        )

    def _rollback_launch_reservation(self, assignment_id: str, record: dict[str, Any], action: str) -> None:
        """Remove only the lease created by this launch attempt and audit it."""
        with self._locked_state() as state:
            current = state["workers"].get(assignment_id)
            if current and current.get("lease_id") == record["lease_id"]:
                self._audit(action, "failed", record)
                del state["workers"][assignment_id]
                try:
                    self._write_state(state)
                except AtomicWriteError as exc:
                    # Do not free the shared lease when the deletion may not have
                    # survived. A retry can inspect the matching durable record.
                    raise ControlPlaneError("worker rollback persistence is uncertain; retain matching lease for recovery") from exc
        self._release_shared_lease(str(record["worktree_id"]), str(record["lease_id"]), int(record["registry_generation"]))

    def _tmux_command(self, command: list[str]) -> str:
        # The profile is trusted operational configuration.  All dynamic values
        # are name-validated or canonical paths and are quoted as one tmux argv.
        return shlex.join(command)

    def _ensure_window(self, window: str, command: list[str]) -> bool:
        if window in self._tmux_windows():
            return False
        self.runner.run(
            ["tmux", "new-window", "-d", "-t", self.profile.tmux_session, "-n", window, self._tmux_command(command)]
        )
        return True

    def bootstrap(self) -> dict[str, Any]:
        with self._locked_state() as state:
            if self._tmux_has_session() and self._tmux_windows() - {"wsh-server", "sprint-planner"}:
                raise ControlPlaneError("bootstrap refused: control tmux session contains an unexpected window")
            server_command = _render_command(self.profile.commands["wsh_server"], self._template_values(window_name="wsh-server"))
            if not self._tmux_has_session():
                self.runner.run(
                    [
                        "tmux",
                        "new-session",
                        "-d",
                        "-s",
                        self.profile.tmux_session,
                        "-n",
                        "wsh-server",
                        self._tmux_command(server_command),
                    ]
                )
            self._ensure_window("wsh-server", server_command)
            # Starting the local server is the only bootstrap step before the
            # authoritative handshake.  Do not create the planner session
            # until this profile is bound to the expected server identity.
        self._await_bootstrap_server_identity()
        planner_command = _render_command(
            self.profile.commands["planner"],
            self._template_values(window_name="sprint-planner", planner_assignment_prompt=self._planner_assignment_prompt()),
        )
        self._assert_server_identity("planner session creation")
        with self._locked_state() as state:
            if self._tmux_windows() - {"wsh-server", "sprint-planner"}:
                raise ControlPlaneError("bootstrap refused: control tmux session contains an unexpected window")
            self._ensure_window("sprint-planner", planner_command)
            state["server_window"] = "wsh-server"
            state["planner_window"] = "sprint-planner"
            self._audit("bootstrap", "succeeded")
            self._write_state(state)
        return self.status()

    def _assert_not_nested_wsh(self) -> None:
        present = [name for name in self.profile.forbidden_environment_variables if os.environ.get(name)]
        if present and os.environ.get("WSH_SESSION_ID") != self.profile.planner_wsh_session_id:
            raise ControlPlaneError("worker launch rejected from an existing WSH session")

    def _discover_worktree(self, candidate: Path) -> tuple[Path, str, str]:
        self._assert_not_nested_wsh()
        absolute = candidate.absolute()
        canonical = candidate.resolve()
        if absolute != canonical:
            raise ControlPlaneError("worker worktree must not be supplied through a symlink")
        listing = self._git("worktree", "list", "--porcelain").stdout.splitlines()
        worktrees = [line.removeprefix("worktree ") for line in listing if line.startswith("worktree ")]
        if not worktrees:
            raise ControlPlaneError("Git reported no worktrees")
        listed = {str(Path(path).resolve()) for path in worktrees}
        primary = Path(worktrees[0]).resolve()
        if canonical == primary:
            raise ControlPlaneError("worker launch rejected: the primary checkout cannot receive a writer lease")
        if str(canonical) not in listed:
            raise ControlPlaneError("worker launch rejected: worktree is not registered by this repository")
        top = self.runner.run(["git", "-C", str(canonical), "rev-parse", "--show-toplevel"]).stdout.strip()
        if Path(top).resolve() != canonical:
            raise ControlPlaneError("worker launch rejected: worktree root is not canonical")
        branch = self.runner.run(["git", "-C", str(canonical), "branch", "--show-current"]).stdout.strip()
        if not branch:
            raise ControlPlaneError("worker launch rejected: worktree must have an issue branch")
        if self.profile.base_ref:
            base = self.runner.run(["git", "-C", str(canonical), "merge-base", self.profile.base_ref, "HEAD"], check=False)
            if base.returncode:
                raise ControlPlaneError("worker launch rejected: worktree does not share the profile base ref")
        return canonical, branch, _fingerprint(str(canonical))

    def _planner_identity_state(self) -> str:
        """Boundedly verify the single profile-bound planner WSH identity."""
        server_identity = self._server_identity_state()
        if server_identity != "matched":
            return f"server-{server_identity}"
        values = self._template_values()
        try:
            result = self.runner.run(
                _render_command(self.profile.commands["wsh_status"], values), check=False,
                timeout_seconds=self.profile.status_timeout_seconds, max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            return "list-unavailable"
        if result.returncode:
            return "list-unavailable"
        session = self.profile.planner_wsh_session_id
        session_pattern = re.compile(rf"(?<![a-zA-Z0-9_-]){re.escape(session)}(?![a-zA-Z0-9_-])")
        matching_rows = [index for index, line in enumerate(result.stdout.splitlines()) if session_pattern.search(line)]
        if not matching_rows:
            return "absent"
        if len(matching_rows) != 1:
            return "duplicate"
        # `list` proves there is exactly one named session. Query that exact
        # session's read-only tag surface instead of interpreting aggregate
        # table continuation formatting for identity authorization.
        try:
            server_identity = self._server_identity_state()
            if server_identity != "matched":
                return f"server-{server_identity}"
            tags = self.runner.run(
                _render_command(self.profile.commands["planner_tag"], values), check=False,
                timeout_seconds=self.profile.status_timeout_seconds, max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            return "tag-unavailable"
        if tags.returncode:
            return "tag-unavailable"
        delimiter = re.search(rf"Session '{re.escape(session)}':(?P<tags>[\s\S]*)\Z", tags.stdout)
        if not delimiter:
            return "wrong-identity"
        tag_values = set(re.findall(r"[a-z0-9][a-z0-9_-]*", delimiter.group("tags")))
        expected_tag = f"planner-{self.profile.profile_id}"
        if expected_tag not in tag_values:
            return "wrong-identity"
        return "present"

    def _assert_planner_identity(self) -> None:
        self._assert_server_identity("worker allocation")
        with self._locked_state() as state:
            if state.get("planner_wsh_session_id") != self.profile.planner_wsh_session_id:
                raise ControlPlaneError("planner control state has a different WSH identity")
            if not self._tmux_has_session() or {"wsh-server", "sprint-planner"} - self._tmux_windows():
                raise ControlPlaneError("planner control session is not bootstrapped; run bootstrap first")
        identity = self._planner_identity_state()
        if identity != "present":
            raise ControlPlaneError(f"worker launch rejected: planner WSH identity is {identity}")

    def launch_worker(self, issue: str, role: str, worktree: Path) -> dict[str, Any]:
        issue = _safe_name(issue, "issue")
        role = _safe_name(role, "role")
        if role not in APPROVED_WORKER_ROLES:
            raise ControlPlaneError("worker launch rejected: role has no approved policy contract")
        assignment_id = f"{issue}-{role}"
        window = f"agent-{assignment_id}"
        session_id = f"{self.profile.worker_session_prefix}-{assignment_id}"
        _safe_name(window, "worker window")
        _safe_name(session_id, "WSH session")
        canonical, branch, worktree_id = self._discover_worktree(worktree)
        with self._locked_state() as state:
            existing = state["workers"].get(assignment_id)
            if existing and existing.get("worktree_id") != worktree_id:
                raise ControlPlaneError("assignment already has a different immutable worktree lease")
            if self._tmux_has_session():
                known_windows = {"wsh-server", "sprint-planner"}
                known_windows.update(
                    str(worker["window"])
                    for worker in state["workers"].values()
                    if isinstance(worker, dict) and isinstance(worker.get("window"), str)
                )
                if self._tmux_windows() - known_windows:
                    raise ControlPlaneError("worker launch rejected: control tmux session contains an unexpected window")
        # This fresh bounded list-presence check happens before any writer
        # lease allocation and rejects missing, duplicate, or wrongly tagged
        # planner identities.
        self._assert_planner_identity()
        shared_lease_id, shared_generation = self._reserve_shared_lease(worktree_id, assignment_id)

        def rollback_local_failure(message: str) -> None:
            # No terminal exists during this part of launch, so a failed local
            # validation or state write must not strand the shared reservation.
            try:
                self._release_shared_lease(worktree_id, shared_lease_id, shared_generation)
            except AtomicWriteError as exc:
                raise ControlPlaneError("provisional shared lease persistence is uncertain; retry recovery without manual deletion") from exc
            raise ControlPlaneError(message)

        with self._locked_state() as state:
            try:
                self._expire_leases(state)
            except Exception:
                self._release_shared_lease(worktree_id, shared_lease_id, shared_generation)
                raise
            try:
                bootstrapped = self._tmux_has_session() and not ({"wsh-server", "sprint-planner"} - self._tmux_windows())
            except Exception:
                rollback_local_failure("planner control session cannot be enumerated before worker creation")
            if not bootstrapped:
                rollback_local_failure("planner control session is not bootstrapped; run bootstrap first")
            workers: dict[str, dict[str, Any]] = state["workers"]
            existing = workers.get(assignment_id)
            if existing:
                if existing.get("worktree_id") == worktree_id and existing.get("wsh_session_id") == session_id:
                    return_after_lock = True
                else:
                    rollback_local_failure("assignment already has a different immutable worktree lease")
            else:
                return_after_lock = False
            if not return_after_lock:
                for worker in workers.values():
                    if worker.get("worktree_id") == worktree_id:
                        rollback_local_failure("worker launch rejected: worktree already has an exclusive lease")
                    if worker.get("wsh_session_id") == session_id:
                        rollback_local_failure("worker launch rejected: WSH session identifier is already allocated")
                try:
                    window_exists = window in self._tmux_windows()
                except Exception:
                    rollback_local_failure("tmux worker window cannot be enumerated before creation")
                if window_exists:
                    rollback_local_failure("worker launch rejected: tmux worker window already exists")
                lease_id = shared_lease_id
                record = {
                    "assignment_id": assignment_id,
                    "issue": issue,
                    "role": role,
                    "lease_id": lease_id,
                    "worktree_id": worktree_id,
                    "registry_generation": shared_generation,
                    "branch": branch,
                    "wsh_session_id": session_id,
                    "window": window,
                    "state": "reserved",
                    "created_at": _utc_now(),
                    "expires_at": (datetime.now(timezone.utc) + timedelta(seconds=self.profile.lease_ttl_seconds)).replace(microsecond=0).isoformat(),
                    "owner": self.profile.planner_owner,
                }
                try:
                    # Rendering and Git-derived template values are still in the
                    # provisional phase. Failures here release only this exact
                    # common lease because no terminal can exist yet.
                    values = self._template_values(
                        window_name=window,
                        wsh_session_id=session_id,
                        assignment_id=assignment_id,
                        issue=issue,
                        role=role,
                        worktree=str(canonical),
                        lease_id=lease_id,
                        assignment_prompt=self._assignment_prompt(issue, role),
                    )
                    command = _render_command(self.profile.commands["worker"], values)
                except Exception:
                    rollback_local_failure("worker command template could not be rendered")
                # Persist the exclusive lease before creating a terminal.  A second
                # planner process cannot win the same worktree during this gap.
                workers[assignment_id] = record
                try:
                    self._write_state(state)
                except AtomicWriteError as exc:
                    # The local state write did not prove a durable worker; remove
                    # the provisional shared lease only when the old state is
                    # still authoritative. Post-replace uncertainty is retained.
                    workers.pop(assignment_id, None)
                    if exc.after_replace:
                        raise ControlPlaneError("worker reservation persistence is uncertain; retry recovery without manual deletion") from exc
                    rollback_local_failure("worker reservation could not be persisted")
                except Exception:
                    workers.pop(assignment_id, None)
                    rollback_local_failure("worker reservation could not be persisted")
        if return_after_lock:
            return self.status()
        # Verify that the profile-selected server is reachable after reservation
        # but before terminal creation. This is bounded and outside the lock.
        try:
            preflight = self.runner.run(
                _render_command(self.profile.commands["wsh_status"], values),
                check=False,
                timeout_seconds=self.profile.status_timeout_seconds,
                max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            self._rollback_launch_reservation(assignment_id, record, "worker-preflight")
            raise
        if preflight.returncode:
            self._rollback_launch_reservation(assignment_id, record, "worker-preflight")
            raise ControlPlaneError("worker launch rejected: profile WSH preflight is unavailable")
        self._audit("worker-preflight", "succeeded", record)
        try:
            codex_preflight = self.runner.run(
                _render_command(self.profile.commands["codex_preflight"], values),
                check=False,
                timeout_seconds=self.profile.status_timeout_seconds,
                max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            self._rollback_launch_reservation(assignment_id, record, "worker-codex-preflight")
            raise
        if codex_preflight.returncode:
            self._rollback_launch_reservation(assignment_id, record, "worker-codex-preflight")
            raise ControlPlaneError("worker launch rejected: supported Codex invocation preflight is unavailable")
        self._audit("worker-codex-preflight", "succeeded", record)
        # Persist this monotonic handoff before invoking tmux. From the call
        # onward, an exception cannot prove that no process/window exists.
        try:
            with self._locked_state() as state:
                current = state["workers"].get(assignment_id)
                if not current or current.get("lease_id") != record["lease_id"]:
                    raise ControlPlaneError("worker lease changed before tmux creation; manual recovery is required")
                # This conservative state is durable before the call because a
                # process/window may exist even when tmux reports an error.
                current["state"] = "start-uncertain"
                self._audit("worker-tmux-create", "attempting", record)
                self._write_state(state)
        except AtomicWriteError as exc:
            if exc.after_replace:
                raise ControlPlaneError("worker tmux handoff persistence is uncertain; retain matching lease for recovery") from exc
            self._rollback_launch_reservation(assignment_id, record, "worker-tmux-handoff")
            raise
        except Exception:
            self._rollback_launch_reservation(assignment_id, record, "worker-tmux-handoff")
            raise
        # The WSH worker is started by tmux after the handoff is durable. Never
        # hold the planner state lock while the terminal creation call runs.
        try:
            self._assert_server_identity("worker session creation")
            self.runner.run(
                ["tmux", "new-window", "-d", "-t", self.profile.tmux_session, "-n", window, "-c", str(canonical), self._tmux_command(command)]
            )
        except Exception:
            with self._locked_state() as state:
                current = state["workers"].get(assignment_id)
                if current and current.get("lease_id") == record["lease_id"]:
                    # A failed create call may still have reached tmux. Preserve
                    # the lease until exact WSH and window absence is established.
                    current["state"] = "start-uncertain"
                    self._audit("worker-started", "uncertain", record)
                    self._write_state(state)
            raise
        with self._locked_state() as state:
            current = state["workers"].get(assignment_id)
            if not current or current.get("lease_id") != record["lease_id"]:
                raise ControlPlaneError("worker lease changed during startup; manual recovery is required")
            current["state"] = "starting"
            self._audit("worker-started", "succeeded", record)
            self._write_state(state)
        return self.status()

    def preflight(self) -> dict[str, str]:
        """Verify the selected local WSH and Codex CLI surfaces without a task."""
        self._assert_planner_identity()
        values = self._template_values()
        codex = self.runner.run(
            _render_command(self.profile.commands["codex_preflight"], values),
            check=False,
            timeout_seconds=self.profile.status_timeout_seconds,
            max_output_bytes=self.profile.status_output_bytes,
        )
        if codex.returncode:
            with self._locked_state():
                self._audit("preflight", "codex-unavailable")
            raise ControlPlaneError("supported Codex invocation preflight is unavailable")
        with self._locked_state():
            self._audit("preflight", "succeeded")
        return {"planner_wsh": "present", "codex": "accepted"}

    def _safe_wsh_status(self, worker: dict[str, Any]) -> dict[str, str]:
        values = self._template_values(
            wsh_session_id=str(worker["wsh_session_id"]),
            assignment_id=str(worker["assignment_id"]),
            issue=str(worker["issue"]),
            role=str(worker["role"]),
            lease_id=str(worker["lease_id"]),
        )
        try:
            self._assert_server_identity("worker status")
            result = self.runner.run(
                _render_command(self.profile.commands["wsh_status"], values),
                check=False,
                timeout_seconds=self.profile.status_timeout_seconds,
                max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            return {"state": "status-unavailable"}
        if result.returncode:
            return {"state": "status-unavailable"}
        try:
            raw = json.loads(result.stdout)
        except json.JSONDecodeError:
            # `wsh list` is the help-verified portable CLI status surface. It
            # carries no transcript data; match only the planner-issued name.
            session_id = str(worker["wsh_session_id"])
            if re.search(rf"(?<![a-zA-Z0-9_-]){re.escape(session_id)}(?![a-zA-Z0-9_-])", result.stdout):
                return {"session_id": session_id, "assignment_id": str(worker["assignment_id"]), "state": "listed"}
            return {"state": "status-unavailable"}
        if not isinstance(raw, dict):
            return {"state": "status-unavailable"}
        if raw.get("session_id") != worker["wsh_session_id"] or raw.get("assignment_id") != worker["assignment_id"]:
            return {"state": "status-unavailable"}
        safe: dict[str, str] = {}
        for key in SAFE_STATUS_FIELDS:
            value = raw.get(key)
            if not isinstance(value, str) or len(value.encode("utf-8")) > self.profile.status_field_bytes:
                continue
            if any(ord(character) < 32 for character in value):
                continue
            safe[key] = value
        return safe if "state" in safe else {"state": "status-unavailable"}

    def _wsh_session_presence(self, worker: dict[str, Any]) -> str:
        """Return present, absent, or unavailable for the exact leased session.

        A successful `wsh list` that does not contain the planner-issued name
        is evidence of absence. Any command/parse ambiguity is unavailable and
        therefore cannot release a potentially live writer lease.
        """
        values = self._template_values(
            wsh_session_id=str(worker["wsh_session_id"]),
            assignment_id=str(worker["assignment_id"]),
            issue=str(worker["issue"]),
            role=str(worker["role"]),
            lease_id=str(worker["lease_id"]),
        )
        try:
            self._assert_server_identity("worker session inspection")
            result = self.runner.run(
                _render_command(self.profile.commands["wsh_status"], values),
                check=False,
                timeout_seconds=self.profile.status_timeout_seconds,
                max_output_bytes=self.profile.status_output_bytes,
            )
        except ControlPlaneError:
            return "unavailable"
        if result.returncode:
            return "unavailable"
        session_id = str(worker["wsh_session_id"])
        if re.search(rf"(?<![a-zA-Z0-9_-]){re.escape(session_id)}(?![a-zA-Z0-9_-])", result.stdout):
            return "present"
        return "absent"

    def _worker_completed(self, worker: dict[str, Any]) -> bool:
        """Prove the adapter has no session and no tmux worker window left."""
        try:
            return self._wsh_session_presence(worker) == "absent" and str(worker["window"]) not in self._tmux_windows()
        except ControlPlaneError:
            return False

    def _status_snapshot(self) -> dict[str, Any]:
        with self._locked_state() as state:
            if self._expire_leases(state):
                self._write_state(state)
            return json.loads(json.dumps(state))

    def status(self) -> dict[str, Any]:
        # WSH calls happen after the state lock is released.  The snapshot has
        # only safe lease metadata and cannot expose terminal content.
        self._assert_server_identity("status")
        state = self._status_snapshot()
        workers = []
        for assignment_id, worker in sorted(state["workers"].items()):
            workers.append(
                {
                    "assignment_id": assignment_id,
                    "lease_id": worker["lease_id"],
                    "worktree_id": worker["worktree_id"],
                    "branch": worker["branch"],
                    "wsh_session_id": worker["wsh_session_id"],
                    "window": worker["window"],
                    "lifecycle": worker["state"],
                    "expires_at": worker["expires_at"],
                    "wsh": self._safe_wsh_status(worker),
                }
            )
        return {
            "profile_id": self.profile.profile_id,
            "planner_owner": self.profile.planner_owner,
            "tmux_session": self.profile.tmux_session,
            "planner_wsh_session_id": self.profile.planner_wsh_session_id,
            "planner_wsh": self._planner_identity_state(),
            "tmux_present": self._tmux_has_session(),
            "windows": sorted(self._tmux_windows()),
            "workers": workers,
        }

    def attach(self) -> None:
        self._assert_server_identity("attach")
        if not self._tmux_has_session():
            raise ControlPlaneError("planner control session is not bootstrapped; run bootstrap first")
        self.runner.run(["tmux", "attach-session", "-t", self.profile.tmux_session])

    def release_worker(self, issue: str, role: str, *, outcome: str | None = None, cleanup_authorized: bool = False) -> dict[str, Any]:
        if outcome not in {"merged", "abandoned"} or not cleanup_authorized:
            raise ControlPlaneError("worker release requires persisted merged/abandoned outcome and explicit cleanup authorization")
        assignment_id = f"{_safe_name(issue, 'issue')}-{_safe_name(role, 'role')}"
        self._assert_server_identity("worker release")
        with self._locked_state() as state:
            worker = state["workers"].get(assignment_id)
            if not worker:
                raise ControlPlaneError("no matching worker lease exists")
            if worker.get("state") == "stopping":
                raise ControlPlaneError("worker release is already in progress")
            worker = dict(worker)
            state["workers"][assignment_id]["cleanup_outcome"] = outcome
            state["workers"][assignment_id]["cleanup_authorized_at"] = _utc_now()
            state["workers"][assignment_id]["state"] = "stopping"
            self._write_state(state)
            values = self._template_values(
                window_name=str(worker["window"]),
                wsh_session_id=str(worker["wsh_session_id"]),
                assignment_id=assignment_id,
                issue=str(worker["issue"]),
                role=str(worker["role"]),
                lease_id=str(worker["lease_id"]),
            )
        # The bounded WSH stop is deliberately outside the state lock. A slow
        # local control command cannot block lease inspection or recovery.
        # A completed `wsh -c` session can disappear before release; reclaim
        # only when both the exact session and tmux worker window are absent.
        try:
            presence = self._wsh_session_presence(worker)
            if presence == "unavailable":
                raise ControlPlaneError("worker release cannot verify exact WSH session state")
            if presence == "absent":
                if str(worker["window"]) in self._tmux_windows():
                    raise ControlPlaneError("worker release cannot reclaim lease while tmux worker window remains")
            else:
                self._assert_server_identity("worker stop")
                self.runner.run(_render_command(self.profile.commands["wsh_stop"], values))
                if str(worker["window"]) in self._tmux_windows():
                    self.runner.run(["tmux", "kill-window", "-t", f"{self.profile.tmux_session}:{worker['window']}"])
            if not self._worker_completed(worker):
                raise ControlPlaneError("worker release could not prove worker session and window are finished")
        except Exception:
            # A session may complete in the narrow interval between list and
            # kill. Recover only if a second bounded check proves completion.
            if self._worker_completed(worker):
                pass
            else:
                with self._locked_state() as state:
                    current = state["workers"].get(assignment_id)
                    if current and current.get("lease_id") == worker["lease_id"]:
                        current["state"] = "release-failed"
                        self._audit("worker-released", "failed", worker)
                        self._write_state(state)
                raise
        with self._locked_state() as state:
            current = state["workers"].get(assignment_id)
            if not current or current.get("lease_id") != worker["lease_id"]:
                raise ControlPlaneError("worker lease changed during release; manual recovery is required")
            del state["workers"][assignment_id]
            self._audit("worker-released", "succeeded", worker)
            self._write_state(state)
        self._release_shared_lease(str(worker["worktree_id"]), str(worker["lease_id"]), int(worker["registry_generation"]))
        return self.status()

    def shutdown(self, *, recovery_authorized: bool = False) -> None:
        self._assert_server_identity("shutdown")
        with self._locked_state() as state:
            if state["workers"]:
                raise ControlPlaneError("shutdown refused while worker leases exist; release each worker first")
            windows = self._tmux_windows() if self._tmux_has_session() else set()
            if windows - {"wsh-server", "sprint-planner"}:
                raise ControlPlaneError("shutdown refused: control tmux session contains an unexpected window")
        # This transition serializes every profile sharing the Git common dir:
        # reservations see durable drain metadata before allocating a writer.
        if recovery_authorized:
            self._resume_repository_shutdown()
        else:
            self._begin_repository_shutdown()
        with self._locked_state() as state:
            if state["workers"]:
                raise ControlPlaneError("shutdown refused while worker leases exist; release each worker first")
            windows = self._tmux_windows() if self._tmux_has_session() else set()
            known = {"wsh-server", "sprint-planner"}
            unexpected = windows - known
            if unexpected:
                raise ControlPlaneError("shutdown refused: control tmux session contains an unexpected window")
        # Do not hold the protected state lock while invoking WSH. A fresh
        # identity result permits exact cleanup only; ambiguity fails closed.
        persisted_absence = recovery_authorized and self._has_planner_absence_proof() and "wsh-server" not in self._tmux_windows()
        planner_identity = self._planner_identity_state()
        if planner_identity not in {"present", "absent"} and not (persisted_absence and planner_identity == "list-unavailable"):
            raise ControlPlaneError(f"shutdown refused: planner WSH identity is {planner_identity}")
        if planner_identity == "present":
            self._assert_server_identity("planner stop")
            self.runner.run(_render_command(self.profile.commands["planner_stop"], self._template_values()))
            if self._planner_identity_state() != "absent":
                raise ControlPlaneError("shutdown refused: exact planner WSH session did not terminate")
        # This is the fresh exact WSH absence proof; killing the WSH server
        # window later intentionally makes a subsequent list unavailable.
        # Persist this proof before stopping the WSH server's status surface.
        # A later failure can then resume the protected drain without guessing.
        self._record_planner_absence_proof()
        planner_absence_proved = True
        with self._locked_state() as state:
            if state["workers"]:
                raise ControlPlaneError("shutdown refused while worker leases exist; release each worker first")
            windows = self._tmux_windows() if self._tmux_has_session() else set()
            known = {"wsh-server", "sprint-planner"}
            unexpected = windows - known
            if unexpected:
                raise ControlPlaneError("shutdown refused: control tmux session contains an unexpected window")
            for window in ("sprint-planner", "wsh-server"):
                if window in windows:
                    self.runner.run(["tmux", "kill-window", "-t", f"{self.profile.tmux_session}:{window}"])
            # A session created by this tool contains only the exact two windows;
            # do not kill an arbitrary named tmux session with extra windows.
            if self._tmux_has_session():
                self.runner.run(["tmux", "kill-session", "-t", self.profile.tmux_session])
            replacement = self._new_state()
            self._audit("shutdown", "succeeded")
            self._write_state(replacement)
        # Clear durable drain metadata only after fresh exact absence evidence.
        if not planner_absence_proved:
            raise ControlPlaneError("shutdown recovery cannot prove exact planner WSH absence")
        if self._tmux_has_session():
            raise ControlPlaneError("shutdown recovery cannot prove exact tmux control session absence")
        self._finish_repository_shutdown()


def _json_print(value: dict[str, Any]) -> None:
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="planner-owned local tmux/WSH control plane")
    parser.add_argument("--profile", required=True, type=Path, help="protected runtime JSON profile")
    subparsers = parser.add_subparsers(dest="action", required=True)
    subparsers.add_parser("bootstrap")
    subparsers.add_parser("status")
    subparsers.add_parser("preflight")
    subparsers.add_parser("attach")
    launch = subparsers.add_parser("launch-worker")
    launch.add_argument("--issue", required=True)
    launch.add_argument("--role", required=True)
    launch.add_argument("--worktree", required=True, type=Path)
    release = subparsers.add_parser("release-worker")
    release.add_argument("--issue", required=True)
    release.add_argument("--role", required=True)
    release.add_argument("--outcome", choices=("merged", "abandoned"), required=True)
    release.add_argument("--cleanup-authorized", action="store_true")
    shutdown = subparsers.add_parser("shutdown")
    shutdown.add_argument("--confirm-shutdown", action="store_true")
    recover_shutdown = subparsers.add_parser("recover-shutdown")
    recover_shutdown.add_argument("--cleanup-authorized", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv or sys.argv[1:])
    try:
        control = ControlPlane(Profile.load(args.profile))
        if args.action == "bootstrap":
            _json_print(control.bootstrap())
        elif args.action == "status":
            _json_print(control.status())
        elif args.action == "preflight":
            _json_print(control.preflight())
        elif args.action == "attach":
            control.attach()
        elif args.action == "launch-worker":
            _json_print(control.launch_worker(args.issue, args.role, args.worktree))
        elif args.action == "release-worker":
            _json_print(control.release_worker(args.issue, args.role, outcome=args.outcome, cleanup_authorized=args.cleanup_authorized))
        elif args.action == "shutdown":
            if not args.confirm_shutdown:
                raise ControlPlaneError("shutdown requires --confirm-shutdown")
            control.shutdown()
        elif args.action == "recover-shutdown":
            if not args.cleanup_authorized:
                raise ControlPlaneError("shutdown recovery requires --cleanup-authorized")
            control.shutdown(recovery_authorized=True)
        return 0
    except ControlPlaneError as exc:
        print(f"planner-wsh-control: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
