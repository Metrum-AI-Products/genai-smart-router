#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Synthetic harness for adaptive_signal_policy.py.

This is a mock-router wiring test. It proves the policy can:
  - classify request content
  - prefer observed-good targets over config-cheap targets
  - pin a conversation across turns
  - ignore response-cache hits
  - book fallback outcomes to the serving target

It is not provider-backed quality evidence.
"""
from __future__ import annotations

import json
import sys
import time
import urllib.request

URL = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:18092"

TARGETS = [
    {
        "provider": "baseten",
        "model": "openai/gpt-oss-120b",
        "modelRef": "gpt-oss-120b",
        "dialect": "openai-chat",
        "tier": "cheap",
        "weight": 70,
        "rpm": 9999,
        "inputPricePerMillionUsd": 0.10,
        "outputPricePerMillionUsd": 0.50,
        "keyConfigured": True,
    },
    {
        "provider": "minimax",
        "model": "m3",
        "modelRef": "m3",
        "dialect": "openai-chat",
        "tier": "heavy",
        "weight": 30,
        "rpm": 10,
        "inputPricePerMillionUsd": 0.40,
        "outputPricePerMillionUsd": 1.60,
        "keyConfigured": True,
    },
]
NAME = [f'{t["provider"]}/{t["model"]}' for t in TARGETS]


def post(path, body):
    r = urllib.request.Request(
        URL + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
    )
    return json.loads(urllib.request.urlopen(r, timeout=5).read())


def get(path):
    return json.loads(urllib.request.urlopen(URL + path, timeout=5).read())


def route(messages, token="rtr_team_prod_k1", system=""):
    return post(
        "/route",
        {
            "group": "adaptive",
            "context": {
                "model": "adaptive",
                "dialect": "openai-chat",
                "messageCount": len(messages),
                "stream": False,
                "textChars": sum(len(m.get("content", "")) for m in messages),
            },
            "requirements": ["text"],
            "inputModalities": ["text"],
            "caller": {
                "id": "team-prod",
                "user": "team",
                "project": "product",
                "environment": "prod",
                "tokenId": token,
                "allow": ["adaptive"],
            },
            "targets": TARGETS,
            "now": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "request": {"model": "adaptive", "system": system, "messages": messages},
            "text": "\n".join(m["content"] for m in messages),
        },
    )


def log_record(
    primary_idx,
    serving_idx,
    *,
    status=200,
    ttfb=800,
    tps=40,
    cache="miss",
    quality=None,
    fp=None,
    diagnostics=True,
):
    p = TARGETS[primary_idx]
    s = TARGETS[serving_idx]
    usage = {"input_tokens": 1000, "output_tokens": 500}
    rec = {
        "ts": time.time(),
        "resolved_group": "adaptive",
        "strategy": "external",
        "cache": cache,
        "status": status,
        "target_provider": p["provider"],
        "target_model": p["model"],
        "input_price_per_million_usd": p["inputPricePerMillionUsd"],
        "output_price_per_million_usd": p["outputPricePerMillionUsd"],
        "total_cost_usd": (
            1000 * p["inputPricePerMillionUsd"] + 500 * p["outputPricePerMillionUsd"]
        )
        / 1e6,
        "attempts": 1 if primary_idx == serving_idx else 2,
        "fallback_used": primary_idx != serving_idx,
        "ttfb_ms": ttfb,
        "upstream_output_tokens_per_sec": tps,
        "usage": usage,
        "quality": quality,
        "policy_fingerprint": fp,
        "serving_prices": {
            "in": s["inputPricePerMillionUsd"],
            "out": s["outputPricePerMillionUsd"],
        },
    }
    if diagnostics:
        failed = (
            [
                {
                    "index": 1,
                    "provider": p["provider"],
                    "model": p["model"],
                    "status_code": 503,
                    "error_class": "upstream_unavailable",
                    "selected": False,
                }
            ]
            if primary_idx != serving_idx
            else []
        )
        rec["attempts_detail"] = failed + [
            {
                "index": rec["attempts"],
                "provider": s["provider"],
                "model": s["model"],
                "status_code": status,
                "selected": True,
            }
        ]
    return rec


results = []


def check(name, cond, detail=""):
    results.append((name, cond))
    print(f'{"PASS" if cond else "FAIL"}  {name}  {detail}')


d1 = route(
    [
        {
            "role": "user",
            "content": "Prove step by step why this trade-off holds for a distributed cache.",
        }
    ],
    token="tA",
)
d2 = route(
    [
        {
            "role": "user",
            "content": "SELECT id FROM users WHERE active = 1; fix the join please",
        }
    ],
    token="tB",
)
check(
    "claim3a content classified from full request",
    d1["classLabel"] == "adaptive:reasoning" and d2["classLabel"] == "adaptive:sql",
    f'{d1["classLabel"]} / {d2["classLabel"]}',
)

for _ in range(8):
    post("/observe", log_record(0, 0, ttfb=2400, tps=12, quality=0.55))
    post("/observe", log_record(1, 1, ttfb=300, tps=90, quality=0.95))
d = route([{"role": "user", "content": "hello there, quick chat"}], token="tC")
check(
    "claim1 picks observed-good target over config-cheap target",
    NAME[d["targetIndex"]] == "minimax/m3",
    f'picked {NAME[d["targetIndex"]]}; config would say baseten',
)
ex = get(f'/explain?rid={d["metadata"]["fingerprint"]}')
check(
    "claim1 decision explains itself with observed stats",
    all("stats" in c for c in ex["explain"]["candidates"]),
)

sess = [{"role": "user", "content": "You are helping me refactor auth.go. Start by reading it."}]
first = route(sess, token="tD")
turn2 = route(
    sess + [{"role": "assistant", "content": "ok"}, {"role": "user", "content": "now add tests"}],
    token="tD",
)
turn3 = route(
    sess
    + [
        {"role": "assistant", "content": "ok"},
        {"role": "user", "content": "now add tests"},
        {"role": "assistant", "content": "done"},
        {"role": "user", "content": "SELECT * FROM nowhere -- would reclassify to sql"},
    ],
    token="tD",
)
check(
    "claim2 session pinned across turns (even when class changes)",
    first["targetIndex"] == turn2["targetIndex"] == turn3["targetIndex"]
    and turn2["classLabel"].startswith("adaptive:pin"),
    f'{NAME[first["targetIndex"]]} x3, labels {first["classLabel"]}, {turn2["classLabel"]}, {turn3["classLabel"]}',
)
other = route([{"role": "user", "content": "unrelated new conversation"}], token="tD")
check(
    "claim2 new conversation from same token is a new pin",
    other["classLabel"] == "adaptive:chat",
    other["classLabel"],
)

before = get("/state")["targets"]["minimax/m3"]["n"]
for _ in range(5):
    post("/observe", log_record(1, 1, ttfb=2, tps=0, cache="hit"))
st = get("/state")
check(
    "claim3b cache hits never enter observations",
    st["targets"]["minimax/m3"]["n"] == before and st["dropped_cache_hits"] == 5,
    f'n stayed {before}, dropped={st["dropped_cache_hits"]}',
)

r = post("/observe", log_record(0, 1, ttfb=350, tps=85, quality=0.9))
check(
    "claim4 fallback booked to serving target (diagnostics on)",
    r["booked_to"] == "minimax/m3" and r["reattributed"],
    f'log said {r["primary_in_log"]}, booked to {r["booked_to"]} via {r["source"]}',
)
d = route(
    [{"role": "user", "content": "```python\ndef x(): pass\n``` explain"}],
    token="tE",
)
r2 = post(
    "/observe",
    {
        **log_record(
            d["targetIndex"],
            d["fallbackIndexes"][0],
            ttfb=400,
            tps=80,
            fp=d["metadata"]["fingerprint"],
            diagnostics=False,
        )
    },
)
check(
    "claim4 fallback booked correctly with diagnostics OFF",
    r2["reattributed"] and r2["source"] == "policy_fallback_order",
    f'{r2["source"]} -> {r2["booked_to"]}',
)
router_cost = log_record(0, 1)["total_cost_usd"]
check(
    "claim4 cost repriced at serving target",
    abs(r["cost_usd"] - (1000 * 0.40 + 500 * 1.60) / 1e6) < 1e-9,
    f'router logged ${router_cost:.6f}, engine booked ${r["cost_usd"]:.6f}',
)

for _ in range(12):
    post("/observe", log_record(1, 1, status=503, ttfb=0, tps=0))
d = route([{"role": "user", "content": "another quick chat"}], token="tF")
check(
    "claim4b reliability is a floor, not the objective",
    NAME[d["targetIndex"]] == "baseten/openai/gpt-oss-120b",
    f'minimax over error floor -> {NAME[d["targetIndex"]]}',
)

print()
print(f"{sum(c for _, c in results)}/{len(results)} passed")
sys.exit(0 if all(c for _, c in results) else 1)
