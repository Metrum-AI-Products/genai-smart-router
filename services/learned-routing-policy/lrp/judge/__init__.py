# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Versioned, position-swapped judgments with content-bound protected caching."""

from __future__ import annotations

import asyncio
import os
import sqlite3
from pathlib import Path
from typing import Any

import httpx

from ..collect import (
    DataError,
    _check_file,
    append_row,
    authorize_content,
    canonical,
    existing_rows,
    identity,
    journal,
    open_private,
    protected_path,
    response_key,
    strict_json,
    utc_now,
    validate_record,
)
from ..fanout import bounded_json, refresh_prices, usage_and_cost
from .sandbox import Sandbox

__all__ = ["Sandbox", "cache_identity", "judge_requests"]
PROMPT_DIR = Path(__file__).with_name("prompts")


def cache_identity(
    request: dict[str, Any],
    response: dict[str, Any],
    anchor: dict[str, Any] | None,
    judge_model: str,
    settings: dict[str, Any],
    sandbox: Sandbox | None = None,
) -> str:
    # Hash full response content/tools/status and usage evidence, both model identities,
    # input instructions, verifier spec/code and prompt bytes, not merely request ID.
    return identity(
        {
            "request": request,
            "response": response,
            "anchor": anchor,
            "judge_model": judge_model,
            "settings": settings,
            "pairwise": (PROMPT_DIR / "pairwise_v1.md").read_text(),
            "absolute": (PROMPT_DIR / "absolute_v1.md").read_text(),
            "worker": Path(__file__).with_name("worker.py").read_text(),
            "sandbox": {"rootfs": str(sandbox.rootfs), "timeout": sandbox.timeout_s}
            if sandbox
            else None,
            "version": "judge-v1",
        }
    )


def _strict_verdict(content: str, absolute: bool) -> dict[str, Any]:
    verdict = strict_json(content)
    field = "score" if absolute else "winner"
    if (
        set(verdict) != {field, "reason"}
        or not isinstance(verdict["reason"], str)
        or len(verdict["reason"]) > 512
        or "\n" in verdict["reason"]
    ):
        raise DataError("invalid_verdict")
    if absolute:
        if type(verdict[field]) not in (int, float) or not 0 <= verdict[field] <= 10:
            raise DataError("invalid_verdict")
    elif verdict[field] not in ("A", "B", "TIE"):
        raise DataError("invalid_verdict")
    # Reason is intentionally discarded: free text can repeat protected content.
    return {field: verdict[field]}


