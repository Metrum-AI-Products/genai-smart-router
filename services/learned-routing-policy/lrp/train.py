# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Deterministic per-target supervised training; test rows are never fitted."""

from __future__ import annotations

import math
import shutil
import tempfile
from pathlib import Path
from typing import Any

import numpy as np
import pandas as pd

from lrp.bundle import (
    SCHEMA_VERSION,
    bundle_version,
    canonical_json,
    sha256_file,
    target_id,
    target_key,
)
from lrp.features import (
    FEATURE_NAMES,
    capped_threads,
    embedding_fingerprint,
    read_rows,
    session_split,
)
from lrp.schemas import TargetKey


def feature_frame(source: Any, seed: int = 42) -> pd.DataFrame:
    frame = (
        pd.read_parquet(source) if isinstance(source, (str, Path)) else source.copy()
    )
    required = {
        *FEATURE_NAMES,
        "request_id",
        "session_key",
        "split",
        "group",
        "source",
        "synthetic",
        "embedding_kind",
        "embedding_fingerprint",
    }
    if not required.issubset(frame.columns) or frame.empty:
        raise ValueError("incomplete feature dataset")
    if frame.request_id.duplicated().any() or frame.session_key.isna().any():
        raise ValueError("duplicate request or missing session")
    expected = frame.session_key.map(lambda value: session_split(str(value), seed))
    if not (expected == frame.split).all():
        raise ValueError("split does not match deterministic session split")
    if not np.isfinite(
        frame.loc[:, list(FEATURE_NAMES)].to_numpy(dtype=np.float32)
    ).all():
        raise ValueError("nonfinite features")
    return frame.sort_values("request_id").reset_index(drop=True)


def trusted_judgment(row: dict[str, Any]) -> bool:
    quality = row.get("quality")
    detail = row.get("detail") or {}
    return (
        quality is not None
        and isinstance(quality, (float, int))
        and math.isfinite(quality)
        and 0 <= quality <= 1
        and not detail.get("parse_failed")
        and not detail.get("uncertain")
        and not detail.get("unsupported")
        and not detail.get("infrastructure_error")
    )


def index_rows(source: Any) -> dict[tuple[str, TargetKey], dict[str, Any]]:
    result = {}
    for row in read_rows(source):
        key = (str(row["request_id"]), target_key(row["target"]))
        if key in result:
            raise ValueError("duplicate request-target row")
        result[key] = row
    return result


def bradley_terry(
    judgments: list[dict[str, Any]],
    anchor: TargetKey | None = None,
    iterations: int = 200,
) -> dict[TargetKey, float]:
    """Fit regularized anchor-relative BT using fractional pairwise wins.

    Only explicit pairwise labels contribute. Verifier/rubric cohorts have no
    pairwise meaning and must not masquerade as BT observations.
    """
    keys = sorted({target_key(row["target"]) for row in judgments})
    if not keys:
        return {}
    positions = {key: i for i, key in enumerate(keys)}
    wins = np.zeros((len(keys), len(keys)), dtype=float)
    for row in judgments:
        if not trusted_judgment(row) or not str(row.get("method", "")).startswith(
            "pairwise"
        ):
            continue
        candidate = target_key(row["target"])
        other = (row.get("detail") or {}).get("anchor")
        reference = target_key(other) if isinstance(other, dict) else anchor
        if reference not in positions or reference == candidate:
            continue
        i, j = positions[candidate], positions[reference]
        wins[i, j] += float(row["quality"])
        wins[j, i] += 1 - float(row["quality"])
    strength = np.ones(len(keys))
    for _ in range(min(max(iterations, 1), 1000)):
        # Symmetric pseudo-observations prevent infinite strengths/separation.
        regularized = wins + 0.5 * (1 - np.eye(len(keys)))
        totals = regularized + regularized.T
        denominator = (totals / (strength[:, None] + strength[None, :])).sum(axis=1)
        updated = regularized.sum(axis=1) / np.maximum(denominator, 1e-12)
        updated = np.maximum(updated, 1e-12)
        updated /= np.exp(np.log(updated).mean())
        if np.max(np.abs(np.log(updated / strength))) < 1e-10:
            strength = updated
            break
        strength = updated
    reference_strength = strength[positions[anchor]] if anchor in positions else 1.0
    return {
        key: (
            1.0
            if key == anchor
            else float(strength[i] / (strength[i] + reference_strength))
        )
        for key, i in positions.items()
    }


