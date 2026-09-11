#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Deployment-owned adaptive external routing policy for Metrum AI Router.

This is a reference policy service, not built-in router state. Wire it with
`strategy: external` and, when content classification is required,
`include_request: true` on a trusted policy boundary.

Endpoints:
  POST /route     router -> policy     pick a target
  POST /observe   log tailer/harness   book outcomes to the SERVING target
  GET  /state     operator             pins, observations, dropped cache hits
  GET  /explain   operator             why a request was routed

Demonstrates:
  1. Decisions from observed latency/throughput/reliability + request class,
     not from configured weight/RPM/cost integers.
  2. Short-lived conversation pins so a multi-turn session can stay on one
     provider long enough to preserve provider prompt-cache prefixes.
  3. Full request inspection when include_request is on; cache hits never
     enter observation statistics.
  4. Fallback attribution to attempts_detail[].selected (or the policy's own
     fallback order) so learning books the target that actually answered.

This example is synthetic-wiring evidence when run with the harness. It is not
provider-backed quality evidence and is not a shipped online-learning control
plane.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import statistics
import threading
import time
from collections import defaultdict, deque
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse
import re

# ---------------------------------------------------------------- config

PROMPT_CACHE_TTL_S = {
    "anthropic": 300,
    "openai": 300,
    "baseten": 120,
    "minimax": 120,
    "vllm": 600,
}
DEFAULT_PIN_TTL_S = 180
OBS_WINDOW_S = 900
MIN_OBS = 5
ERROR_FLOOR = 0.25
CLASS_PATTERNS = {
    "sql": re.compile(r"\b(select|insert|update|delete|create table|join)\b", re.I),
    "code": re.compile(r"```|\bdef |\bfunc |\bclass |\bimport |#include|=>", re.I),
    "reasoning": re.compile(r"\b(prove|derive|step by step|trade-?offs?|why does)\b", re.I),
    "extract": re.compile(r"\b(extract|json|schema|fields?|parse)\b", re.I),
}
CLASS_TIER = {
    "reasoning": "heavy",
    "code": "heavy",
    "sql": "cheap",
    "extract": "cheap",
    "chat": "cheap",
}


class Obs:
    __slots__ = ("ts", "ttfb_ms", "tps", "ok", "cost_usd", "quality")

    def __init__(self, ts, ttfb_ms, tps, ok, cost_usd, quality):
        self.ts = ts
        self.ttfb_ms = ttfb_ms
        self.tps = tps
        self.ok = ok
        self.cost_usd = cost_usd
        self.quality = quality


class State:
    def __init__(self):
        self.lock = threading.RLock()
        self.obs: dict[str, deque[Obs]] = defaultdict(lambda: deque(maxlen=2000))
        self.pins: dict[str, dict] = {}
        self.decisions: dict[str, dict] = {}
        self.dropped_cache_hits = 0
        self.reattributed = 0

    def stats(self, key: str, now: float) -> dict:
        recs = [o for o in self.obs[key] if now - o.ts <= OBS_WINDOW_S]
        if not recs:
            return {"n": 0}
        ttfb = sorted(o.ttfb_ms for o in recs if o.ttfb_ms > 0)
        tps = [o.tps for o in recs if o.tps > 0]
        q = [o.quality for o in recs if o.quality is not None]
        return {
            "n": len(recs),
            "p95_ttfb_ms": ttfb[int(0.95 * (len(ttfb) - 1))] if ttfb else None,
            "tps": statistics.fmean(tps) if tps else None,
            "error_rate": 1 - sum(o.ok for o in recs) / len(recs),
            "cost_usd_avg": statistics.fmean(o.cost_usd for o in recs),
            "quality": statistics.fmean(q) if q else None,
        }


S = State()


def tkey(t: dict) -> str:
    return f'{t["provider"]}/{t["model"]}'


def fingerprint(payload: dict) -> str:
    """Stable conversation key: caller token + hash of system + first user turn."""
    req = payload.get("request") or {}
    msgs = req.get("messages") or []
    first_user = next((m.get("content", "") for m in msgs if m.get("role") == "user"), "")
    if isinstance(first_user, list):
        first_user = " ".join(
            str(part.get("text") or "") for part in first_user if isinstance(part, dict)
        )
    h = hashlib.sha256()
    h.update((req.get("system") or "").encode())
    h.update(str(first_user)[:2000].encode())
    return f'{payload.get("caller", {}).get("tokenId", "anon")}:{h.hexdigest()[:16]}'


