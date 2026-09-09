#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Exercise the real offline pipeline with explicit synthetic embeddings/upstreams/judge."""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import subprocess
import sys
import time
from pathlib import Path

import httpx
from lrp.bundle import load_bundle
from lrp.collect import collect
from lrp.eval import evaluate
from lrp.fanout import run_fanout
from lrp.features import FeatureBuilder, SyntheticEmbedder, featurize
from lrp.judge import judge_requests
from lrp.train import train


class SyntheticVerifier:
    """Injected deterministic test double; never used for operator verifier code."""

    rootfs = Path("/synthetic/test-rootfs")
    timeout_s = 30

    def verify(self, verifier, content):
        return float(content == verifier["spec"]["expected"]), {"passed": content == verifier["spec"]["expected"]}


def mock(request: httpx.Request) -> httpx.Response:
    if request.method == "GET":
        return httpx.Response(
            200,
            json={
                "data": [
                    {
                        "id": model,
                        "pricing": {
                            "prompt": str(price / 1e6),
                            "completion": str(price / 1e6),
                        },
                    }
                    for model, price in [
                        ("cheap", 0.1),
                        ("strong", 1),
                        ("synthetic-judge", 1),
                    ]
                ]
            },
        )
    payload = json.loads(request.content)
    text = "".join(message.get("content", "") for message in payload["messages"])
    hard = len(text) > 8000
    value = "PASS" if payload["model"] == "strong" or not hard else "FAIL"
    return httpx.Response(
        200,
        headers={"x-request-id": "synthetic-router-request"},
        json={
            "model": payload["model"],
            "provider": "synthetic",
            "choices": [
                {
                    "message": {"role": "assistant", "content": value},
                    "finish_reason": "stop",
                }
            ],
            "usage": {
                "prompt_tokens": len(text.encode()) // 4 + 1,
                "completion_tokens": 100 if hard else 10,
            },
        },
    )


