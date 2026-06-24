#!/usr/bin/env python3
"""Validate release package contents for package-safe docs and private markers."""

from __future__ import annotations

import argparse
import re
import sys
import tarfile
from pathlib import Path
from typing import Iterable


TEXT_SCAN_LIMIT = 10 * 1024 * 1024

FORBIDDEN_NAME_RE = re.compile(r"(^|/)PRODUCTION_RUNBOOK\.md$")
FORBIDDEN_TEXT_PATTERNS = [
    (
        "private production host marker",
        re.compile(r"\b(?:100\.30\.225\.66|llm-api-engg\.metrum\.ai)\b"),
    ),
    (
        "private SSH user or key path",
        re.compile(r"(?:\bubuntu@[A-Za-z0-9_.-]+|~/.ssh/[^\s'\"`]+\.pem|\bssh\s+-i\s+[^\n]+\.pem)"),
    ),
    (
        "live production compose/config/env path",
        re.compile(
            r"/opt/smart-llmrouter/compose/(?:"
            r"config/(?:config\.yaml|env\.json|scripts/router\.ts)|"
            r"ROUTER_TOKEN[^\s'\"`]*|"
            r"\.env|"
            r"state/router-state\.json|"
            r"logs/requests\.jsonl"
            r")"
        ),
    ),
    ("raw Anthropic API key", re.compile(r"\bsk-ant-[A-Za-z0-9_-]{20,}\b")),
    ("raw OpenRouter API key", re.compile(r"\bsk-or-v1-[A-Za-z0-9_-]{20,}\b")),
    ("raw OpenAI-style API key", re.compile(r"\bsk-[A-Za-z0-9_-]{20,}\b")),
    ("raw xAI API key", re.compile(r"\bxai-[A-Za-z0-9_-]{20,}\b")),
    (
        "raw router token",
        re.compile(r"\brtr_metrum(?:_[A-Za-z0-9-]+){4,}_[A-Za-z0-9_-]{20,}\b"),
    ),
    ("GitHub token", re.compile(r"\bgh[pousr]_[A-Za-z0-9_]{20,}\b")),
]


def load_allowlist(path: Path) -> set[str]:
    docs: set[str] = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        docs.add(Path(stripped).name)
    if not docs:
        raise SystemExit(f"{path} does not list any package docs")
    return docs


def package_relative_name(member_name: str) -> str:
    parts = Path(member_name).parts
    if len(parts) <= 1:
        return member_name
    return str(Path(*parts[1:]))


def is_docs_member(member_name: str) -> bool:
    rel = package_relative_name(member_name)
    return rel.startswith("docs/") and rel != "docs/"


def should_scan_text(member_name: str, size: int) -> bool:
    if size > TEXT_SCAN_LIMIT:
        return False
    suffixes = Path(member_name).suffixes
    if any(suffix in {".tar", ".gz", ".zip"} for suffix in suffixes):
        return False
    return True


def decode_text(blob: bytes) -> str | None:
    if b"\x00" in blob:
        return None
    try:
        return blob.decode("utf-8")
    except UnicodeDecodeError:
        return None


def validate_archive(archive: Path, allowed_docs: set[str]) -> list[str]:
    errors: list[str] = []
    actual_docs: set[str] = set()

    try:
        package = tarfile.open(archive, "r:*")
    except tarfile.TarError as exc:
        return [f"{archive}: cannot read tar archive: {exc}"]

    with package:
        for member in package.getmembers():
            rel = package_relative_name(member.name)
            if FORBIDDEN_NAME_RE.search(member.name):
                errors.append(f"{archive}: forbidden private runbook included: {rel}")

            if is_docs_member(member.name) and member.isfile():
                actual_docs.add(Path(rel).name)

            if not member.isfile() or not should_scan_text(member.name, member.size):
                continue

            extracted = package.extractfile(member)
            if extracted is None:
                continue
            text = decode_text(extracted.read(TEXT_SCAN_LIMIT + 1))
            if text is None:
                continue
            for label, pattern in FORBIDDEN_TEXT_PATTERNS:
                if pattern.search(text):
                    errors.append(f"{archive}: {rel} contains {label}")

    extra_docs = actual_docs - allowed_docs
    missing_docs = allowed_docs - actual_docs
    for doc in sorted(extra_docs):
        errors.append(f"{archive}: docs/{doc} is not in package docs allowlist")
    for doc in sorted(missing_docs):
        errors.append(f"{archive}: docs/{doc} from package docs allowlist is missing")

    return errors


def validate_archives(archives: Iterable[Path], allowlist: Path) -> list[str]:
    allowed_docs = load_allowlist(allowlist)
    errors: list[str] = []
    for archive in archives:
        errors.extend(validate_archive(archive, allowed_docs))
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allowlist", default="scripts/package_docs_allowlist.txt", type=Path)
    parser.add_argument("archives", nargs="+", type=Path)
    args = parser.parse_args()

    errors = validate_archives(args.archives, args.allowlist)
    if errors:
        print("package content validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print(f"validated {len(args.archives)} package artifact(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
