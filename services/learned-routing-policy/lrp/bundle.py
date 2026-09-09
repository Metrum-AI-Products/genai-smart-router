# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
"""Hash-checked immutable model snapshots; failed reloads preserve the old one."""

from __future__ import annotations

import hashlib
import json
import math
import threading
from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from itertools import pairwise
from pathlib import Path, PurePosixPath
from types import MappingProxyType
from typing import Any

import numpy as np

from lrp.features import (
    FEATURE_NAMES,
    FeatureBuilder,
    ONNXEmbedder,
    SyntheticEmbedder,
    Vector,
    capped_threads,
)
from lrp.policy import Prediction
from lrp.schemas import TargetKey

SCHEMA_VERSION = "lrp.bundle.v1"


def target_key(value: Mapping[str, Any]) -> TargetKey:
    provider, model = value.get("provider"), value.get("model")
    if (
        not isinstance(provider, str)
        or not provider
        or not isinstance(model, str)
        or not model
    ):
        raise ValueError("invalid target identity")
    return provider, model


def target_id(key: TargetKey) -> str:
    return hashlib.sha256(json.dumps(key, separators=(",", ":")).encode()).hexdigest()[
        :32
    ]


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_json(value: Any) -> bytes:
    return (
        json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n"
    ).encode()


def bundle_version(manifest: Mapping[str, Any]) -> str:
    return hashlib.sha256(
        canonical_json({k: v for k, v in manifest.items() if k != "version"})
    ).hexdigest()[:24]


def _file(root: Path, relative: str) -> Path:
    path = PurePosixPath(relative)
    if path.is_absolute() or ".." in path.parts or "\\" in relative or not path.parts:
        raise ValueError("unsafe bundle path")
    candidate = root.joinpath(*path.parts)
    if candidate.is_symlink() or any(
        p.is_symlink() for p in candidate.parents if p != root.parent
    ):
        raise ValueError("bundle symlinks are forbidden")
    if (
        not candidate.resolve().is_relative_to(root.resolve())
        or not candidate.is_file()
    ):
        raise ValueError("missing or unsafe bundle file")
    return candidate


@dataclass(frozen=True)
class TargetModel:
    quality: Any
    out_tokens: Any
    calibration_x: tuple[float, ...]
    calibration_y: tuple[float, ...]
    n_train: int


@dataclass(frozen=True)
class ModelBundle:
    version: str
    manifest: Mapping[str, Any]
    builder: FeatureBuilder
    models: Mapping[TargetKey, TargetModel]
    baseline: Mapping[TargetKey, Prediction]
    threads: int = 1

    def build(self, payload: Any, turn_index: int | None = None) -> Vector:
        return self.builder.build(payload, turn_index)

    def predict(
        self, vector: Vector, targets: Iterable[TargetKey] | None = None
    ) -> dict[TargetKey, Prediction]:
        vector = np.asarray(vector, dtype=np.float32)
        if vector.shape != (len(FEATURE_NAMES),) or not np.isfinite(vector).all():
            raise ValueError("invalid prediction vector")
        predictions = {}
        for key in self.models if targets is None else targets:
            if key not in self.models:
                continue
            model = self.models[key]
            raw = float(
                model.quality.predict(vector[None, :], num_threads=self.threads)[0]
            )
            quality = float(np.interp(raw, model.calibration_x, model.calibration_y))
            log_tokens = float(
                model.out_tokens.predict(vector[None, :], num_threads=self.threads)[0]
            )
            output = math.expm1(min(max(log_tokens, 0), math.log1p(10_000_000)))
            if not math.isfinite(quality) or not math.isfinite(output):
                raise ValueError("nonfinite model prediction")
            predictions[key] = Prediction(quality, output)
        return predictions

    def bt_predictions(
        self, targets: Iterable[TargetKey] | None = None
    ) -> dict[TargetKey, Prediction]:
        return {
            key: self.baseline[key]
            for key in (self.baseline if targets is None else targets)
            if key in self.baseline
        }

    def _explain_target(
        self, vector: Vector, key: TargetKey, limit: int = 10
    ) -> list[dict[str, Any]]:
        if key not in self.models:
            return []
        values = np.asarray(
            self.models[key].quality.predict(
                vector[None, :], pred_contrib=True, num_threads=self.threads
            )
        )[0, :-1]
        order = np.argsort(-np.abs(values), kind="stable")[: min(max(limit, 0), 10)]
        return [
            {"feature": FEATURE_NAMES[i], "contribution": float(values[i])}
            for i in order
        ]

    def explain(
        self, vector: Vector, keys: Iterable[TargetKey] | None = None
    ) -> dict[TargetKey, list[dict[str, Any]]]:
        return {
            key: self._explain_target(vector, key)
            for key in (self.models if keys is None else keys)
        }