def classify(payload: dict) -> tuple[str, dict]:
    req = payload.get("request") or {}
    text = payload.get("text") or ""
    for m in req.get("messages") or []:
        content = m.get("content") or ""
        if isinstance(content, list):
            text += "\n" + " ".join(
                str(part.get("text") or "") for part in content if isinstance(part, dict)
            )
        else:
            text += "\n" + str(content)
        for p in m.get("parts") or []:
            text += "\n" + (p.get("text") or "")
    text = (req.get("system") or "") + "\n" + text
    # Prefer scalar context when include_request is off.
    ctx = payload.get("context") or {}
    if not text.strip() and ctx.get("textChars"):
        return "chat", {"chars": int(ctx.get("textChars") or 0), "tools": int(ctx.get("toolCount") or 0), "hits": {}}
    hits = {k: len(rx.findall(text)) for k, rx in CLASS_PATTERNS.items()}
    hits = {k: v for k, v in hits.items() if v}
    label = max(hits, key=hits.get) if hits else "chat"
    return label, {"chars": len(text), "tools": len(req.get("tools") or []), "hits": hits}


def norm_inverse(x, xs):
    xs = [v for v in xs if v is not None]
    if x is None or not xs or max(xs) == min(xs):
        return 0.5
    return 1 - (x - min(xs)) / (max(xs) - min(xs))


def norm(x, xs):
    return 1 - norm_inverse(x, xs) if x is not None else 0.5


def decide(payload: dict) -> dict:
    now = time.time()
    targets = payload["targets"]
    eligible = [i for i, t in enumerate(targets) if t.get("keyConfigured")]
    if not eligible:
        return {"targetIndex": 0, "classLabel": "adaptive:no-candidates"}

    label, features = classify(payload)
    want_tier = CLASS_TIER.get(label, "cheap")
    fp = fingerprint(payload)
    explain = {"class": label, "features": features, "fingerprint": fp, "candidates": []}

    with S.lock:
        pin = S.pins.get(fp)
        if pin and pin["until"] > now:
            idx = next((i for i in eligible if tkey(targets[i]) == pin["target"]), None)
            st = S.stats(pin["target"], now) if idx is not None else {"n": 0}
            if idx is not None and (st["n"] < MIN_OBS or st["error_rate"] < ERROR_FLOOR):
                pin["hits"] += 1
                explain["decision"] = "pinned"
                return finish(payload, idx, eligible, f"adaptive:pin:{label}", explain, fp)
            explain["pin_evicted"] = {"target": pin["target"], "stats": st}
            S.pins.pop(fp, None)

        stats = {i: S.stats(tkey(targets[i]), now) for i in eligible}
        ttfbs = [stats[i].get("p95_ttfb_ms") for i in eligible]
        tpss = [stats[i].get("tps") for i in eligible]
        costs = [stats[i].get("cost_usd_avg") for i in eligible if stats[i]["n"]]
        scored = []
        for i in eligible:
            st, t = stats[i], targets[i]
            if st["n"] >= MIN_OBS and st["error_rate"] >= ERROR_FLOOR:
                explain["candidates"].append({"target": tkey(t), "excluded": "error_floor", "stats": st})
                continue
            observed_cost = st.get("cost_usd_avg") if st["n"] else None
            score = (
                0.35 * (st.get("quality") if st.get("quality") is not None else 0.5)
                + 0.25 * norm_inverse(observed_cost, costs)
                + 0.20 * norm_inverse(st.get("p95_ttfb_ms"), ttfbs)
                + 0.10 * norm(st.get("tps"), tpss)
                + 0.10 * (1.0 if t.get("tier") == want_tier else 0.0)
            )
            if st["n"] < MIN_OBS:
                score += 0.05
            scored.append((score, i))
            explain["candidates"].append({"target": tkey(t), "score": round(score, 4), "stats": st})
        if not scored:
            scored = [(0.0, i) for i in eligible]
            scored.sort(key=lambda p: stats[p[1]]["error_rate"])
        else:
            scored.sort(reverse=True)
        best = scored[0][1]
        ttl = PROMPT_CACHE_TTL_S.get(targets[best]["provider"], DEFAULT_PIN_TTL_S)
        S.pins[fp] = {"target": tkey(targets[best]), "until": now + ttl, "hits": 0, "class": label}
        explain["decision"] = "scored"
        explain["pin_ttl_s"] = ttl
        return finish(payload, best, eligible, f"adaptive:{label}", explain, fp)


