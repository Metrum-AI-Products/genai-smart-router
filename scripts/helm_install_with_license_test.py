#!/usr/bin/env python3
"""Contract test for the license-backed Helm deployment wrapper."""

from __future__ import annotations

import os
import subprocess
import tempfile
import textwrap
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "helm_install_with_license.sh"


def write_executable(path: Path, body: str) -> None:
    first_line, *remaining_lines = body.splitlines()
    path.write_text(first_line + "\n" + textwrap.dedent("\n".join(remaining_lines)), encoding="utf-8")
    path.chmod(0o755)


def main() -> None:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        log = root / "commands.log"
        entitlement = root / "entitlement.yaml"
        config = root / "config.yaml"
        upstream_env = root / "env.json"
        entitlement.write_text("license_id: test\n", encoding="utf-8")
        config.write_text("server: {}\n", encoding="utf-8")
        upstream_env.write_text("{}\n", encoding="utf-8")

        kubeconfig = root / "config"
        kubeconfig.write_text("apiVersion: v1\n", encoding="utf-8")
        license_key = root / "license.key"
        secret_value = "must-not-appear-in-command-log"
        license_key.write_text(secret_value + "\n", encoding="utf-8")
        write_executable(
            root / "license-cli",
            """#!/usr/bin/env python3
            import os, pathlib, sys
            pathlib.Path(os.environ['COMMAND_LOG']).open('a').write('license ' + ' '.join(sys.argv[1:]) + '\\n')
            out = sys.argv[sys.argv.index('--out') + 1]
            pathlib.Path(out).write_text('{"signed":true}\\n')
            """,
        )
        write_executable(
            root / "kubectl",
            """#!/usr/bin/env python3
            import os, sys
            open(os.environ['COMMAND_LOG'], 'a').write('kubectl ' + ' '.join(sys.argv[1:]) + '\\n')
            if 'create' in sys.argv:
                print('apiVersion: v1\\nkind: Secret')
            """,
        )
        write_executable(
            root / "helm",
            """#!/usr/bin/env python3
            import os, sys
            open(os.environ['COMMAND_LOG'], 'a').write('helm ' + ' '.join(sys.argv[1:]) + '\\n')
            """,
        )

        env = os.environ | {
            "COMMAND_LOG": str(log),
            "LICENSE_CLI": str(root / "license-cli"),
            "KUBECTL": str(root / "kubectl"),
            "HELM": str(root / "helm"),
            "LICENSE_SIGNING_KEY_FILE": str(license_key),
        }
        result = subprocess.run(
            [
                str(SCRIPT),
                "--kubeconfig", str(kubeconfig),
                "--namespace", "router-test",
                "--release", "router-test",
                "--chart", "/tmp/chart",
                "--entitlement", str(entitlement),
                "--valid-for", "12h",
                "--config", str(config),
                "--env-file", str(upstream_env),
                "--image-repository", "smart-llmrouter",
                "--image-tag", "test",
            ],
            check=False,
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
        )
        assert result.returncode == 0, result.stderr
        commands = log.read_text(encoding="utf-8").splitlines()
        assert commands[0].startswith("license issue ")
        assert f"--key {license_key}" in commands[0]
        assert any(command.startswith(f"kubectl --kubeconfig {kubeconfig} create namespace router-test") for command in commands)
        assert any(command.startswith(f"kubectl --kubeconfig {kubeconfig} -n router-test create secret generic smart-llmrouter-secrets") for command in commands)
        assert sum(command.startswith(f"kubectl --kubeconfig {kubeconfig} apply -f -") for command in commands) == 2
        assert commands[-1].startswith("helm upgrade --install router-test /tmp/chart")
        assert secret_value not in "\n".join(commands)


if __name__ == "__main__":
    main()
