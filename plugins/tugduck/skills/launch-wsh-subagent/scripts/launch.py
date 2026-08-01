#!/usr/bin/env python3
"""Supervise a WSH server and coding-agent WSH client with PM2."""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


SESSION_NAME = re.compile(r"^[A-Za-z0-9._-]{1,64}$")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", required=True)
    parser.add_argument("--agent", required=True)
    parser.add_argument("--task", required=True)
    parser.add_argument("--session-name")
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def config_path(project_root: Path) -> Path:
    if value := os.environ.get("TUGDUCK_CONFIG"):
        return Path(value).expanduser()
    return project_root / ".tugduck" / "config.json"


def bootstrap_config(path: Path) -> bool:
    if path.is_file():
        return False
    if os.environ.get("TUGDUCK_CONFIG"):
        raise ValueError(f"TUGDUCK_CONFIG points to a missing config: {path}")
    template = Path(__file__).resolve().parents[3] / "config.example.json"
    if not template.is_file():
        raise RuntimeError("Tugduck plugin template is missing: config.example.json")
    path.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(template, path)
    path.chmod(0o600)
    return True


def load_config(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ValueError(f"Tugduck config not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ValueError(f"Tugduck config is not valid JSON: {path}") from exc
    if not isinstance(value, dict):
        raise ValueError("Tugduck config must contain an object")
    return value


def require_object(value: dict[str, Any], key: str) -> dict[str, Any]:
    result = value.get(key)
    if not isinstance(result, dict):
        raise ValueError(f"config.{key} must be an object")
    return result


def require_string(value: dict[str, Any], key: str) -> str:
    result = value.get(key)
    if not isinstance(result, str) or not result.strip():
        raise ValueError(f"config.{key} must be a non-empty string")
    return result


def string_list(value: Any, field: str) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ValueError(f"{field} must be an array of strings")
    return value


def string_map(value: Any, field: str) -> dict[str, str]:
    if not isinstance(value, dict) or not all(isinstance(key, str) and isinstance(item, str) for key, item in value.items()):
        raise ValueError(f"{field} must be an object of string values")
    return value


def pm2_processes() -> list[dict[str, Any]]:
    completed = subprocess.run(["pm2", "jlist"], text=True, capture_output=True, check=False)
    if completed.returncode != 0:
        raise RuntimeError(f"pm2 jlist failed: {completed.stderr.strip()}")
    try:
        value = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError("pm2 jlist did not return JSON") from exc
    return value if isinstance(value, list) else []


def pm2_start(command: str, name: str, cwd: str, args: list[str], env: dict[str, str], *, namespace: str | None = None, autorestart: bool) -> None:
    invocation = ["pm2", "start", command, "--name", name, "--cwd", cwd, "--interpreter", "none"]
    if namespace:
        invocation.extend(["--namespace", namespace])
    if not autorestart:
        invocation.append("--no-autorestart")
    invocation.extend(["--", *args])
    completed = subprocess.run(invocation, env={**os.environ, **env}, text=True, capture_output=True, check=False)
    if completed.returncode != 0:
        raise RuntimeError(f"pm2 start {name} failed: {completed.stderr.strip() or completed.stdout.strip()}")


def wait_for_wsh(base_url: str, timeout_seconds: int) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"{base_url.rstrip('/')}/health", timeout=2) as response:
                if response.status == 200:
                    return
        except urllib.error.URLError:
            time.sleep(0.5)
    raise RuntimeError("PM2 started the WSH process, but its configured health endpoint did not become ready")


def main() -> None:
    args = parse_args()
    config_file = config_path(Path(args.project_root).resolve())
    if bootstrap_config(config_file):
        print(f"Created {config_file}. Set its placeholder values and rerun Tugduck.")
        return
    config = load_config(config_file)
    wsh = require_object(config, "wsh")
    listen_address = require_string(wsh, "listen_address")
    listen_port = require_string(wsh, "listen_port")
    token = require_string(wsh, "token")
    base_url = f"http://{listen_address}:{listen_port}"
    server_name = require_string(wsh, "server_name")
    project_root = require_string(config, "project_root")
    agents = require_object(config, "agents")
    profile = agents.get(args.agent)
    if not isinstance(profile, dict):
        raise ValueError(f"agent profile not found: {args.agent}")
    agent_command = require_string(profile, "command")
    agent_args = string_list(profile.get("args", []), f"config.agents.{args.agent}.args")
    pm2 = require_object(config, "pm2")
    server = require_object(pm2, "server")
    client = require_object(pm2, "client")
    server_process = require_string(server, "name")
    server_command = require_string(server, "command")
    server_args = ["server", "--bind", f"{listen_address}:{listen_port}", "--server-name", server_name]
    server_env = {"WSH_TOKEN": token}
    client_command = require_string(client, "command")
    client_namespace = require_string(client, "namespace")
    client_prefix = require_string(client, "name_prefix")
    session_name = args.session_name or f"{client_prefix}-{args.agent}"
    if not SESSION_NAME.fullmatch(session_name):
        raise ValueError("session name must be 1-64 letters, digits, dots, hyphens, or underscores")
    environment = string_map(config.get("environment", {}), "config.environment")
    client_env = {**environment, "WSH_TOKEN": token}
    agent_launch = shlex.join([agent_command, *agent_args, args.task])
    client_args = ["-L", server_name, "--name", session_name, "-c", agent_launch]
    client_process = session_name
    if args.dry_run:
        print(json.dumps({
            "server": {"pm2_name": server_process, "command": server_command, "args": server_args, "env": {key: "[redacted]" for key in server_env}},
            "subagent_client": {"pm2_name": client_process, "namespace": client_namespace, "command": client_command, "args": client_args, "env": {key: "[redacted]" for key in client_env}},
        }, indent=2))
        return
    if not any(process.get("name") == server_process for process in pm2_processes()):
        pm2_start(server_command, server_process, project_root, server_args, server_env, autorestart=True)
    wait_for_wsh(base_url, 15)
    pm2_start(client_command, client_process, project_root, client_args, client_env, namespace=client_namespace, autorestart=False)
    print(json.dumps({"server_process": server_process, "subagent_process": client_process, "session": session_name, "agent": args.agent}, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
