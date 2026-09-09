#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Compatibility wrapper for the Go docs config example checker."""

from __future__ import annotations

import subprocess
import sys


def main() -> int:
    return subprocess.call(["go", "run", "./cmd/docs-config-example-check"])


if __name__ == "__main__":
    raise SystemExit(main())
