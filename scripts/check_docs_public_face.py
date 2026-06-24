#!/usr/bin/env python3
"""Check public-facing docs for private markers and stale current-route claims."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parents[1]

PUBLIC_DOC_PATHS = [
    ROOT / "README.md",
    ROOT / "docs-site" / "docs",
    ROOT / "docs" / "DOCKER_DEPLOYMENT.md",
    ROOT / "docs" / "DEPLOYMENT.md",
    ROOT / "docs" / "solution-brief.md",
    ROOT / "docs" / "PRODUCT_CAPABILITY_MATRIX.md",
    ROOT / "docs" / "SMOKE_TEST_MATRIX.md",
]

HISTORICAL_FILES = {
    Path("docs-site/docs/evaluation/harbor-case-study.mdx"),
    Path("docs/harbor-case-study.md"),
}

PRIVATE_PATTERNS = [
    ("private production host/IP", re.compile(r"\b(?:100\.30\.225\.66|llm-api-engg\.metrum\.ai)\b")),
    ("private SSH detail", re.compile(r"(?:\bubuntu@[A-Za-z0-9_.-]+|~/.ssh/[^\s'\"`]+\.pem|\bssh\s+-i\s+[^\n]+\.pem)")),
    (
        "live production compose/config path",
        re.compile(
            r"/opt/smart-llmrouter/compose/(?:"
            r"config/(?:config\.yaml|env\.json|scripts/router\.ts)|"
            r"ROUTER_TOKEN[^\s'\"`]*|"
            r"\.env|state/router-state\.json|logs/requests\.jsonl"
            r")"
        ),
    ),
]

STALE_CURRENT_ROUTE_PATTERNS = [
    (
        "stale active DeepSeek route claim",
        re.compile(r"\b(?:current|active|production|reference|hosted)[^\n.]{0,120}\bDeepSeek\b|\bDeepSeek\b[^\n.]{0,120}\b(?:current|active|production|reference|hosted)\b", re.IGNORECASE),
    ),
    (
        "stale active Qwen route claim",
        re.compile(r"\b(?:current|active|production|reference|hosted)[^\n.]{0,120}\bQwen 3\.6\b|\bQwen 3\.6\b[^\n.]{0,120}\b(?:current|active|production|reference|hosted)\b", re.IGNORECASE),
    ),
    (
        "stale Crusoe catalog-only claim for Gemma",
        re.compile(r"\bCrusoe\b[^\n.]{0,160}\bGemma\b[^\n.]{0,160}\bcatalog-only\b|\bcatalog-only\b[^\n.]{0,160}\bCrusoe\b[^\n.]{0,160}\bGemma\b", re.IGNORECASE),
    ),
]


def iter_public_files() -> Iterable[Path]:
    for path in PUBLIC_DOC_PATHS:
        if path.is_dir():
            yield from sorted(p for p in path.rglob("*") if p.is_file() and p.suffix in {".md", ".mdx"})
        elif path.exists():
            yield path


def line_errors(path: Path, line_no: int, line: str) -> Iterable[str]:
    rel = path.relative_to(ROOT)
    if rel not in HISTORICAL_FILES:
        for label, pattern in PRIVATE_PATTERNS:
            if pattern.search(line):
                yield f"{rel}:{line_no}: contains {label}"

    for label, pattern in STALE_CURRENT_ROUTE_PATTERNS:
        if pattern.search(line):
            yield f"{rel}:{line_no}: contains {label}; mark historical or update to config.example.yaml"


def main() -> int:
    errors: list[str] = []
    for path in iter_public_files():
        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        for idx, line in enumerate(text.splitlines(), start=1):
            errors.extend(line_errors(path, idx, line))

    if errors:
        print("public docs QA failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print("public docs QA passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
