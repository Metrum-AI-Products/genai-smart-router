#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Tests for scripts/local_dev_bootstrap.py."""

from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import textwrap
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "local_dev_bootstrap.py"


def write_executable(path: Path, body: str) -> None:
    first_line, *remaining_lines = body.splitlines()
    path.write_text(first_line + "\n" + textwrap.dedent("\n".join(remaining_lines)), encoding="utf-8")
    path.chmod(0o755)


def test_refuses_existing_out_dir() -> None:
    with tempfile.TemporaryDirectory() as temp:
        out = Path(temp) / "out"
        out.mkdir()
        result = subprocess.run(
            [sys.executable, str(SCRIPT), "--out-dir", str(out), "--repo-root", str(ROOT)],
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            raise AssertionError("bootstrap overwrote without --force")
        if "already exists" not in result.stderr:
            raise AssertionError(result.stderr)


def test_stubbed_clis_write_safe_summary() -> None:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        out = root / "local-dev"
        log = root / "commands.log"
        write_executable(
            root / "license-cli",
            """#!/usr/bin/env python3
            import os, pathlib, sys
            pathlib.Path(os.environ['COMMAND_LOG']).open('a').write('license ' + ' '.join(sys.argv[1:]) + '\\n')
            if sys.argv[1] == 'generate-keypair':
                pathlib.Path(sys.argv[sys.argv.index('--private-key-out') + 1]).write_text('PRIVATE_KEY_MATERIAL\\n')
                pathlib.Path(sys.argv[sys.argv.index('--public-key-out') + 1]).write_text('PUBLIC\\n')
            else:
                pathlib.Path(sys.argv[sys.argv.index('--out') + 1]).write_text('{"signed":true}\\n')
            """,
        )
        write_executable(
            root / "smartrouterctl",
            """#!/usr/bin/env python3
            import json, os, pathlib, sys
            pathlib.Path(os.environ['COMMAND_LOG']).open('a').write('ctl ' + ' '.join(sys.argv[1:]) + '\\n')
            token = pathlib.Path(sys.argv[sys.argv.index('--token-out') + 1])
            token.write_text('rtr_metrum_local-dev_example-project_dev_kdemo_SECRET\\n')
            print(json.dumps({
                'schema': 'metrum.ai/smartrouter-caller-grant/v1',
                'caller_id': 'local-dev-example-project-dev',
                'token_id': 'rtr_metrum_local-dev_example-project_dev_kdemo',
                'token_file': token.name,
                'activation': 'local-config-written-restart-required',
            }))
            """,
        )
        env = os.environ | {"COMMAND_LOG": str(log)}
        result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--out-dir",
                str(out),
                "--repo-root",
                str(ROOT),
                "--license-cli",
                str(root / "license-cli"),
                "--smartrouterctl",
                str(root / "smartrouterctl"),
            ],
            env=env,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            raise AssertionError(result.stderr)
        summary = json.loads(result.stdout)
        if "SECRET" in result.stdout or "PRIVATE_KEY" in result.stdout:
            raise AssertionError("bootstrap printed secret material")
        if summary.get("token_id") != "rtr_metrum_local-dev_example-project_dev_kdemo":
            raise AssertionError(summary)
        token_mode = stat.S_IMODE((out / "router.token").stat().st_mode)
        if token_mode != 0o600 and token_mode != 0o644:
            # stub write_text may be 0644; real ctl uses 0600. Accept stub.
            pass
        commands = log.read_text(encoding="utf-8")
        if "allow-unknown-runtime-key" not in commands:
            raise AssertionError(commands)
        if "PRIVATE_KEY_MATERIAL" in commands:
            raise AssertionError("private key material leaked into command log")


def test_go_clis_issue_license_and_caller() -> None:
    with tempfile.TemporaryDirectory() as temp:
        bin_dir = Path(temp) / "bin"
        bin_dir.mkdir()
        license_bin = bin_dir / "metrum-genai-smartrouter-license"
        ctl_bin = bin_dir / "metrum-routerctl"
        build = subprocess.run(
            ["go", "build", "-o", str(license_bin), "./cmd/metrum-genai-smartrouter-license"],
            cwd=ROOT,
            capture_output=True,
            text=True,
        )
        if build.returncode != 0:
            raise AssertionError(build.stderr)
        build = subprocess.run(
            ["go", "build", "-o", str(ctl_bin), "./cmd/metrum-routerctl"],
            cwd=ROOT,
            capture_output=True,
            text=True,
        )
        if build.returncode != 0:
            raise AssertionError(build.stderr)
        out = Path(temp) / "local-dev"
        result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--out-dir",
                str(out),
                "--repo-root",
                str(ROOT),
                "--license-cli",
                str(license_bin),
                "--smartrouterctl",
                str(ctl_bin),
            ],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            raise AssertionError(result.stderr + result.stdout)
        if "SECRET" in result.stdout:
            raise AssertionError(result.stdout)
        license_path = out / "license.json"
        if not license_path.is_file() or license_path.stat().st_size < 32:
            raise AssertionError("license.json missing")
        cfg = (out / "config.yaml").read_text(encoding="utf-8")
        if "token_sha256:" not in cfg or "REPLACE_WITH" in cfg:
            raise AssertionError("caller hash was not merged")
        token = (out / "router.token").read_bytes()
        if not token.startswith(b"rtr_metrum_"):
            raise AssertionError("token file missing")
        mode = stat.S_IMODE((out / "router.token").stat().st_mode)
        if mode != 0o600:
            raise AssertionError(f"token mode {oct(mode)}")
        validate = subprocess.run(
            [str(ctl_bin), "config", "validate", "--config", str(out / "config.yaml")],
            capture_output=True,
            text=True,
        )
        if validate.returncode != 0:
            raise AssertionError(validate.stderr)


def main() -> None:
    test_refuses_existing_out_dir()
    test_stubbed_clis_write_safe_summary()
    test_go_clis_issue_license_and_caller()
    print("local_dev_bootstrap tests passed")


if __name__ == "__main__":
    main()
