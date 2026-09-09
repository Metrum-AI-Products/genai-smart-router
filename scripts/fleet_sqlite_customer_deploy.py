#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""fleet_sqlite_customer_deploy.py was moved into the packaged Fleet CLI.

One-release rename notice only. Use:
  metrum-genai-smartrouter-fleetctl customer write-manifest, then customer create --intent <signed-intent>
"""

from __future__ import annotations

import sys


def main() -> int:
    print(
        "scripts/fleet_sqlite_customer_deploy.py was replaced by "
        "metrum-genai-smartrouter-fleetctl customer write-manifest then create "
        "--intent <signed-intent>; install the fleet-admin binary package",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
