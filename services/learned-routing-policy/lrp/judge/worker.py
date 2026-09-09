# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Executed ONLY inside the configured private-root, no-network sandbox."""

import json
import os
import resource
import subprocess
import sys


def main() -> None:
    resource.setrlimit(resource.RLIMIT_CPU, (30, 30))
    resource.setrlimit(resource.RLIMIT_AS, (256 * 1024 * 1024,) * 2)
    resource.setrlimit(resource.RLIMIT_FSIZE, (1024 * 1024,) * 2)
    resource.setrlimit(resource.RLIMIT_NPROC, (32, 32))
    resource.setrlimit(resource.RLIMIT_NOFILE, (64, 64))
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    if os.getuid() == 0 or os.getcwd() != "/work":
        sys.exit(78)
    payload = json.load(sys.stdin)
    kind, spec, content = payload["kind"], payload["spec"], payload["content"]
    passed = False
    try:
        if kind == "exact":
            passed = content == spec["expected"]
        elif kind == "regex":
            import re

            passed = re.fullmatch(spec["pattern"], content) is not None
        elif kind == "json_schema":
            import jsonschema

            # Remote references are forbidden; no implicit retrieval of schema URLs.
            def check(value: object) -> None:
                if isinstance(value, dict):
                    for key, child in value.items():
                        if (
                            key in ("$ref", "$dynamicRef")
                            and isinstance(child, str)
                            and not child.startswith("#")
                        ):
                            raise ValueError("external_reference")
                        check(child)
                elif isinstance(value, list):
                    for child in value:
                        check(child)

            check(spec["schema"])
            jsonschema.validate(json.loads(content), spec["schema"])
            passed = True
        elif kind == "pytest":
            with open("solution.py", "x") as handle:
                handle.write(content)
            with open("test_solution.py", "x") as handle:
                handle.write(spec["tests"])
            with open(os.devnull, "wb") as sink:
                result = subprocess.run(
                    [
                        sys.executable,
                        "-I",
                        "-m",
                        "pytest",
                        "-q",
                        "-p",
                        "no:cacheprovider",
                        "/work/test_solution.py",
                    ],
                    stdout=sink,
                    stderr=sink,
                    env={"PATH": "/usr/bin", "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1"},
                    timeout=29,
                    check=False,
                )
            passed = result.returncode == 0
        else:
            sys.exit(78)
    except ImportError:
        sys.exit(78)
    except Exception:  # noqa: BLE001 - untrusted verifier exceptions never escape the isolated worker
        passed = False
    print(json.dumps({"passed": passed}), flush=True)


if __name__ == "__main__":
    main()