async def run(out: Path, count: int) -> dict:
    os.umask(0o077)
    out.mkdir(parents=True, exist_ok=True, mode=0o700)
    seed = out / "seed.ndjson"
    rows = []
    for i in range(count):
        hard = i % 2 == 1
        prompt = (
            "Review this multi-file code and identify the invariant. "
            + (
                "module synthetic: function preserves counter invariant. "
                * (160 + i % 40)
            )
            if hard
            else f"Calculate {i} + 1. Return the integer only."
        )
        rows.append(
            {
                "schema_version": "lrp.request.v1",
                "request_id": f"synthetic-{i}",
                "captured_at": "2026-09-09T00:00:00Z",
                "source": "synthetic",
                "group": "lrp-demo",
                "dialect": "openai-chat",
                "caller": {"project": "synthetic", "environment": "test"},
                "session_key": f"synthetic-session-{i}",
                "turn_index": 0,
                "messages": [{"role": "user", "content": prompt}],
                "max_tokens": 128,
                "context": {
                    "estimatedTokens": len(prompt.encode()) // 4 + 1,
                    "textChars": len(prompt.encode()),
                    "messageCount": 1,
                    "maxTokens": 128,
                },
                "verifier": {"kind": "exact", "spec": {"expected": "PASS"}},
            }
        )
    serialized = "".join(json.dumps(row) + "\n" for row in rows)
    if seed.exists() and seed.read_text() != serialized:
        raise ValueError("synthetic input changed; use a new output directory")
    seed.write_text(serialized)
    seed.chmod(0o600)
    requests, responses, judgments = (
        out / name
        for name in ("requests.ndjson", "responses.ndjson", "judgments.ndjson")
    )
    collect(dataset=seed, out=requests)
    targets = [
        {
            "provider": "synthetic",
            "model": name,
            "model_ref": "",
            "router_group": name,
            "router_targets": [
                {"provider": "synthetic", "model": name, "model_ref": ""}
            ],
        }
        for name in ("cheap", "strong")
    ]
    async with httpx.AsyncClient(transport=httpx.MockTransport(mock)) as client:
        fanout = await run_fanout(
            requests=requests,
            out=responses,
            targets=targets,
            via="router",
            base_url="http://127.0.0.1:8080/v1",
            client=client,
            seed=42,
        )
        judged = await judge_requests(
            requests=requests,
            responses=responses,
            out=judgments,
            anchor=("synthetic", "strong"),
            judge_model="synthetic-judge",
            client=client,
            sandbox=SyntheticVerifier(),
        )
    features = out / "features.parquet"
    featurize(requests, features, builder=FeatureBuilder(SyntheticEmbedder()))
    training_started = time.monotonic()
    bundle = train(
        features,
        judgments,
        responses,
        out / "bundles",
        embedding={"kind": "synthetic"},
        anchor=("synthetic", "strong"),
        seed=42,
        threads=1,
    )
    training_seconds = time.monotonic() - training_started
    loaded = load_bundle(bundle, threads=1)
    report = evaluate(
        loaded,
        features,
        judgments,
        responses,
        {"groups": {"lrp-demo": {"quality_floor": 0.8}}},
        out=out / "eval_report.json",
    )
    evidence = {
        "schema_version": "lrp.synthetic-demo.v1",
        "source": "synthetic",
        "promotable": False,
        "requests": count,
        "fanout": fanout,
        "judgments": judged,
        "bundle": str(bundle),
        "report": str(out / "eval_report.json"),
        "gates_evaluated": bool(report.get("groups")),
        "real_onnx_latency": "not_run",
        "provider_backed_quality": "not_run",
    }
    (out / "evidence.json").write_text(json.dumps(evidence, indent=2) + "\n")
    # This explicit synthetic-only projection is the public CI artifact. Never
    # upload evidence.json, manifests, features, datasets or arbitrary logs.
    manifest = loaded.manifest
    group = report["groups"]["lrp-demo"]
    training = {
        "schema_version": "lrp.public-training.v1",
        "source": "synthetic",
        "promotable": False,
        "embedding_kind": "synthetic",
        "seed": 42,
        "threads": 1,
        "training_seconds": training_seconds,
        "requests": count,
        "split_counts": {key: manifest["split_counts"][key] for key in ("train", "valid", "test")},
        "targets": [
            {key: target[key] for key in (
                "provider", "model", "n_train", "n_valid",
                "calibration_brier", "calibration_brier_raw", "mean_out_tokens",
            )}
            for target in manifest["targets"]
        ],
        "training_versions": {key: manifest["training_versions"][key] for key in ("lightgbm", "numpy")},
        "quality_floor": group["quality_floor"],
        "holdout": {
            name: {key: group["baselines"][name][key] for key in (
                "n", "quality_mean", "cost_total_usd", "floor_violation_rate",
                "quality_observed", "cost_observed", "billed_cost_observed",
            )}
            for name in ("lrp", "always_cheapest", "always_anchor", "weighted_random", "bt_only", "oracle")
        },
        "gates": {key: group["gates"][key] for key in (
            "complete_holdout", "cost_vs_anchor", "dominates_bt", "floor_violations",
            "quality_vs_anchor", "real_data_and_embedding", "target_auc_and_coverage",
        )},
        "provider_backed_quality": "not_run",
        "real_onnx_performance": "not_run",
    }
    (out / "public-training.json").write_text(json.dumps(training, indent=2) + "\n")
    events = [
        {"event": "training_completed", "source": "synthetic", "seconds": training_seconds,
         "seed": 42, "threads": 1, "split_counts": training["split_counts"]},
        *[{"event": "target_calibrated", "source": "synthetic", **target} for target in training["targets"]],
        {"event": "holdout_evaluated", "source": "synthetic", "gates": training["gates"], "promotable": False},
    ]
    (out / "public-training.log").write_text("".join(json.dumps(event) + "\n" for event in events))
    return evidence


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--out-dir", type=Path, default=Path("/var/tmp/metrum-lrp-synthetic-demo")
    )
    parser.add_argument("--requests", type=int, default=800)
    parser.add_argument("--e2e", action="store_true")
    args = parser.parse_args()
    if not 400 <= args.requests <= 2000:
        parser.error("synthetic request count must be 400..2000")
    evidence = asyncio.run(run(args.out_dir.resolve(), args.requests))
    if args.e2e:
        root = Path(__file__).resolve().parents[1]
        subprocess.run([sys.executable, str(root / "scripts/run_lrp_e2e.py"),
                        "--bundle", evidence["bundle"], "--lrp-python", sys.executable,
                        "--service-dir", str(root / "services/learned-routing-policy"),
                        "--explain-evidence",
                        "--output", str(args.out_dir / "public-inference.json")], check=True)
    print(
        json.dumps(
            {
                "source": "synthetic",
                "requests": evidence["requests"],
                "gates_evaluated": evidence["gates_evaluated"],
                "promotable": False,
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
