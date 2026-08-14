#!/usr/bin/env python3
"""fleet_sqlite_customer_deploy.py was moved into the packaged Fleet CLI.

One-release rename notice only. Use:
  metrum-genai-smartrouter-fleetctl customer create [--delete-first]
"""

from __future__ import annotations

import sys


def main() -> int:
    print(
        "scripts/fleet_sqlite_customer_deploy.py was replaced by "
        "metrum-genai-smartrouter-fleetctl customer create; install the "
        "fleet-admin binary package and invoke metrum-genai-smartrouter-fleetctl instead",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
