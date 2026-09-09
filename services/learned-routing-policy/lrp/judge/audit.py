# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Governed human judge audit sampling and agreement gates.

Protected review artifacts may hold content pointers only inside operator-owned
storage. Public reports and checked-in fixtures must never include prompts,
candidate text, tool payloads, or free-text judge reasons.
"""

from __future__ import annotations

import hashlib
import hmac
import os
from pathlib import Path
from typing import Any

from ..collect import (
    DataError,
    append_row,
    canonical,
    existing_rows,
    identity,
    journal,
    open_private,
    protected_path,
    read_rows,
    response_key,
    utc_now,
)

DEFAULT_SAMPLE_RATE = 0.02
DEFAULT_MIN_AGREEMENT = 0.8
DEFAULT_MIN_COVERAGE = 0.5
DEFAULT_MIN_REVIEWS = 5
AUDIT_QUEUE_SCHEMA = "lrp.human_audit_queue.v1"
AUDIT_REVIEW_SCHEMA = "lrp.human_audit_review.v1"
AUDIT_REPORT_SCHEMA = "lrp.human_audit_report.v1"


def _bounded_rate(value: float, name: str) -> float:
    if type(value) not in (int, float) or isinstance(value, bool) or not 0 < value <= 1:
        raise DataError(name)
    return float(value)


def _seed_bytes(seed: str | int | None) -> bytes:
    if seed is None:
        seed = "lrp-human-audit-v1"
    return str(seed).encode()


def should_sample(
    *,
    request_id: str,
    target: dict[str, Any],
    method: str,
    rate: float = DEFAULT_SAMPLE_RATE,
    seed: str | int | None = None,
) -> bool:
    """Deterministic Bernoulli sample over response identity; no content hashed."""
    rate = _bounded_rate(rate, "invalid_audit_sample_rate")
    if method not in {"pairwise_vs_anchor:v1", "absolute_rubric:v1"}:
        return False
    material = identity(
        {
            "request_id": request_id,
            "target": {"provider": target.get("provider"), "model": target.get("model")},
            "method": method,
            "seed": str(seed if seed is not None else "lrp-human-audit-v1"),
            "version": "human-audit-sample-v1",
        }
    )
    digest = hmac.new(_seed_bytes(seed), material.encode(), hashlib.sha256).digest()
    bucket = int.from_bytes(digest[:8], "big") / float(2**64)
    return bucket < rate


def queue_row_from_judgment(judgment: dict[str, Any]) -> dict[str, Any]:
    """Scalar-only audit queue row. Never copies content or free-text reasons."""
    detail = judgment.get("detail") or {}
    return {
        "schema_version": AUDIT_QUEUE_SCHEMA,
        "request_id": judgment["request_id"],
        "target": {
            "provider": judgment["target"]["provider"],
            "model": judgment["target"]["model"],
        },
        "source": judgment.get("source", "synthetic"),
        "method": judgment["method"],
        "judge_quality": judgment.get("quality"),
        "swapped_agree": detail.get("swapped_agree"),
        "votes": detail.get("votes"),
        "cache_key": judgment.get("cache_key"),
        "judge_model": judgment.get("judge_model"),
        "sampled_at": utc_now(),
        "content_included": False,
    }


def sample_judgments(
    *,
    judgments: Path,
    out: Path,
    rate: float = DEFAULT_SAMPLE_RATE,
    seed: str | int | None = None,
) -> dict[str, int]:
    """Write a protected audit queue from existing judgments (scalar fields only)."""
    rate = _bounded_rate(rate, "invalid_audit_sample_rate")
    out = protected_path(out)
    stats = {"written": 0, "skipped": 0, "eligible": 0}
    with journal(out) as handle:
        existing = {
            response_key(row): row
            for row in read_rows(out)
            if row.get("schema_version") == AUDIT_QUEUE_SCHEMA
        }
        for judgment in existing_rows(judgments, "judgment", response_key).values():
            if judgment.get("method") not in {
                "pairwise_vs_anchor:v1",
                "absolute_rubric:v1",
            }:
                continue
            stats["eligible"] += 1
            if response_key(judgment) in existing:
                stats["skipped"] += 1
                continue
            if not should_sample(
                request_id=judgment["request_id"],
                target=judgment["target"],
                method=judgment["method"],
                rate=rate,
                seed=seed,
            ):
                continue
            row = queue_row_from_judgment(judgment)
            append_row(handle, row)
            stats["written"] += 1
    return stats


def _validate_review(row: dict[str, Any]) -> dict[str, Any]:
    if row.get("schema_version") != AUDIT_REVIEW_SCHEMA:
        raise DataError("invalid_human_review")
    for field in ("request_id", "reviewed_at", "reviewer_id_hash"):
        if not isinstance(row.get(field), str) or not row[field]:
            raise DataError("invalid_human_review")
    if not isinstance(row.get("target"), dict):
        raise DataError("invalid_human_review")
    for field in ("provider", "model"):
        if not isinstance(row["target"].get(field), str) or not row["target"][field]:
            raise DataError("invalid_human_review")
    quality = row.get("human_quality")
    if (
        not isinstance(quality, (int, float))
        or isinstance(quality, bool)
        or not 0 <= float(quality) <= 1
    ):
        raise DataError("invalid_human_review")
    if row.get("content_included") is True:
        raise DataError("human_review_content_forbidden")
    for forbidden in ("reason", "notes", "prompt", "content", "messages"):
        if forbidden in row:
            raise DataError("human_review_content_forbidden")
    return {
        "schema_version": AUDIT_REVIEW_SCHEMA,
        "request_id": row["request_id"],
        "target": {
            "provider": row["target"]["provider"],
            "model": row["target"]["model"],
        },
        "human_quality": float(quality),
        "reviewer_id_hash": row["reviewer_id_hash"],
        "reviewed_at": row["reviewed_at"],
        "content_included": False,
    }


def _agreement(human: float, judge: float | None) -> bool | None:
    if judge is None:
        return None
    human_pass = human >= 0.5
    judge_pass = judge >= 0.5
    return human_pass == judge_pass


def build_agreement_report(
    *,
    queue: Path,
    reviews: Path,
    min_agreement: float = DEFAULT_MIN_AGREEMENT,
    min_coverage: float = DEFAULT_MIN_COVERAGE,
    min_reviews: int = DEFAULT_MIN_REVIEWS,
) -> dict[str, Any]:
    """Scalar agreement report suitable for public evidence (no content)."""
    min_agreement = _bounded_rate(min_agreement, "invalid_audit_min_agreement")
    min_coverage = _bounded_rate(min_coverage, "invalid_audit_min_coverage")
    if not isinstance(min_reviews, int) or isinstance(min_reviews, bool) or min_reviews < 1:
        raise DataError("invalid_audit_min_reviews")
    queue_rows = [
        row for row in read_rows(queue) if row.get("schema_version") == AUDIT_QUEUE_SCHEMA
    ]
    review_rows = [_validate_review(row) for row in read_rows(reviews)]
    by_key = {response_key(row): row for row in queue_rows}
    matched = 0
    agree = 0
    disagree = 0
    uncertain = 0
    for review in review_rows:
        queued = by_key.get(response_key(review))
        if queued is None:
            continue
        matched += 1
        result = _agreement(review["human_quality"], queued.get("judge_quality"))
        if result is None:
            uncertain += 1
        elif result:
            agree += 1
        else:
            disagree += 1
    sampled = len(queue_rows)
    coverage = (matched / sampled) if sampled else 0.0
    decided = agree + disagree
    agreement_rate: float | None = (agree / decided) if decided else None
    reasons: list[str] = []
    if sampled == 0:
        reasons.append("no_audit_sample")
    if matched < min_reviews:
        reasons.append("insufficient_reviews")
    if coverage < min_coverage:
        reasons.append("insufficient_coverage")
    if agreement_rate is None:
        reasons.append("no_comparable_reviews")
    elif agreement_rate < min_agreement:
        reasons.append("agreement_below_floor")
    return {
        "schema_version": AUDIT_REPORT_SCHEMA,
        "sampled_count": sampled,
        "reviewed_count": matched,
        "coverage": coverage,
        "agree_count": agree,
        "disagree_count": disagree,
        "uncertain_count": uncertain,
        "agreement_rate": agreement_rate,
        "min_agreement": min_agreement,
        "min_coverage": min_coverage,
        "min_reviews": min_reviews,
        "gate_passed": not reasons,
        "gate_reasons": reasons,
        "content_included": False,
        "reported_at": utc_now(),
    }


def write_agreement_report(report: dict[str, Any], out: Path) -> dict[str, Any]:
    out = protected_path(out)
    out.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    text = canonical(report) + "\n"
    fd = open_private(out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC)
    try:
        os.write(fd, text.encode())
        os.fsync(fd)
    finally:
        os.close(fd)
    return report