async def judge_requests(
    *,
    requests: Path,
    responses: Path,
    out: Path,
    anchor: tuple[str, str],
    judge_model: str,
    client: httpx.AsyncClient | None = None,
    approved_content: bool = False,
    sandbox: Sandbox | None = None,
    cache: Path | None = None,
    timeout_s: float = 120,
    temperature: float = 0,
    max_tokens: int = 512,
) -> dict[str, int]:
    if (
        not 0 < timeout_s <= 120
        or not 1 <= max_tokens <= 4096
        or not 0 <= temperature <= 2
        or not judge_model
    ):
        raise DataError("invalid_judge_settings")
    reqs = existing_rows(requests, "request", lambda row: row["request_id"])
    resps = existing_rows(responses, "response", response_key)
    authorize_content(list(reqs.values()), approved_content)
    grouped: dict[str, list[dict[str, Any]]] = {}
    for response in resps.values():
        request = reqs.get(response["request_id"])
        if request is None or request["source"] != response["source"]:
            raise DataError("response_request_join_failed")
        if response.get("request_hash") and response["request_hash"] != identity(
            request
        ):
            raise DataError("response_request_content_changed")
        grouped.setdefault(response["request_id"], []).append(response)
    settings = {
        "temperature": temperature,
        "max_tokens": max_tokens,
        "timeout_s": timeout_s,
    }
    cache = protected_path(cache or out.with_suffix(".sqlite"))
    cache.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = open_private(cache, os.O_RDWR | os.O_CREAT)
    try:
        _check_file(fd)
    finally:
        os.close(fd)
    for suffix in ("-journal", "-wal", "-shm"):
        sidecar = Path(str(cache) + suffix)
        if sidecar.exists() or sidecar.is_symlink():
            sidecar_fd = open_private(sidecar, os.O_RDONLY)
            os.close(sidecar_fd)
    stats = {"written": 0, "skipped": 0, "cached": 0, "uncertain": 0}
    owned_client = client is None
    connection = sqlite3.connect(cache)
    # DELETE journal inherits private db permissions. No raw content in keys.
    connection.execute("PRAGMA journal_mode=DELETE")
    connection.execute(
        "CREATE TABLE IF NOT EXISTS judgments (cache_key TEXT PRIMARY KEY, verdict TEXT NOT NULL)"
    )
    prices: dict[str, dict[str, Any]] | None = None

    async def vote(
        payload: dict[str, Any], absolute: bool
    ) -> tuple[dict[str, Any] | None, float | None]:
        nonlocal client, prices
        if client is None:
            token = os.environ.get("OPENROUTER_API_KEY", "")
            if not token:
                raise DataError("judge_credential_required")
            client = httpx.AsyncClient(
                trust_env=False, headers={"Authorization": "Bearer " + token}
            )
        if prices is None:
            prices = await refresh_prices(client, [judge_model])
        pricing = prices.get(judge_model)
        if pricing is None:
            raise DataError("judge_pricing_unavailable")
        prompt = (
            PROMPT_DIR / ("absolute_v1.md" if absolute else "pairwise_v1.md")
        ).read_text()
        cost: float | None = 0.0
        for _ in range(2):
            try:
                status, result, _, _ = await bounded_json(
                    client,
                    "POST",
                    "https://openrouter.ai/api/v1/chat/completions",
                    timeout_s=timeout_s,
                    body={
                        "model": judge_model,
                        "temperature": temperature,
                        "max_tokens": max_tokens,
                        "stream": False,
                        "response_format": {"type": "json_object"},
                        "messages": [
                            {"role": "system", "content": prompt},
                            {"role": "user", "content": canonical(payload)},
                        ],
                    },
                )
                if status != 200:
                    cost = None
                    continue
                _, computed, billed = usage_and_cost(result, pricing)
                current_cost = billed if billed is not None else computed
                cost = (
                    cost + current_cost
                    if cost is not None and current_cost is not None
                    else None
                )
                choices = result.get("choices")
                if not isinstance(choices, list) or len(choices) != 1:
                    raise DataError("invalid_judge_response")
                text = choices[0]["message"]["content"]
                if not isinstance(text, str):
                    raise DataError("invalid_judge_response")
                return _strict_verdict(text, absolute), cost
            except (DataError, KeyError, TypeError, TimeoutError, httpx.HTTPError):
                continue
        return None, cost

    try:
        with journal(out) as handle:
            existing = existing_rows(out, "judgment", response_key)
            for rid, response_list in grouped.items():
                request = reqs[rid]
                anchor_response = next(
                    (
                        r
                        for r in response_list
                        if (r["target"]["provider"], r["target"]["model"]) == anchor
                    ),
                    None,
                )
                for response in response_list:
                    key = response_key(response)
                    digest = cache_identity(
                        request,
                        response,
                        anchor_response,
                        judge_model,
                        settings,
                        sandbox,
                    )
                    old = existing.get(key)
                    if old:
                        if old.get("cache_key") != digest:
                            raise DataError("resume_judgment_input_changed")
                        stats["skipped"] += 1
                        continue
                    hit = connection.execute(
                        "SELECT verdict FROM judgments WHERE cache_key = ?", (digest,)
                    ).fetchone()
                    if hit:
                        record = validate_record(strict_json(hit[0]), "judgment")
                        if (
                            record.get("cache_key") != digest
                            or response_key(record) != key
                        ):
                            raise DataError("invalid_judgment_cache")
                        stats["cached"] += 1
                    else:
                        record = {
                            "schema_version": "lrp.judgment.v1",
                            "request_id": rid,
                            "target": response["target"],
                            "source": request["source"],
                            "method": "upstream_status:v1",
                            "quality": None,
                            "detail": {},
                            "judge_model": judge_model,
                            "judge_cost_usd": 0.0,
                            "judged_at": utc_now(),
                            "cache_key": digest,
                        }
                        verifier = request.get("verifier", {"kind": "none"})
                        if response["status"] != "ok":
                            record["detail"] = {
                                "error_class": "candidate_unavailable",
                                "unsupported": response["status"] == "ineligible",
                            }
                            record["quality"] = (
                                None if response["status"] == "ineligible" else 0.0
                            )
                        elif verifier.get("kind", "none") != "none":
                            record["method"] = "verifier:" + verifier["kind"] + ":v1"
                            if sandbox is None:
                                record["detail"] = {
                                    "unsupported": True,
                                    "error_class": "sandbox_unavailable",
                                }
                            else:
                                (
                                    record["quality"],
                                    record["detail"],
                                ) = await asyncio.to_thread(
                                    sandbox.verify,
                                    verifier,
                                    response.get("content", ""),
                                )
                        elif (
                            anchor_response is not None
                            and anchor_response["status"] == "ok"
                            and response_key(anchor_response) == key
                        ):
                            record.update(
                                method="pairwise_vs_anchor:v1",
                                quality=1.0,
                                detail={"anchor_identity": True},
                            )
                        else:
                            task = {
                                key: request[key]
                                for key in (
                                    "system",
                                    "input",
                                    "messages",
                                    "tools",
                                    "response_format",
                                )
                                if key in request
                            }
                            answer = {
                                key: response[key]
                                for key in ("content", "tool_calls")
                                if key in response
                            }
                            absolute = (
                                anchor_response is None
                                or anchor_response["status"] != "ok"
                            )
                            if absolute:
                                record["method"] = "absolute_rubric:v1"
                                verdict, cost = await vote(
                                    {"task": task, "answer": answer}, True
                                )
                                record["judge_cost_usd"] = cost
                                if verdict is None:
                                    record["detail"] = {"parse_failed": True}
                                else:
                                    record["quality"] = verdict["score"] / 10
                            else:
                                record["method"] = "pairwise_vs_anchor:v1"
                                assert anchor_response is not None
                                reference = {
                                    key: anchor_response[key]
                                    for key in ("content", "tool_calls")
                                    if key in anchor_response
                                }
                                first, first_cost = await vote(
                                    {"task": task, "A": answer, "B": reference}, False
                                )
                                second, second_cost = await vote(
                                    {"task": task, "A": reference, "B": answer}, False
                                )
                                record["judge_cost_usd"] = (
                                    first_cost + second_cost
                                    if first_cost is not None
                                    and second_cost is not None
                                    else None
                                )
                                if first is None or second is None:
                                    record["detail"] = {"parse_failed": True}
                                else:
                                    first_good = first["winner"] in ("A", "TIE")
                                    second_good = second["winner"] in ("B", "TIE")
                                    record["quality"] = (
                                        int(first_good) + int(second_good)
                                    ) / 2
                                    record["detail"] = {
                                        "votes": [first["winner"], second["winner"]],
                                        "swapped_agree": first_good == second_good,
                                    }
                        record = validate_record(record, "judgment")
                        # Unsupported infrastructure can change; never cache it as evidence.
                        if not record.get("detail", {}).get("unsupported"):
                            connection.execute(
                                "INSERT OR REPLACE INTO judgments VALUES (?, ?)",
                                (digest, canonical(record)),
                            )
                            connection.commit()
                    append_row(handle, record)
                    stats["written"] += 1
                    if record.get("quality") is None:
                        stats["uncertain"] += 1
        return stats
    finally:
        connection.close()
        if owned_client and client is not None:
            await client.aclose()
