import json
import shutil

# ruff: noqa: F811 -- pytest fixtures are imported from the owned training test module.
from concurrent.futures import ThreadPoolExecutor

import numpy as np
import pytest
from test_train import dataset, trained  # noqa: F401

from lrp.bundle import AtomicBundle, bundle_version, canonical_json, load_bundle
from lrp.features import FEATURE_NAMES


def test_load_predict_explain_and_atomic_failed_reload(trained, tmp_path):
    model = load_bundle(trained)
    vector = np.zeros(len(FEATURE_NAMES), dtype=np.float32)
    predictions = model.predict(vector)
    assert set(predictions) == set(model.models)
    assert all(0 <= p.quality <= 1 and p.out_tokens >= 0 for p in predictions.values())
    assert model.predict(vector, [("unknown", "unknown")]) == {}
    assert len(model.explain(vector)[("synthetic", "cheap/model")]) == 10
    current = AtomicBundle(model)
    corrupt = tmp_path / "corrupt"
    shutil.copytree(trained, corrupt)
    manifest = json.loads((corrupt / "manifest.json").read_text())
    (corrupt / manifest["targets"][0]["quality_file"]).write_text("modified")
    with pytest.raises(ValueError, match="digest"):
        current.reload(corrupt)
    assert current.snapshot() is model
    with ThreadPoolExecutor(max_workers=4) as pool:
        values = list(pool.map(lambda _: current.snapshot().predict(vector), range(20)))
    assert values == [predictions] * 20
    assert current.reload(trained).version == model.version


@pytest.mark.parametrize(
    "change", ["traversal", "unhashed", "calibration", "undertrained", "provenance"]
)
def test_manifest_contract_rejected(trained, tmp_path, change):
    shutil.copytree(trained, tmp_path / "bundle")
    root = tmp_path / "bundle"
    manifest = json.loads((root / "manifest.json").read_text())
    if change == "traversal":
        manifest["files"]["../outside"] = "0" * 64
    elif change == "unhashed":
        manifest["files"].pop(manifest["targets"][0]["quality_file"])
    elif change == "undertrained":
        manifest["targets"][0]["n_train"] = 199
    elif change == "provenance":
        manifest["synthetic"] = False
    else:
        from lrp.bundle import sha256_file

        file = root / manifest["targets"][0]["calibration_file"]
        file.write_text('{"x":[0,1],"y":[1,0]}')
        manifest["files"][file.relative_to(root).as_posix()] = sha256_file(file)
    manifest["version"] = bundle_version(manifest)
    (root / "manifest.json").write_bytes(canonical_json(manifest))
    with pytest.raises(ValueError):
        load_bundle(root)