def finish(payload, idx, eligible, label, explain, fp):
    fallbacks = [i for i in eligible if i != idx]
    dec = {
        "targetIndex": idx,
        "fallbackIndexes": fallbacks,
        "classLabel": label[:64],
        "metadata": {"fingerprint": fp, "decision": explain.get("decision")},
    }
    order = [tkey(payload["targets"][i]) for i in [idx] + fallbacks]
    rid = ((payload.get("request") or {}).get("raw") or {}).get("metadata", {}).get("request_id")
    with S.lock:
        S.decisions[rid or fp] = {"ts": time.time(), "order": order, "explain": explain}
    return dec


def observe(rec: dict) -> dict:
    if rec.get("cache") == "hit":
        with S.lock:
            S.dropped_cache_hits += 1
        return {"ignored": "cache_hit"}

    serving, source = None, "primary"
    for a in rec.get("attempts_detail") or []:
        if a.get("selected"):
            serving, source = f'{a["provider"]}/{a["model"]}', "attempts_detail.selected"
    if serving is None and rec.get("fallback_used"):
        fp = rec.get("policy_fingerprint") or rec.get("request_id")
        with S.lock:
            d = S.decisions.get(fp)
        if d and len(d["order"]) >= max(2, rec.get("attempts", 2)):
            serving, source = d["order"][rec.get("attempts", 2) - 1], "policy_fallback_order"
    if serving is None:
        serving = f'{rec.get("target_provider")}/{rec.get("target_model")}'
    primary = f'{rec.get("target_provider")}/{rec.get("target_model")}'
    reattributed = serving != primary

    usage = rec.get("usage") or {}
    cost = rec.get("total_cost_usd") or 0.0
    price = rec.get("serving_prices")
    if reattributed and price:
        cost = (
            usage.get("input_tokens", 0) * price["in"]
            + usage.get("output_tokens", 0) * price["out"]
        ) / 1e6
    ok = 200 <= int(rec.get("status") or 0) < 400
    obs = Obs(
        time.time(),
        rec.get("ttfb_ms") or 0,
        rec.get("upstream_output_tokens_per_sec") or 0,
        ok,
        cost,
        rec.get("quality"),
    )
    with S.lock:
        S.obs[serving].append(obs)
        if reattributed:
            S.reattributed += 1
    return {
        "booked_to": serving,
        "source": source,
        "reattributed": reattributed,
        "primary_in_log": primary,
        "cost_usd": round(cost, 6),
    }


class H(BaseHTTPRequestHandler):
    server_version = "adaptive-signal-policy/1.0"

    def log_message(self, *_):
        pass

    def _json(self, code, body):
        raw = json.dumps(body, default=str).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _body(self):
        n = int(self.headers.get("content-length", "0"))
        return json.loads(self.rfile.read(n) or b"{}")

    def do_POST(self):
        p = urlparse(self.path).path
        if p == "/route":
            return self._json(200, decide(self._body()))
        if p == "/observe":
            return self._json(200, observe(self._body()))
        self._json(404, {"error": "not found"})

    def do_GET(self):
        u = urlparse(self.path)
        now = time.time()
        if u.path == "/state":
            with S.lock:
                return self._json(
                    200,
                    {
                        "targets": {k: S.stats(k, now) for k in S.obs},
                        "pins": {
                            k: {**v, "ttl_left_s": round(v["until"] - now)}
                            for k, v in S.pins.items()
                        },
                        "dropped_cache_hits": S.dropped_cache_hits,
                        "reattributed_fallbacks": S.reattributed,
                    },
                )
        if u.path == "/explain":
            rid = parse_qs(u.query).get("rid", [""])[0]
            with S.lock:
                return self._json(200, S.decisions.get(rid) or {"error": "unknown request"})
        self._json(404, {"error": "not found"})


def tail_log(path: str, url: str):
    import urllib.request

    with open(path, "r", encoding="utf-8") as f:
        f.seek(0, 2)
        while True:
            line = f.readline()
            if not line:
                time.sleep(0.25)
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            req = urllib.request.Request(
                url + "/observe",
                data=json.dumps(rec).encode(),
                headers={"Content-Type": "application/json"},
            )
            try:
                urllib.request.urlopen(req, timeout=2).read()
            except Exception:
                pass


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=18092)
    ap.add_argument("--tail-log", help="router JSONL request log to follow for /observe feedback")
    a = ap.parse_args()
    if a.tail_log:
        threading.Thread(
            target=tail_log,
            args=(a.tail_log, f"http://{a.host}:{a.port}"),
            daemon=True,
        ).start()
    print(
        f"adaptive signal policy on http://{a.host}:{a.port}  (/route /observe /state /explain)",
        flush=True,
    )
    ThreadingHTTPServer((a.host, a.port), H).serve_forever()


if __name__ == "__main__":
    main()