def _binary_data(x: Any, y: Any) -> tuple[Any, Any, Any]:
    # Fractional labels are weighted copies of both classes, including ties.
    labels = np.asarray(y, dtype=float)
    matrix = np.asarray(x, dtype=np.float32)
    return (
        np.concatenate((matrix, matrix)),
        np.concatenate((np.ones(len(labels)), np.zeros(len(labels)))),
        np.concatenate((labels, 1 - labels)),
    )


def train(
    features: Any,
    judgments: Any,
    responses: Any,
    out: str | Path,
    *,
    embedding: dict[str, Any],
    seed: int = 42,
    threads: int = 1,
    min_train_rows: int = 200,
    num_boost_round: int = 400,
    anchor: TargetKey | None = None,
) -> Path:
    import lightgbm as lgb
    from sklearn.isotonic import IsotonicRegression

    capped_threads(threads)
    if min_train_rows < 200 or not 1 <= num_boost_round <= 400:
        raise ValueError("invalid training resource or minimum-row limits")
    frame = feature_frame(features, seed)
    judgment_index, response_index = index_rows(judgments), index_rows(responses)
    feature_ids = set(frame.request_id)
    if any(key[0] not in feature_ids for key in judgment_index | response_index):
        raise ValueError("outcome has no feature row")
    kind = embedding.get("kind")
    if kind not in {"synthetic", "onnx"} or set(frame.embedding_kind) != {kind}:
        raise ValueError("embedding provenance mismatch")
    fingerprint = embedding_fingerprint(embedding)
    if set(frame.embedding_fingerprint) != {fingerprint}:
        raise ValueError("embedding artifact differs from featurization")
    synthetic = (
        kind == "synthetic"
        or bool(frame.synthetic.any())
        or any(
            row.get("source") == "synthetic"
            for row in [*judgment_index.values(), *response_index.values()]
        )
    )
    train_ids = set(frame.loc[frame.split == "train", "request_id"])
    train_judgments = [
        row for (rid, _), row in judgment_index.items() if rid in train_ids
    ]
    bt = bradley_terry(train_judgments, anchor)
    manifest: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "feature_names": list(FEATURE_NAMES),
        "seed": seed,
        "threads": threads,
        "min_train_rows": min_train_rows,
        "synthetic": synthetic,
        "embedding": {"kind": kind},
        "embedding_fingerprint": fingerprint,
        "targets": [],
        "skipped": [],
        "files": {},
        "split_counts": {k: int(v) for k, v in frame.split.value_counts().items()},
        "uncertain_judgments": sum(
            not trusted_judgment(r) for r in judgment_index.values()
        ),
        "anchor": {"provider": anchor[0], "model": anchor[1]} if anchor else None,
        "training_versions": {"lightgbm": lgb.__version__, "numpy": np.__version__},
    }
    root = Path(out)
    root.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=".training-", dir=root))
    try:
        if kind == "onnx":
            from lrp.features import ONNXEmbedder

            # Validate operator assets before copying into a published snapshot.
            ONNXEmbedder(
                embedding["model_path"],
                embedding["tokenizer_path"],
                threads,
                embedding.get("pooling", "cls"),
                embedding.get("prefix", ""),
            ).encode("Synthetic validation.")
            (temporary / "embed").mkdir()
            for asset_key, name in [
                ("model_path", "model.onnx"),
                ("tokenizer_path", "tokenizer.json"),
            ]:
                relative = "embed/" + name
                shutil.copyfile(embedding[asset_key], temporary / relative)
                manifest["embedding"][asset_key] = relative
            manifest["embedding"].update(
                pooling=embedding.get("pooling", "cls"),
                prefix=embedding.get("prefix", ""),
            )
        params = {
            "objective": "binary",
            "metric": "binary_logloss",
            "num_leaves": 31,
            "learning_rate": 0.05,
            "seed": seed,
            "num_threads": threads,
            "deterministic": True,
            "force_col_wise": True,
            "verbosity": -1,
            "histogram_pool_size": 64,
        }
        for target in sorted({key[1] for key in judgment_index}):
            records = []
            for (request_id, key), judgment in judgment_index.items():
                response = response_index.get((request_id, key))
                if (
                    key != target
                    or not trusted_judgment(judgment)
                    or response is None
                    or response.get("status") == "ineligible"
                ):
                    continue
                usage = response.get("usage") or {}
                output = usage.get("output_tokens")
                records.append(
                    {
                        "request_id": request_id,
                        "quality": float(judgment["quality"]),
                        "output_tokens": output,
                        "response_ok": response.get("status") == "ok",
                    }
                )
            joined = (
                frame.merge(
                    pd.DataFrame(records), on="request_id", validate="one_to_one"
                )
                if records
                else frame.iloc[:0].copy()
            )
            training = joined.loc[joined.split == "train"]
            validation = joined.loc[joined.split == "valid"]
            output_training = (
                training.loc[training.response_ok & training.output_tokens.notna()]
                if records
                else training
            )
            output_validation = (
                validation.loc[
                    validation.response_ok & validation.output_tokens.notna()
                ]
                if records
                else validation
            )
            entry: dict[str, Any] = {
                "provider": target[0],
                "model": target[1],
                "id": target_id(target),
                "n_train": len(training),
                "n_valid": len(validation),
                "bt_strength": bt.get(target, 0.5),
                "mean_out_tokens": float(output_training.output_tokens.mean())
                if len(output_training)
                else 0.0,
            }
            if (
                len(training) < min_train_rows
                or len(output_training) < min_train_rows
                or len(validation) < 2
                or len(output_validation) < 2
            ):
                entry["skipped"] = "insufficient_train_or_validation_rows"
                manifest["skipped"].append(
                    {
                        "provider": target[0],
                        "model": target[1],
                        "reason": entry["skipped"],
                    }
                )
                manifest["targets"].append(entry)
                continue
            x, y, w = _binary_data(training[list(FEATURE_NAMES)], training.quality)
            vx, vy, vw = _binary_data(
                validation[list(FEATURE_NAMES)], validation.quality
            )
            dataset = lgb.Dataset(
                x, label=y, weight=w, feature_name=list(FEATURE_NAMES)
            )
            valid = lgb.Dataset(vx, label=vy, weight=vw, reference=dataset)
            quality_model = lgb.train(
                params,
                dataset,
                num_boost_round=num_boost_round,
                valid_sets=[valid],
                callbacks=[lgb.early_stopping(30, verbose=False)],
            )
            raw = quality_model.predict(
                validation[list(FEATURE_NAMES)].to_numpy(), num_threads=threads
            )
            calibration = IsotonicRegression(
                y_min=0, y_max=1, out_of_bounds="clip"
            ).fit(raw, validation.quality)
            calibrated = calibration.predict(raw)
            entry["calibration_brier_raw"] = float(
                np.mean((raw - validation.quality.to_numpy()) ** 2)
            )
            entry["calibration_brier"] = float(
                np.mean((calibrated - validation.quality.to_numpy()) ** 2)
            )
            token_dataset = lgb.Dataset(
                output_training[list(FEATURE_NAMES)],
                label=np.log1p(output_training.output_tokens.astype(float)),
            )
            token_valid = lgb.Dataset(
                output_validation[list(FEATURE_NAMES)],
                label=np.log1p(output_validation.output_tokens.astype(float)),
                reference=token_dataset,
            )
            token_model = lgb.train(
                {**params, "objective": "regression", "metric": "l2"},
                token_dataset,
                num_boost_round=num_boost_round,
                valid_sets=[token_valid],
                callbacks=[lgb.early_stopping(30, verbose=False)],
            )
            for field, directory, suffix in [
                ("quality_file", "quality", ".lgbm.txt"),
                ("out_tokens_file", "out_tokens", ".lgbm.txt"),
                ("calibration_file", "calibration", ".isotonic.json"),
            ]:
                (temporary / directory).mkdir(exist_ok=True)
                entry[field] = f"{directory}/{entry['id']}{suffix}"
            quality_model.save_model(str(temporary / entry["quality_file"]))
            token_model.save_model(str(temporary / entry["out_tokens_file"]))
            (temporary / entry["calibration_file"]).write_bytes(
                canonical_json(
                    {
                        "x": calibration.X_thresholds_.tolist(),
                        "y": calibration.y_thresholds_.tolist(),
                    }
                )
            )
            manifest["targets"].append(entry)
        for file in sorted(temporary.rglob("*")):
            if file.is_file():
                manifest["files"][file.relative_to(temporary).as_posix()] = sha256_file(
                    file
                )
        manifest["version"] = bundle_version(manifest)
        (temporary / "manifest.json").write_bytes(canonical_json(manifest))
        destination = root / str(manifest["version"])
        if destination.exists():
            if (destination / "manifest.json").read_bytes() != canonical_json(
                manifest
            ) or any(
                sha256_file(destination / name) != digest
                for name, digest in manifest["files"].items()
            ):
                raise ValueError("existing bundle version was modified")
        else:
            temporary.rename(destination)
        return destination
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)
