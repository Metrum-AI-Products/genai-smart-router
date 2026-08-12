#!/usr/bin/env python3
"""Compatibility wrapper for fleet_customer_lifecycle.py create.

Operator-only. Prefer:
  rtk python3 scripts/fleet_customer_lifecycle.py create --customer-id <id>
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "fleet_customer_lifecycle.py"


def main() -> None:
    args = sys.argv[1:]
    if "--delete-first" in args:
        args = [a for a in args if a != "--delete-first"]
        customer = None
        for i, a in enumerate(args):
            if a == "--customer-id" and i + 1 < len(args):
                customer = args[i + 1]
                break
        if not customer:
            print("--delete-first requires --customer-id", file=sys.stderr)
            raise SystemExit(2)
        subprocess.run([sys.executable, str(SCRIPT), "delete", "--customer-id", customer], check=False)
    raise SystemExit(subprocess.call([sys.executable, str(SCRIPT), "create", *args]))


if __name__ == "__main__":
    main()
