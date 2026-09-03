#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""fleet_customer_lifecycle.py was moved into the packaged Fleet CLI.

One-release rename notice only. Use:
  metrum-genai-smartrouter-fleetctl customer write-manifest|create|status|smoke|grant-caller|update-config|delete
"""

from __future__ import annotations

import sys


def main() -> int:
    print(
        "scripts/fleet_customer_lifecycle.py was replaced by "
        "metrum-genai-smartrouter-fleetctl customer …; install the fleet-admin "
        "binary package and invoke metrum-genai-smartrouter-fleetctl instead",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
