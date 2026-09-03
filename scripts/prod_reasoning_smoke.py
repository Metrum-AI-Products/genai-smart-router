#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Compatibility wrapper for the standalone reasoning smoke script."""

from __future__ import annotations

import sys
import json

from reasoning_smoke import main


def run(argv: list[str]) -> int:
    try:
        return main(argv)
    except RuntimeError as exc:
        print(json.dumps({"event": "reasoning_smoke_failed", "error": str(exc)}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(run(sys.argv[1:]))
