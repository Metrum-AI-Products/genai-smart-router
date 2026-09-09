#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate license headers on eligible tracked first-party source files."""

from __future__ import annotations

from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
SOURCE_SUFFIXES = {
    ".c",
    ".cjs",
    ".css",
    ".go",
    ".h",
    ".html",
    ".js",
    ".jsx",
    ".mjs",
    ".py",
    ".sh",
    ".ts",
    ".tsx",
}
EXCLUDED_PREFIXES = (
    "docs-site/static/vendor/",
    "internal/router/admindist/static/",
)
EXCLUDED_FILES = {"internal/router/admindist/index.html"}

# Product copyright year for first-party Metrum notices (not third-party NOTICE
# entries). Update this when the product copyright year changes.
COPYRIGHT_YEAR = 2026
COPYRIGHT = f"Copyright {COPYRIGHT_YEAR} Metrum AI"
SPDX = "SPDX-License-Identifier: Apache-2.0"

# Any first-party "Copyright YYYY Metrum AI" must use COPYRIGHT_YEAR.
METRUM_COPYRIGHT_RE = re.compile(r"Copyright\s+(\d{4})\s+Metrum AI")
NOTICE_PATHS = ("NOTICE", "README.md", "LICENSE")


def tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    return sorted(path for path in result.stdout.decode().split("\0") if path)


def eligible_path(path: str) -> bool:
    return (
        Path(path).suffix in SOURCE_SUFFIXES
        and path not in EXCLUDED_FILES
        and not path.startswith(EXCLUDED_PREFIXES)
    )


def expected_lines(path: str) -> tuple[str, str]:
    suffix = Path(path).suffix
    if suffix in {".py", ".sh"}:
        return f"# {COPYRIGHT}", f"# {SPDX}"
    if suffix == ".css":
        return f"/* {COPYRIGHT} */", f"/* {SPDX} */"
    if suffix == ".html":
        return f"<!-- {COPYRIGHT} -->", f"<!-- {SPDX} -->"
    return f"// {COPYRIGHT}", f"// {SPDX}"


def has_header(path: str, text: str) -> bool:
    lines = text.splitlines()
    copyright_line, spdx_line = expected_lines(path)
    return any(
        lines[index : index + 2] == [copyright_line, spdx_line]
        for index in range(min(10, len(lines) - 1))
    )


def wrong_year_hits(text: str) -> list[int]:
    """Return years used in first-party Metrum copyright notices that are wrong."""
    return [
        int(match.group(1))
        for match in METRUM_COPYRIGHT_RE.finditer(text)
        if int(match.group(1)) != COPYRIGHT_YEAR
    ]


def main() -> int:
    missing: list[str] = []
    wrong_year: list[str] = []

    for path in tracked_files():
        if not eligible_path(path):
            continue
        text = (ROOT / path).read_text(encoding="utf-8")
        if len(text.splitlines()) >= 5 and not has_header(path, text):
            missing.append(path)
        years = wrong_year_hits(text)
        if years:
            wrong_year.append(f"{path} (found Copyright {sorted(set(years))} Metrum AI)")

    for path in NOTICE_PATHS:
        notice = ROOT / path
        if not notice.is_file():
            wrong_year.append(f"{path} (missing)")
            continue
        text = notice.read_text(encoding="utf-8")
        if COPYRIGHT not in text:
            wrong_year.append(f"{path} (missing {COPYRIGHT!r})")
        years = wrong_year_hits(text)
        if years:
            wrong_year.append(f"{path} (found Copyright {sorted(set(years))} Metrum AI)")

    failed = False
    if missing:
        failed = True
        print("Missing or invalid Apache-2.0 source header:", file=sys.stderr)
        for path in missing:
            print(f"  {path}", file=sys.stderr)
    if wrong_year:
        failed = True
        print(
            f"Wrong first-party copyright year (expected {COPYRIGHT_YEAR}):",
            file=sys.stderr,
        )
        for path in wrong_year:
            print(f"  {path}", file=sys.stderr)
    if failed:
        return 1
    print(
        "All eligible first-party source files have valid Apache-2.0 headers "
        f"and Copyright {COPYRIGHT_YEAR} Metrum AI notices."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
