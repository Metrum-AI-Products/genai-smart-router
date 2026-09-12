#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate public docs release metadata and release-note hygiene."""

from __future__ import annotations

import re
import sys
from pathlib import Path

import canonical_product


ROOT = Path(__file__).resolve().parents[1]
DOCS_SITE = ROOT / "docs-site"
RELEASE_NOTES = DOCS_SITE / "docs" / "release-notes" / "index.md"

FORBIDDEN_RELEASE_NOTE_PATTERNS = [
    ("raw token or token hash", re.compile(r"(?i)(bearer\s+[A-Za-z0-9._~+/=-]{16,}|router[_-]?token|token[_-]?hash)")),
    ("provider API key", re.compile(r"(?i)(api[_-]?key\s*[:=]|provider[_-]?key)")),
    # llm-api.apps.metrum.ai is allowed only as https://llm-api.apps.metrum.ai/docs...
    ("private host/IP", re.compile(r"\b(?:100\.30\.225\.66|54\.84\.22\.33|llm-api-engg\.metrum\.ai|llm-api\.apps\.metrum\.ai|backups\.metrum\.ai)\b")),
    ("private AWS account", re.compile(r"\b121701826775\b")),
    ("private repository visibility claim", re.compile(r"(?i)repository visibility remains private")),
    ("private freeze commit in customer notes", re.compile(r"\ba77be222e2c35c2ec839e4be09387a9cbc57365f\b")),
    ("private SSH detail", re.compile(r"(?:\bubuntu@[A-Za-z0-9_.-]+|~/.ssh/[^\s'\"`]+\.pem|\bssh\s+-i\s+[^\n]+\.pem)")),
    ("production config path", re.compile(r"/opt/smart-llmrouter/compose/[^\s'\"`]+")),
    ("private license material", re.compile(r"(?i)(private signing key|customer-specific license payload value|signing-service credentials)")),
    ("stale MiniMax model name", re.compile(r"MiniMax-Text-01")),
    ("stale Qwen VLM activation candidate", re.compile(r"qwen/qwen3-vl-32b-instruct:nitro")),
    ("fictional competitor version", re.compile(r"(?i)fictional competitor")),
]


def require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        errors.append(message)


def main() -> int:
    errors: list[str] = []

    banner = DOCS_SITE / "src" / "components" / "DocsVersionBanner.js"
    banner_text = banner.read_text(encoding="utf-8") if banner.exists() else ""
    require(banner.exists(), "missing DocsVersionBanner component", errors)
    require('meta name="docs-version"' in banner_text, "DocsVersionBanner must emit docs-version meta tag", errors)
    require("routerLatestVersion" in banner_text, "DocsVersionBanner must render latest router version context", errors)

    root = DOCS_SITE / "src" / "theme" / "Root.js"
    root_text = root.read_text(encoding="utf-8") if root.exists() else ""
    require("DocsVersionBanner" in root_text, "Root.js must render DocsVersionBanner", errors)

    config = (DOCS_SITE / "docusaurus.config.js").read_text(encoding="utf-8")
    require("DOCS_ROUTER_VERSION" in config, "docusaurus config must read DOCS_ROUTER_VERSION", errors)
    require("DOCS_ROUTER_BUILD_DATE" in config, "docusaurus config must read DOCS_ROUTER_BUILD_DATE", errors)
    require("routerLatestVersion" in config, "docusaurus config must expose routerLatestVersion", errors)
    require("console.warn" in config, "docusaurus config must warn on version/latest mismatch", errors)

    releases_page = DOCS_SITE / "src" / "pages" / "releases.js"
    releases_text = releases_page.read_text(encoding="utf-8") if releases_page.exists() else ""
    require(releases_page.exists(), "missing releases page", errors)
    require("Release Notes" in releases_text, "releases page must link release notes", errors)

    release_text = RELEASE_NOTES.read_text(encoding="utf-8") if RELEASE_NOTES.exists() else ""
    require("### Highlights" in release_text, "release notes must contain at least one release entry with Highlights", errors)
    require("### Operator Impact" in release_text, "release notes must contain Operator Impact", errors)
    require("### Caller Impact" in release_text, "release notes must contain Caller Impact", errors)
    require("### Validation" in release_text, "release notes must contain Validation", errors)
    require("### Rollback" in release_text, "release notes must contain Rollback", errors)
    require("Current Package" in release_text or re.search(r"^## v?\d", release_text, re.MULTILINE), "release notes must include a current or versioned release entry", errors)

    for line_no, line in enumerate(release_text.splitlines(), start=1):
        privacy_line = canonical_product.strip_allowed_docs_urls(line)
        for label, pattern in FORBIDDEN_RELEASE_NOTE_PATTERNS:
            if pattern.search(privacy_line):
                errors.append(f"release notes line {line_no} contains {label}")

    if errors:
        print("docs versioning validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print("docs versioning validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
