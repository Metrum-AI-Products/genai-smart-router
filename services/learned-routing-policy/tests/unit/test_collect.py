import json
import os
from pathlib import Path

import pytest
from lrp.collect import (
    DataError,
    collect,
    journal,
    read_rows,
    strict_json,
    validate_record,
)


def request(request_id="synthetic-1", **changes):
    row = {
        "schema_version": "lrp.request.v1",
        "request_id": request_id,
        "captured_at": "2026-09-09T00:00:00Z",
        "source": "synthetic",
        "group": "synthetic-eval",
        "dialect": "openai-chat",
        "messages": [{"role": "user", "content": "What is 2+2?"}],
        "max_tokens": 32,
    }
    row.update(changes)
    return row


def write(path, rows):
    path.write_text("".join(json.dumps(row) + "\n" for row in rows))
    path.chmod(0o600)
    return path


def test_metadata_only_never_reconstructs_content(tmp_path):
    log = write(
        tmp_path / "logs",
        [
            {
                "request_id": "r1",
                "messages": [{"role": "user", "content": "must discard"}],
                "caller_id": "discard-me",
                "token_id": "discard-me",
            }
        ],
    )
    out = tmp_path / "requests"
    assert collect(router_log=log, out=out) == {
        "written": 0,
        "skipped": 0,
        "missing_content": 1,
    }
    assert out.read_bytes() == b""


def test_collect_join_discards_caller_identity_and_resumes(tmp_path):
    row = request(caller={"id": "discard", "tokenId": "discard", "project": "demo"})
    source = write(tmp_path / "source", [row])
    log = write(
        tmp_path / "log",
        [{"request_id": row["request_id"], "caller_environment": "staging"}],
    )
    out = tmp_path / "out"
    assert collect(dataset=source, router_log=log, out=out)["written"] == 1
    result = next(iter(read_rows(out)))
    assert result["caller"] == {"project": "demo", "environment": "staging"}
    assert "discard" not in out.read_text()
    assert result["session_key"].startswith("sha256:")
    assert collect(dataset=source, router_log=log, out=out)["skipped"] == 1
    assert out.stat().st_mode & 0o777 == 0o600


def test_governed_approval_required(tmp_path):
    source = write(tmp_path / "source", [request(source="content_capture")])
    with pytest.raises(DataError, match="approval"):
        collect(dataset=source, out=tmp_path / "out")
    assert (
        collect(dataset=source, out=tmp_path / "out", approved_content=True)["written"]
        == 1
    )


def test_conflicting_resume_and_partial_tail_recovery(tmp_path):
    source = write(tmp_path / "source", [request()])
    out = tmp_path / "out"
    collect(dataset=source, out=out)
    with out.open("ab") as handle:
        handle.write(b'{"partial":')
    assert collect(dataset=source, out=out)["skipped"] == 1
    write(source, [request(messages=[{"role": "user", "content": "changed"}])])
    with pytest.raises(DataError, match="resume_input_changed"):
        collect(dataset=source, out=out)


@pytest.mark.parametrize(
    "changes",
    [
        {"secret": "never persist"},
        {"messages": [{"role": "user", "content": "test", "extra": "never persist"}]},
        {"context": {"tokenId": "never persist"}},
        {"max_tokens": True},
        {"captured_at": "invalid"},
    ],
)
def test_strict_record_rejects_unknowns_and_coercions(changes):
    with pytest.raises(DataError, match="invalid_request_record"):
        validate_record(request(**changes), "request")


@pytest.mark.parametrize("text", ['{"a":1,"a":2}', '{"a":NaN}', "[]", "{"])
def test_strict_json(text):
    with pytest.raises(DataError):
        strict_json(text)


def test_file_permissions_symlink_hardlink_and_lock(tmp_path):
    source = write(tmp_path / "source", [request()])
    source.chmod(0o644)
    with pytest.raises(DataError, match="private_file"):
        list(read_rows(source))
    source.chmod(0o600)
    link = tmp_path / "link"
    link.symlink_to(source)
    with pytest.raises(DataError, match="symlink"):
        list(read_rows(link))
    os.link(source, tmp_path / "hardlink")
    with pytest.raises(DataError, match="regular_private"):
        list(read_rows(source))
    out = tmp_path / "out"
    with journal(out), pytest.raises(DataError, match="locked"), journal(out):
        pass


def test_storage_rejects_repositories(tmp_path):
    repo = tmp_path / "repo"
    repo.mkdir()
    (repo / ".git").write_text("gitdir: elsewhere")
    with pytest.raises(DataError, match="outside_repository"), journal(repo / "out"):
        pass


def test_all_synthetic_fixtures_validate():
    fixture_dir = Path(__file__).parents[1] / "fixtures"
    for name, kind in (
        ("requests", "request"),
        ("responses", "response"),
        ("judgments", "judgment"),
    ):
        rows = list(read_rows(fixture_dir / (name + ".ndjson")))
        assert 0 < len(rows) <= 100
        for row in rows:
            assert validate_record(row, kind)["source"] == "synthetic"
