#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate that sensitive local files are excluded from Docker build context."""

from __future__ import annotations

import fnmatch
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

FORBIDDEN_FIXTURES = [
    ".env",
    "env.json",
    "nested/env.json",
    "config.production.yaml",
    "ROUTER_TOKEN.txt",
    "ROUTER_TOKENS_prod.txt",
    "license.json",
    "config/license.json",
    "license-state.json",
    "config/license-state.json",
    "license-private-key.json",
    "license-signing.key",
    "signing-private-key.pem",
    "usage.sqlite",
    "requests.jsonl",
    "router-state.json",
    "tmp/debug.txt",
    "dist/package.tar.gz",
    "jobs/run/state.json",
]

ALLOWED_FIXTURES = [
    "config.example.yaml",
    "env.example.json",
    "docs/DEPLOYMENT.md",
    "scripts/router.ts",
]


def load_patterns(path: Path) -> list[str]:
    patterns: list[str] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        patterns.append(stripped)
    return patterns


def matches_pattern(path: str, pattern: str) -> bool:
    negated = pattern.startswith("!")
    if negated:
        pattern = pattern[1:]
    pattern = pattern.rstrip("/")
    if pattern.startswith("/"):
        pattern = pattern[1:]

    candidates = {path}
    parts = path.split("/")
    candidates.update(parts)

    if pattern.startswith("**/"):
        suffix = pattern[3:]
        return path == suffix or path.endswith("/" + suffix) or fnmatch.fnmatch(path, pattern)
    if "/" not in pattern:
        return any(fnmatch.fnmatch(candidate, pattern) for candidate in candidates)
    return fnmatch.fnmatch(path, pattern) or path.startswith(pattern + "/")


def included_by_dockerignore(path: str, patterns: list[str]) -> bool:
    included = True
    for pattern in patterns:
        negated = pattern.startswith("!")
        if matches_pattern(path, pattern):
            included = negated
    return included


def main() -> int:
    patterns = load_patterns(ROOT / ".dockerignore")
    errors: list[str] = []

    for fixture in FORBIDDEN_FIXTURES:
        if included_by_dockerignore(fixture, patterns):
            errors.append(f"{fixture} would enter Docker build context")

    for fixture in ALLOWED_FIXTURES:
        if not included_by_dockerignore(fixture, patterns):
            errors.append(f"{fixture} should remain in Docker build context")

    if errors:
        print("Docker context validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print("Docker context validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
