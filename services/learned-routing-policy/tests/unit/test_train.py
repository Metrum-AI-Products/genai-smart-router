# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import json

import numpy as np
import pandas as pd
import pytest
from lrp.features import FEATURE_NAMES, SyntheticEmbedder, session_split
from lrp.train import bradley_terry, feature_frame, train


@pytest.fixture(scope="module")
def dataset():
    rows, judgments, responses = [], [], []
    for i in range(1000):
        x = (i % 20) / 20
        values = np.zeros(len(FEATURE_NAMES), dtype=np.float32)
        values[0], values[FEATURE_NAMES.index("estimatedTokens")] = x, 100
        row = dict(zip(FEATURE_NAMES, values))
        row.update(
            request_id=f"synthetic-{i:04d}",
            session_key=f"session-{i // 2}",
            split=session_split(f"session-{i // 2}"),
            group="demo",
            source="synthetic",
            synthetic=True,
            embedding_kind="synthetic",
            embedding_fingerprint=SyntheticEmbedder.fingerprint,
            language="en",
        )
        rows.append(row)
        for name, quality, price in [
            ("cheap/model", float(x < 0.6), 1),
            ("anchor/model", 1.0, 10),
        ]:
            target = {"provider": "synthetic", "model": name}
            judgments.append(
                {
                    "request_id": row["request_id"],
                    "target": target,
                    "source": "synthetic",
                    "judged_at": "2026-09-09T00:00:00Z",
                    "method": "pairwise_vs_anchor:v1",
                    "quality": quality,
                    "detail": {},
                }
            )
            responses.append(
                {
                    "request_id": row["request_id"],
                    "target": target,
                    "source": "synthetic",
                    "started_at": "2026-09-09T00:00:00Z",
                    "status": "ok",
                    "usage": {"input_tokens": 100, "output_tokens": 20 + i % 20},
                    "pricing": {
                        "input_per_m_usd": price,
                        "output_per_m_usd": price,
                        "source": "https://example.test/pricing",
                        "fetched_at": "2026-09-09T00:00:00Z",
                    },
                    "cost_usd": price * (120 + i % 20) / 1e6,
                    "duration_ms": 50,
                    "ttfb_ms": 5,
                }
            )
    return pd.DataFrame(rows), judgments, responses


@pytest.fixture(scope="module")
def trained(tmp_path_factory, dataset):
    frame, judgments, responses = dataset
    return train(
        frame,
        judgments,
        responses,
        tmp_path_factory.mktemp("bundles"),
        embedding={"kind": "synthetic"},
        anchor=("synthetic", "anchor/model"),
        num_boost_round=35,
    )


def test_real_training_reproducible_and_calibrated(dataset, trained, tmp_path):
    frame, judgments, responses = dataset
    other = train(
        frame.sample(frac=1, random_state=1),
        list(reversed(judgments)),
        list(reversed(responses)),
        tmp_path,
        embedding={"kind": "synthetic"},
        anchor=("synthetic", "anchor/model"),
        num_boost_round=35,
    )
    assert other.name == trained.name
    manifest = json.loads((trained / "manifest.json").read_text())
    for entry in manifest["targets"]:
        assert entry["calibration_brier"] <= entry["calibration_brier_raw"] + 1e-12
        for key in ("quality_file", "out_tokens_file", "calibration_file"):
            assert (trained / entry[key]).read_bytes() == (
                other / entry[key]
            ).read_bytes()
    assert frame.groupby("session_key").split.nunique().max() == 1


def test_undertrained_and_parse_uncertainty(dataset, tmp_path):
    frame, judgments, responses = dataset
    subset = [
        dict(j, detail={"parse_failed": True})
        if j["target"]["model"] == "cheap/model"
        else j
        for j in judgments
    ]
    path = train(
        frame,
        subset,
        responses,
        tmp_path,
        embedding={"kind": "synthetic"},
        num_boost_round=2,
    )
    manifest = json.loads((path / "manifest.json").read_text())
    assert manifest["skipped"][0]["model"] == "cheap/model"
    assert manifest["uncertain_judgments"] == 1000
    changed = frame.copy()
    changed.loc[0, "split"] = "test" if changed.loc[0, "split"] != "test" else "train"
    with pytest.raises(ValueError, match="split"):
        feature_frame(changed)
    changed = frame.copy()
    changed["embedding_fingerprint"] = "wrong-artifact"
    with pytest.raises(ValueError, match="artifact"):
        train(changed, judgments, responses, tmp_path, embedding={"kind": "synthetic"})


def test_bt_direction_and_no_verifier_inference():
    anchor = ("synthetic", "anchor")
    rows = [
        {
            "target": {"provider": "synthetic", "model": name},
            "quality": q,
            "method": "pairwise_vs_anchor",
            "detail": {"anchor": {"provider": anchor[0], "model": anchor[1]}},
        }
        for name, q in [("winner", 1), ("loser", 0), ("anchor", 1)]
        for _ in range(50)
    ]
    strengths = bradley_terry(rows, anchor)
    assert strengths[("synthetic", "winner")] > 0.9
    assert strengths[("synthetic", "loser")] < 0.1
    assert strengths[anchor] == 1
