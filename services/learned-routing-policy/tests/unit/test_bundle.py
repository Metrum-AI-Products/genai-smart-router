# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import base64
import json
import shutil

# ruff: noqa: F811 -- pytest fixtures are imported from the owned training test module.
from concurrent.futures import ThreadPoolExecutor

import numpy as np
import pytest
from lrp.bundle import (
    SIGNATURE_FILE,
    AtomicBundle,
    bundle_version,
    canonical_json,
    generate_keypair,
    load_bundle,
    sign_bundle,
    verify_bundle_signature,
    write_trust_keys,
)
from lrp.features import FEATURE_NAMES
from test_train import dataset, trained  # noqa: F401


def _private_copy(src, dest):
    shutil.copytree(src, dest)
    dest.chmod(0o700)
    for path in dest.rglob("*"):
        path.chmod(0o700 if path.is_dir() else 0o600)


def _operator_keys(tmp_path, *key_ids: str):
    tmp_path.mkdir(parents=True, exist_ok=True)
    tmp_path.chmod(0o700)
    trust = {}
    seeds = {}
    for key_id in key_ids:
        public, seed = generate_keypair()
        trust[key_id] = public
        seeds[key_id] = seed
        (tmp_path / f"{key_id}.seed").write_text(base64.b64encode(seed).decode())
        (tmp_path / f"{key_id}.seed").chmod(0o600)
        (tmp_path / f"{key_id}.pub").write_text(base64.b64encode(public).decode())
        (tmp_path / f"{key_id}.pub").chmod(0o600)
    trust_path = tmp_path / "trust.json"
    write_trust_keys(trust_path, trust)
    return trust, seeds, trust_path


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
    _private_copy(trained, corrupt)
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
    _private_copy(trained, tmp_path / "bundle")
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


def test_sign_verify_require_signed_and_trust_rotation(trained, tmp_path):
    root = tmp_path / "signed"
    _private_copy(trained, root)
    trust, seeds, trust_path = _operator_keys(tmp_path, "lrp-test-2026-01", "lrp-test-2026-06")

    unsigned = load_bundle(root)
    assert unsigned.signature_key_id is None
    with pytest.raises(ValueError, match="signature required"):
        load_bundle(root, require_signed=True, trusted_keys=trust_path)

    envelope = sign_bundle(root, seeds["lrp-test-2026-01"], "lrp-test-2026-01")
    assert envelope["manifest_version"] == unsigned.version
    assert verify_bundle_signature(root, trust_path) == "lrp-test-2026-01"
    loaded = load_bundle(root, require_signed=True, trusted_keys=trust)
    assert loaded.signature_key_id == "lrp-test-2026-01"
    assert loaded.version == unsigned.version

    # Rotate signing key while keeping both public keys trusted.
    sign_bundle(root, seeds["lrp-test-2026-06"], "lrp-test-2026-06")
    assert (
        load_bundle(root, require_signed=True, trusted_keys=trust_path).signature_key_id
        == "lrp-test-2026-06"
    )

    # Retire the active key from the trust store: signature no longer verifies.
    write_trust_keys(trust_path, {"lrp-test-2026-01": trust["lrp-test-2026-01"]})
    with pytest.raises(ValueError, match="unknown signature key"):
        load_bundle(root, require_signed=True, trusted_keys=trust_path)


def test_signature_hash_binding_and_negative_cases(trained, tmp_path):
    root = tmp_path / "bound"
    _private_copy(trained, root)
    trust, seeds, trust_path = _operator_keys(tmp_path, "lrp-test-bind")
    sign_bundle(root, seeds["lrp-test-bind"], "lrp-test-bind")

    # Tampered signature bytes.
    envelope = json.loads((root / SIGNATURE_FILE).read_text())
    bad_sig = bytearray(base64.b64decode(envelope["signature_base64"]))
    bad_sig[0] ^= 0x01
    envelope["signature_base64"] = base64.b64encode(bytes(bad_sig)).decode()
    (root / SIGNATURE_FILE).write_bytes(canonical_json(envelope))
    with pytest.raises(ValueError, match="signature mismatch"):
        verify_bundle_signature(root, trust_path)

    # Re-sign, then break hash binding by rewriting signature digest without resigning.
    sign_bundle(root, seeds["lrp-test-bind"], "lrp-test-bind")
    envelope = json.loads((root / SIGNATURE_FILE).read_text())
    envelope["manifest_sha256"] = "0" * 64
    (root / SIGNATURE_FILE).write_bytes(canonical_json(envelope))
    with pytest.raises(ValueError, match="hash binding"):
        load_bundle(root, trusted_keys=trust)

    # Signature present without a trust store must not silently skip verification.
    sign_bundle(root, seeds["lrp-test-bind"], "lrp-test-bind")
    with pytest.raises(ValueError, match="without trusted keys"):
        load_bundle(root)

    # Wrong operator key cannot verify a foreign signature.
    other_trust, _, other_path = _operator_keys(tmp_path / "other", "lrp-test-other")
    with pytest.raises(ValueError, match="unknown signature key"):
        verify_bundle_signature(root, other_path)
    assert other_trust


def test_signed_reload_rollback_preserves_old_bundle(trained, tmp_path):
    good = tmp_path / "good"
    bad = tmp_path / "bad"
    _private_copy(trained, good)
    _private_copy(trained, bad)
    trust, seeds, trust_path = _operator_keys(tmp_path, "lrp-test-reload")
    sign_bundle(good, seeds["lrp-test-reload"], "lrp-test-reload")
    sign_bundle(bad, seeds["lrp-test-reload"], "lrp-test-reload")

    current = AtomicBundle(
        load_bundle(good, require_signed=True, trusted_keys=trust_path)
    )
    previous = current.snapshot()
    envelope = json.loads((bad / SIGNATURE_FILE).read_text())
    envelope["signature_base64"] = base64.b64encode(b"\x00" * 64).decode()
    (bad / SIGNATURE_FILE).write_bytes(canonical_json(envelope))
    with pytest.raises(ValueError, match="signature"):
        current.reload(bad, require_signed=True, trusted_keys=trust_path)
    assert current.snapshot() is previous
    assert current.reload(good, require_signed=True, trusted_keys=trust).version == previous.version
