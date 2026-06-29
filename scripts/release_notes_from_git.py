#!/usr/bin/env python3
"""Draft public release notes from Git release tags."""

from __future__ import annotations

import re
import subprocess
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "docs-site" / "docs" / "release-notes" / "index.md"

FORBIDDEN_PATTERNS = [
    re.compile(r"(?i)(api[_-]?key|bearer\s+[A-Za-z0-9._~+/=-]{16,}|router[_-]?token|token[_-]?hash)"),
    re.compile(r"(?i)(private[_-]?key|license payload|signing-service credentials)"),
    re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b"),
    re.compile(r"~/.ssh/[^\s'\"`]+"),
    re.compile(r"/opt/smart-llmrouter/compose/[^\s'\"`]+"),
    re.compile(r"MiniMax-Text-01"),
    re.compile(r"qwen/qwen3-vl-32b-instruct:nitro"),
    re.compile(r"(?i)fictional competitor"),
]


@dataclass(frozen=True)
class Release:
    tag: str
    date: str
    subjects: list[str]


def git(args: list[str]) -> str:
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def clean_subject(subject: str) -> str | None:
    subject = subject.strip()
    if not subject:
      return None
    if any(pattern.search(subject) for pattern in FORBIDDEN_PATTERNS):
      return None
    subject = re.sub(r"^Merge pull request #(\d+) from [^\s]+", r"PR #\1", subject)
    subject = re.sub(r"\s+", " ", subject)
    return subject


def release_tags() -> list[str]:
    tags = git(["tag", "--list", "--sort=creatordate"])
    return [tag for tag in tags.splitlines() if tag.strip()]


def tag_date(tag: str) -> str:
    value = git(["log", "-1", "--format=%cs", tag])
    return value or "unknown"


def subjects_between(previous: str | None, tag: str) -> list[str]:
    rev = tag if previous is None else f"{previous}..{tag}"
    log = git(["log", "--format=%s", "--merges", rev])
    subjects = [clean_subject(line) for line in log.splitlines()]
    cleaned = [line for line in subjects if line]
    if cleaned:
        return cleaned

    fallback = git(["log", "--format=%s", "--max-count=12", rev])
    subjects = [clean_subject(line) for line in fallback.splitlines()]
    return [line for line in subjects if line]


def releases_from_tags(tags: list[str]) -> list[Release]:
    releases: list[Release] = []
    previous: str | None = None
    for tag in tags:
        releases.append(Release(tag=tag, date=tag_date(tag), subjects=subjects_between(previous, tag)))
        previous = tag
    return list(reversed(releases))


def entry(release: Release) -> str:
    highlights = release.subjects[:8] or ["Release package published."]
    bullet_lines = "\n".join(f"- {subject}" for subject in highlights)
    return f"""## {release.tag} - {release.date}

### Highlights

{bullet_lines}

### Operator Impact

- Config: review package notes for config changes before deployment.
- Database: verify release validation notes and migration guidance before deployment.
- License: verify entitlement, renewal, and validation guidance before deployment.
- Metrics and reports: review changed operational surfaces before rollout.

### Caller Impact

- API behavior: review changed caller-visible surfaces before rollout.
- Model groups: verify `/v1/models` for each caller class after deployment.
- Errors: review release-specific validation notes for changed errors.

### Validation

- `/readyz`: run after deployment.
- `/version`: confirm router version and build timestamp.
- `/v1/models`: run with a test caller.
- Completion smoke: run for changed model groups and API skins.
- Admin report smoke: run when enabled.

### Rollback

- Restore the previous router package.
- Restore the previous reviewed config and license file if they changed.
- Restore database backup only if this release includes a non-reversible schema or data migration.
"""


def current_preamble() -> str:
    return """---
title: Release Notes
---

# Release Notes

Release notes help customer operators decide whether to deploy, how to validate the release, and how to roll back if needed. Each packaged router build also displays its router version and build timestamp on every docs page.

For upgrade execution, see [Upgrade Guide](/docs/release-notes/upgrade-guide). For the docs package index, see [Releases](/docs/releases).
"""


def template_section() -> str:
    return """
## Release Note Fields

| Field | Purpose |
|---|---|
| Release version | Identifies the package installed on the deployment. |
| Build timestamp | Confirms the running binary matches the shipped artifact. |
| Package architecture | Confirms `linux-amd64` or `linux-arm64` package selection. |
| Config changes | Shows whether operators must update model groups, callers, admin policy, database settings, or license paths. |
| Database changes | Identifies migration, backup, and rollback considerations. |
| License changes | Identifies new feature gates, limits, or license replacement needs. |
| Client-visible behavior | Explains API, error, model metadata, or routing changes. |
| Validation | Lists readiness, model, report, metrics, and caller smokes. |
| Rollback | Names the prior package and config artifact to restore. |

Do not include private hostnames, SSH details, raw tokens, token hashes, provider API keys, full production config, private signing details, or customer-specific license payloads in public release notes.
"""


def main() -> int:
    tags = release_tags()
    if not tags:
        print("No Git release tags found; leaving existing release notes unchanged.")
        return 0

    releases = releases_from_tags(tags)
    body = "\n\n".join(entry(release) for release in releases)
    OUTPUT.write_text(f"{current_preamble()}\n\n{body}\n{template_section()}", encoding="utf-8")
    print(f"Wrote {len(releases)} release note entries to {OUTPUT.relative_to(ROOT)}.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
