#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate license headers on eligible tracked first-party source files."""

from pathlib import Path
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
COPYRIGHT = "Copyright 2006 Metrum AI"
SPDX = "SPDX-License-Identifier: Apache-2.0"


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


def main() -> int:
    missing = []
    for path in tracked_files():
        if not eligible_path(path):
            continue
        text = (ROOT / path).read_text(encoding="utf-8")
        if len(text.splitlines()) >= 5 and not has_header(path, text):
            missing.append(path)
    if missing:
        print("Missing or invalid Apache-2.0 source header:", file=sys.stderr)
        for path in missing:
            print(f"  {path}", file=sys.stderr)
        return 1
    print("All eligible first-party source files have valid Apache-2.0 headers.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
