from concurrent.futures import ThreadPoolExecutor

import numpy as np
import pytest
from lrp.features import (
    FEATURE_NAMES,
    FeatureBuilder,
    ONNXEmbedder,
    SyntheticEmbedder,
    featurize,
    normalize,
)


def test_identical_train_serve_ir(tmp_path):
    builder = FeatureBuilder(SyntheticEmbedder())
    request = {
        "system": "Answer briefly.",
        "messages": [
            {
                "role": "user",
                "parts": [{"type": "text", "text": "What is two plus two?"}],
            }
        ],
        "tools": [
            {
                "type": "function",
                "function": {"name": "synthetic", "parameters": {"type": "object"}},
            }
        ],
    }
    context = {
        "toolCount": 3,
        "hasTools": True,
        "estimatedTokens": 55,
        "textChars": 100,
    }
    offline = {
        **request,
        "messages": [{"role": "user", "content": "What is two plus two?"}],
        "captured_at": "2026-09-09T00:00:00Z",
        "request_id": "synthetic-1",
        "session_key": "session-1",
        "source": "synthetic",
        "group": "demo",
        "context": context,
    }
    frame = featurize([offline], tmp_path / "features.parquet", builder=builder)
    served = builder.build({"request": request, "context": context})
    trained = frame[list(FEATURE_NAMES)].to_numpy(dtype=np.float32)[0]
    assert served.tobytes() == trained.tobytes()
    assert served[FEATURE_NAMES.index("toolCount")] == 3
    assert served[FEATURE_NAMES.index("hasTools")] == 1


def test_bounded_latest_input_language_and_concurrency():
    builder = FeatureBuilder(SyntheticEmbedder())
    payload = {
        "request": {"input": "old " * 250000 + "The latest question is important."}
    }
    assert normalize(payload)[1].endswith("The latest question is important.")
    assert len(normalize(payload)[1]) <= 32768
    expected = builder.build(payload)
    with ThreadPoolExecutor(max_workers=4) as pool:
        assert all(
            value.tobytes() == expected.tobytes()
            for value in pool.map(builder.build, [payload] * 8)
        )
    assert np.isfinite(builder.build({"request": {"input": ""}})).all()
    assert (
        builder.language(
            "This is a sufficiently long English sentence about a computer and its software."
        )
        == "en"
    )
    assert (
        builder.language(
            "Ceci est une phrase suffisamment longue en français pour identifier la langue."
        )
        == "fr"
    )


def tiny_onnx(tmp_path):
    import onnx
    from onnx import TensorProto, helper, numpy_helper
    from tokenizers import Tokenizer, models, pre_tokenizers, processors

    tokenizer = Tokenizer(
        models.WordLevel(
            {"[UNK]": 0, "[CLS]": 1, "[SEP]": 2, "old": 3, "latest": 4},
            unk_token="[UNK]",
        )
    )
    tokenizer.pre_tokenizer = pre_tokenizers.Whitespace()
    tokenizer.post_processor = processors.TemplateProcessing(
        single="[CLS] $A [SEP]", special_tokens=[("[CLS]", 1), ("[SEP]", 2)]
    )
    tokenizer.save(str(tmp_path / "tokenizer.json"))
    weights = np.arange(5 * 384, dtype=np.int16).reshape(5, 384).astype(np.int8)
    nodes = [
        helper.make_node(
            "DequantizeLinear", ["weights", "scale", "zero"], ["float_weights"]
        ),
        helper.make_node("Gather", ["float_weights", "input_ids"], ["output"], axis=0),
    ]
    graph = helper.make_graph(
        nodes,
        "synthetic-int8-test",
        [helper.make_tensor_value_info("input_ids", TensorProto.INT64, [1, "seq"])],
        [helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, "seq", 384])],
        [
            numpy_helper.from_array(weights, "weights"),
            numpy_helper.from_array(np.array(0.1, dtype=np.float32), "scale"),
            numpy_helper.from_array(np.array(0, dtype=np.int8), "zero"),
        ],
    )
    model = helper.make_model(
        graph, opset_imports=[helper.make_opsetid("", 17)], ir_version=9
    )
    onnx.save(model, tmp_path / "model.onnx")
    (tmp_path / "model.onnx").chmod(0o600)
    (tmp_path / "tokenizer.json").chmod(0o600)
    return tmp_path / "model.onnx", tmp_path / "tokenizer.json"


def test_real_onnx_tokenizer_quantized_inference(tmp_path):
    model, tokenizer = tiny_onnx(tmp_path)
    embedder = ONNXEmbedder(model, tokenizer)
    vector = embedder.encode("old " * 10000 + "latest")
    encoding = embedder.tokenizer.encode("old " * 10000 + "latest")
    assert len(encoding.ids) == 512 and encoding.ids[-2] == 4
    assert vector.shape == (384,) and np.isclose(np.linalg.norm(vector), 1)
    assert embedder.session.get_session_options().intra_op_num_threads == 1
    with pytest.raises(ValueError, match="threads"):
        ONNXEmbedder(model, tokenizer, threads=8)


def test_protected_io_rejects_repo_symlink_hardlink_and_public_mode(tmp_path):
    import os

    from lrp.collect import DataError
    from lrp.features import preflight_file, read_rows, write_private

    repo = tmp_path / "public"
    repo.mkdir()
    (repo / ".git").write_text("gitdir: synthetic")
    with pytest.raises(DataError):
        write_private(repo / "artifact", b"synthetic")
    original = tmp_path / "original"
    write_private(original, b"synthetic")
    linked = tmp_path / "linked"
    os.link(original, linked)
    with pytest.raises(DataError):
        preflight_file(original)
    linked.unlink()
    linked.symlink_to(original)
    with pytest.raises(DataError):
        preflight_file(linked)
    original.chmod(0o644)
    with pytest.raises(DataError):
        write_private(original, b"must not overwrite")
    assert original.read_bytes() == b"synthetic"
    records = tmp_path / "records.ndjson"
    write_private(records, b'{"source":"synthetic","source":"synthetic"}\n')
    with pytest.raises(DataError):
        read_rows(records)


def test_feature_temporary_path_preflight(tmp_path):
    from lrp.collect import DataError

    output = tmp_path / "features.parquet"
    temporary = tmp_path / "features.parquet.tmp"
    temporary.write_text("unchanged")
    temporary.chmod(0o644)
    with pytest.raises(DataError):
        featurize([], output, builder=FeatureBuilder(SyntheticEmbedder()))
    assert not output.exists() and temporary.read_text() == "unchanged"