def load_bundle(path: str | Path, threads: int = 1) -> ModelBundle:
    import lightgbm as lgb

    capped_threads(threads)
    root = Path(path).resolve()
    manifest_file = _file(root, "manifest.json")
    if manifest_file.stat().st_size > 4 * 1024 * 1024:
        raise ValueError("manifest too large")
    manifest = json.loads(manifest_file.read_bytes())
    if manifest.get("schema_version") != SCHEMA_VERSION or manifest.get(
        "feature_names"
    ) != list(FEATURE_NAMES):
        raise ValueError("incompatible bundle schema or features")
    if manifest.get("version") != bundle_version(manifest):
        raise ValueError("manifest digest mismatch")
    hashes = manifest.get("files", {})
    if not isinstance(hashes, dict) or len(hashes) > 2048:
        raise ValueError("invalid bundle file inventory")
    for name, digest in hashes.items():
        if sha256_file(_file(root, name)) != digest:
            raise ValueError("bundle file digest mismatch")

    def verified(name: str) -> Path:
        if name not in hashes:
            raise ValueError("unhashed bundle dependency")
        return _file(root, name)

    embedding = manifest["embedding"]
    if embedding["kind"] == "synthetic":
        if not manifest.get("synthetic"):
            raise ValueError("synthetic embedding requires synthetic provenance")
        builder = FeatureBuilder(SyntheticEmbedder())
    elif embedding["kind"] == "onnx":
        builder = FeatureBuilder(
            ONNXEmbedder(
                verified(embedding["model_path"]),
                verified(embedding["tokenizer_path"]),
                threads,
                embedding.get("pooling", "cls"),
                embedding.get("prefix", ""),
            )
        )
    else:
        raise ValueError("unknown embedding kind")
    if builder.embedder.fingerprint != manifest.get("embedding_fingerprint"):
        raise ValueError("embedding feature fingerprint mismatch")
    models: dict[TargetKey, TargetModel] = {}
    baseline = {}
    seen = set()
    for entry in manifest["targets"]:
        key = target_key(entry)
        if key in seen or entry["id"] != target_id(key):
            raise ValueError("duplicate or inconsistent bundle target")
        seen.add(key)
        strength, output = float(entry["bt_strength"]), float(entry["mean_out_tokens"])
        if (
            not math.isfinite(strength)
            or not 0 <= strength <= 1
            or not math.isfinite(output)
            or output < 0
        ):
            raise ValueError("invalid baseline prediction")
        if entry.get("skipped"):
            continue
        if entry["n_train"] < max(200, manifest.get("min_train_rows", 200)):
            raise ValueError("undertrained learned model")
        baseline[key] = Prediction(strength, output)
        calibration = json.loads(verified(entry["calibration_file"]).read_bytes())
        xs, ys = calibration["x"], calibration["y"]
        if (
            not xs
            or len(xs) != len(ys)
            or any(not math.isfinite(v) for v in [*xs, *ys])
        ):
            raise ValueError("invalid calibration")
        if (
            any(a >= b for a, b in pairwise(xs))
            or any(a > b for a, b in pairwise(ys))
            or any(not 0 <= v <= 1 for v in ys)
        ):
            raise ValueError("nonmonotone calibration")
        quality = lgb.Booster(model_file=str(verified(entry["quality_file"])))
        tokens = lgb.Booster(model_file=str(verified(entry["out_tokens_file"])))
        if (
            quality.num_feature() != len(FEATURE_NAMES)
            or tokens.num_feature() != len(FEATURE_NAMES)
            or quality.feature_name() != list(FEATURE_NAMES)
            or tokens.feature_name() != list(FEATURE_NAMES)
        ):
            raise ValueError("model feature count mismatch")
        models[key] = TargetModel(
            quality, tokens, tuple(xs), tuple(ys), entry["n_train"]
        )
    if not models:
        raise ValueError("bundle has no trained targets")
    snapshot = ModelBundle(
        manifest["version"],
        MappingProxyType(manifest),
        builder,
        MappingProxyType(models),
        MappingProxyType(baseline),
        threads,
    )
    # Validation and readiness include a real inference through both stages.
    snapshot.predict(snapshot.build({"text": "Synthetic warmup request."}))
    return snapshot


class AtomicBundle:
    def __init__(self, bundle: ModelBundle | None = None) -> None:
        self._bundle = bundle
        self._lock = threading.Lock()
        self._reload_lock = threading.Lock()

    def snapshot(self) -> ModelBundle | None:
        with self._lock:
            return self._bundle

    def reload(self, path: str | Path, threads: int = 1) -> ModelBundle:
        with self._reload_lock:
            candidate = load_bundle(path, threads)
            with self._lock:
                self._bundle = candidate
            return candidate


def validate(path: str | Path, threads: int = 1) -> ModelBundle:
    return load_bundle(path, threads)
