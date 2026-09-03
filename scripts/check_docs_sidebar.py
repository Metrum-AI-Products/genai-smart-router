#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate that every Docusaurus doc has one sidebar home."""

from __future__ import annotations

import json
import subprocess
import sys
from collections import Counter
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCS_DIR = ROOT / "docs-site" / "docs"
SIDEBARS = ROOT / "docs-site" / "sidebars.js"


def load_sidebars() -> object:
    script = """
const sidebars = require(process.argv[1]);
process.stdout.write(JSON.stringify(sidebars));
"""
    result = subprocess.run(
        ["node", "-e", script, str(SIDEBARS)],
        cwd=ROOT,
        check=True,
        text=True,
        capture_output=True,
    )
    return json.loads(result.stdout)


def collect_doc_ids(value: object) -> list[str]:
    ids: list[str] = []
    if isinstance(value, str):
        ids.append(value)
    elif isinstance(value, list):
        for item in value:
            ids.extend(collect_doc_ids(item))
    elif isinstance(value, dict):
        if value.get("type") == "doc" and isinstance(value.get("id"), str):
            ids.append(value["id"])
        if "items" in value:
            ids.extend(collect_doc_ids(value["items"]))
        elif "type" not in value:
            for item in value.values():
                ids.extend(collect_doc_ids(item))
    return ids


def docs_on_disk() -> set[str]:
    ids: set[str] = set()
    for path in DOCS_DIR.rglob("*"):
        if path.suffix not in {".md", ".mdx"}:
            continue
        rel = path.relative_to(DOCS_DIR).with_suffix("")
        doc_id = rel.as_posix()
        if doc_id.endswith("/index"):
            ids.add(doc_id[:-6])
        ids.add(doc_id)
    return ids


def main() -> int:
    sidebar_ids = collect_doc_ids(load_sidebars())
    counts = Counter(sidebar_ids)
    unique_sidebar_ids = set(sidebar_ids)
    disk_ids = docs_on_disk()

    missing = sorted(doc_id for doc_id in unique_sidebar_ids if doc_id not in disk_ids)
    orphaned = sorted(
        doc_id
        for doc_id in disk_ids
        if doc_id not in unique_sidebar_ids
        and not (doc_id.endswith("/index") and doc_id[:-6] in unique_sidebar_ids)
        and f"{doc_id}/index" not in unique_sidebar_ids
    )
    duplicates = sorted(doc_id for doc_id, count in counts.items() if count > 1)

    if not (missing or orphaned or duplicates):
        print(f"docs sidebar ok: {len(unique_sidebar_ids)} unique docs referenced")
        return 0

    if missing:
        print("Missing sidebar files:", file=sys.stderr)
        for doc_id in missing:
            print(f"  - {doc_id}", file=sys.stderr)
    if orphaned:
        print("Docs missing from sidebar:", file=sys.stderr)
        for doc_id in orphaned:
            print(f"  - {doc_id}", file=sys.stderr)
    if duplicates:
        print("Duplicate sidebar homes:", file=sys.stderr)
        for doc_id in duplicates:
            print(f"  - {doc_id} ({counts[doc_id]} entries)", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
