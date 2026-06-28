#!/usr/bin/env python3
"""Validate release/build metadata before shell recipes consume it."""

from __future__ import annotations

import os
import re
import sys


RULES: dict[str, re.Pattern[str]] = {
    "VERSION": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+/-]{0,127}$"),
    "COMMIT": re.compile(r"^(?:unknown|[A-Fa-f0-9]{7,64})$"),
    "BUILD_DATE": re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$"),
    "GOOS": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$"),
    "GOARCH": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$"),
    "PKG_NAME": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$"),
    "DIST_DIR": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$"),
    "IMAGE_NAME": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$"),
    "IMAGE_TAG": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$"),
}


def main() -> int:
    errors: list[str] = []
    for name, pattern in RULES.items():
        value = os.environ.get(name, "")
        if not value:
            errors.append(f"{name} is empty")
            continue
        if value.startswith("/") or ".." in value.split("/"):
            errors.append(f"{name} contains an unsafe path component")
            continue
        if not pattern.fullmatch(value):
            errors.append(f"{name} contains unsupported characters")

    if errors:
        print("build metadata validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 2

    print("build metadata validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
