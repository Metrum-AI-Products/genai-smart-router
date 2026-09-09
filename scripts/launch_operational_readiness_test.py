#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import importlib.util
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("launch_readiness", ROOT / "scripts/launch_operational_readiness.py")
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main() -> int:
    gates = [gate for gate, _, _ in MODULE.CHECKS]
    assert gates == ["OPS-01", "OPS-03", "OPS-04", "OPS-05", "OPS-08"]
    flattened = " ".join(part for _, _, command in MODULE.CHECKS for part in command)
    for required in ("validate_release_matrix.py", "coding_agent_matrix.py", "FallbackOrdering429", "UsageDBSchemaIsRelationalOnly", "test-k8s-amd-instinct-local-serving"):
        assert required.lower() in flattened.lower(), required
    print("launch operational readiness contract test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
