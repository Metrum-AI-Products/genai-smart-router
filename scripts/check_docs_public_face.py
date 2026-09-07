#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Check public-facing docs for private markers and stale current-route claims."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parents[1]

PUBLIC_DOC_PATHS = [
    ROOT / "README.md",
    ROOT / "AGENTS.md",
    ROOT / "GOVERNANCE.md",
    ROOT / "SUPPORT.md",
    ROOT / "SECURITY.md",
    ROOT / "docs-site" / "docs",
    ROOT / "docs-site" / "src",
    ROOT / "docs" / "DOCKER_DEPLOYMENT.md",
    ROOT / "docs" / "DEPLOYMENT.md",
    ROOT / "docs" / "solution-brief.md",
    ROOT / "docs" / "PRODUCT_CAPABILITY_MATRIX.md",
    ROOT / "docs" / "SMOKE_TEST_MATRIX.md",
    ROOT / "docs" / "CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md",
    ROOT / "docs" / "TROUBLESHOOTING_RUNBOOK.md",
    ROOT / "docs" / "LICENSE_OPERATIONS.md",
    ROOT / "ops.env.example.json",
    ROOT / "config.example.yaml",
    ROOT / "deploy" / "Caddyfile.compose",
    ROOT / "examples" / "customer-lifecycle" / "onboard-acme.sandbox.example.json",
]

HISTORICAL_FILES = {
    Path("docs-site/docs/evaluation/harbor-case-study.mdx"),
    Path("docs/harbor-case-study.md"),
}

DOC_TYPE_VALUES = {"tutorial", "howto", "reference", "explanation"}
DOCS_SITE_DOCS = ROOT / "docs-site" / "docs"

PRIVATE_PATTERNS = [
    (
        "private production host/IP",
        re.compile(
            r"\b(?:"
            r"100\.30\.225\.66|54\.84\.22\.33|52\.3\.128\.72|"
            r"llm-api-engg\.metrum\.ai|llm-api\.metrum\.ai|llm-api\.apps\.metrum\.ai|"
            r"backups\.metrum\.ai"
            r")\b"
        ),
    ),
    ("private AWS account", re.compile(r"\b121701826775\b")),
    ("private backup bucket", re.compile(r"metrum-backups/smart-llmrouter|RESTIC_REPO_PATH.:\s*[\"']metrum-cto")),
    ("stale Metrum-issued license wording", re.compile(r"Metrum-issued")),
    (
        "private personal copy-paste email",
        re.compile(r"(?:CADDY_EMAIL=|email\s+\{?\$\{?CADDY_EMAIL:)chetan@metrum\.ai"),
    ),
    ("private real caller-id prefix", re.compile(r"rtr_metrum_chetan_metrum-insights_")),
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
        "stale active OpenRouter DeepSeek route claim",
        re.compile(r"\b(?:current|active|production|reference|hosted)[^\n.]{0,120}\bOpenRouter\b[^\n.]{0,120}\bDeepSeek\b|\bOpenRouter\b[^\n.]{0,120}\bDeepSeek\b[^\n.]{0,120}\b(?:current|active|production|reference|hosted)\b", re.IGNORECASE),
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
            yield from sorted(
                p
                for p in path.rglob("*")
                if p.is_file()
                and p.suffix in {".md", ".mdx", ".js", ".jsx", ".ts", ".tsx", ".json", ".yaml", ".yml"}
                and "node_modules" not in p.parts
            )
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


def doc_type_error(path: Path, text: str) -> str | None:
    if not path.is_relative_to(DOCS_SITE_DOCS) or path.suffix not in {".md", ".mdx"}:
        return None
    rel = path.relative_to(ROOT)
    if not text.startswith("---\n"):
        return f"{rel}: missing frontmatter with doc_type"
    end = text.find("\n---\n", 4)
    if end == -1:
        return f"{rel}: malformed frontmatter"
    frontmatter = text[4:end]
    matches = re.findall(r"^doc_type:\s*([A-Za-z_-]+)\s*$", frontmatter, flags=re.MULTILINE)
    if not matches:
        return f"{rel}: missing doc_type frontmatter"
    if len(matches) > 1:
        return f"{rel}: has multiple doc_type frontmatter fields"
    if matches[0] not in DOC_TYPE_VALUES:
        return f"{rel}: invalid doc_type {matches[0]!r}; expected one of {', '.join(sorted(DOC_TYPE_VALUES))}"
    return None


def main() -> int:
    errors: list[str] = []
    for path in iter_public_files():
        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        doc_type = doc_type_error(path, text)
        if doc_type:
            errors.append(doc_type)
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
