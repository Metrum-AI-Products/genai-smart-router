#!/usr/bin/env python3
"""Static security regression checks for live E2E harness scripts."""

from __future__ import annotations

import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    live_c = (ROOT / "scripts/live_cli_c_e2e.sh").read_text(encoding="utf-8")
    compose = (ROOT / "scripts/compose_live_e2e.sh").read_text(encoding="utf-8")

    require("docker run" in live_c, "C candidate execution must go through docker run")
    require("--network none" in live_c, "C sandbox must disable networking")
    require("--read-only" in live_c, "C sandbox must use a read-only root filesystem")
    require("--pids-limit" in live_c and "--memory" in live_c and "--cpus" in live_c, "C sandbox must set resource limits")
    require(not re.search(r"\n\s*cc\s+-std=c11", live_c), "live C harness must not compile directly on host")
    require(not re.search(r'\n\s*if "\$bin_file"', live_c), "live C harness must not execute generated binary on host")

    require("run_tool_sandbox" in compose, "compose tool smokes must use the sandbox wrapper")
    require("COMPOSE_E2E_TOOL_SANDBOX_IMAGE" in compose, "compose tool sandbox image must be explicit")
    require("COMPOSE_E2E_PERMISSIONS_IMAGE" in compose, "compose permissions helper image must be configurable")
    require("protect_compose_config_for_router" in compose, "compose config permissions helper is missing")
    require("chown -R 65532:65532 /config" in compose, "router UID 65532 must own bind-mounted config")
    require("find /config -type d -exec chmod 0700" in compose, "compose config directories must remain private")
    require("find /config -type f -exec chmod 0600" in compose, "compose config files must remain non-world-readable")
    require("reset_config_permissions" in compose, "cleanup must restore config ownership for retained/removable workdirs")
    require("--cap-drop ALL" in compose and "--security-opt no-new-privileges" in compose, "tool sandbox must drop privileges")
    require("--mount \"type=bind" in compose or "--mount type=bind" in compose, "tool sandbox must bind only the test workspace")
    bypass_index = compose.find("--dangerously-bypass-approvals-and-sandbox")
    sandbox_call_index = compose.find('run_tool_sandbox "$CODEX_TOOL_WORK"')
    require(bypass_index != -1 and sandbox_call_index != -1 and bypass_index < sandbox_call_index, "Codex bypass flag must be staged only for sandbox execution")
    require('chmod 0600 "$WORKDIR/config/config.yaml" "$WORKDIR/config/env.json" "$WORKDIR/config/scripts/router.ts"' in compose, "compose config files must be mode 0600 before ownership handoff")
    require('chmod 0700 "$WORKDIR" "$WORKDIR/config" "$WORKDIR/config/scripts"' in compose, "compose secret directories must be private")
    require("scrub_retained_secrets" in compose, "retained compose workdirs must scrub secret files")

    print("harness security self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
