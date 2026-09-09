# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import json
import os
from pathlib import Path

import httpx
import pytest
from lrp.collect import DataError, read_rows
from lrp.judge import Sandbox, cache_identity, judge_requests
from test_collect import request, write
from test_fanout import completion, pricing

ANCHOR = ("openrouter", "synthetic/anchor")
JUDGE = "synthetic/judge"


def response(model, **changes):
    row = {
        "schema_version": "lrp.response.v1",
        "request_id": "synthetic-1",
        "source": "synthetic",
        "target": {"provider": "openrouter", "model": model},
        "started_at": "2026-09-09T00:00:00Z",
        "duration_ms": 1.0,
        "status": "ok",
        "content": "4",
    }
    row.update(changes)
    return row


def inputs(tmp_path, req=None, anchor_changes=None):
    requests = write(tmp_path / "requests", [req or request()])
    responses = write(
        tmp_path / "responses",
        [
            response("synthetic/candidate"),
            response(ANCHOR[1], **(anchor_changes or {})),
        ],
    )
    return requests, responses


def mock_judge(verdicts, seen):
    def handler(req):
        if req.method == "GET":
            return httpx.Response(200, json=pricing([JUDGE]))
        seen.append(json.loads(req.content))
        value = verdicts.pop(0)
        return httpx.Response(
            200, json=completion(choices=[{"message": {"content": value}}])
        )

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "votes,quality,agree",
    [
        (["A", "B"], 1.0, True),
        (["B", "A"], 0.0, True),
        (["A", "A"], 0.5, False),
        (["TIE", "TIE"], 1.0, True),
    ],
)
async def test_position_swapped_pairwise(tmp_path, votes, quality, agree):
    reqs, resps = inputs(tmp_path)
    seen = []
    async with httpx.AsyncClient(
        transport=mock_judge(
            [
                json.dumps({"winner": v, "reason": "private reason discarded"})
                for v in votes
            ],
            seen,
        )
    ) as client:
        await judge_requests(
            requests=reqs,
            responses=resps,
            out=tmp_path / "out",
            anchor=ANCHOR,
            judge_model=JUDGE,
            client=client,
        )
    rows = list(read_rows(tmp_path / "out"))
    assert rows[0]["quality"] == quality and rows[0]["detail"]["swapped_agree"] is agree
    assert rows[1]["quality"] == 1
    assert len(seen) == 2
    first, second = [json.loads(r["messages"][1]["content"]) for r in seen]
    assert first["A"] == second["B"] and first["B"] == second["A"]
    assert "private reason" not in (tmp_path / "out").read_text()


@pytest.mark.asyncio
async def test_strict_parse_retry_uncertain_excluded(tmp_path):
    reqs, resps = inputs(tmp_path)
    seen = []
    invalid = '{"winner":"A","reason":"x","extra":"discard"}'
    async with httpx.AsyncClient(
        transport=mock_judge(["garbage", invalid, "[]", "{}"], seen)
    ) as client:
        result = await judge_requests(
            requests=reqs,
            responses=resps,
            out=tmp_path / "out",
            anchor=ANCHOR,
            judge_model=JUDGE,
            client=client,
        )
    row = next(iter(read_rows(tmp_path / "out")))
    assert row.get("quality") is None and row["detail"]["parse_failed"]
    assert result["uncertain"] == 1 and len(seen) == 4
    assert row["judge_cost_usd"] == pytest.approx(4 * 0.000028)


@pytest.mark.asyncio
async def test_failed_anchor_uses_absolute_rubric(tmp_path):
    reqs, resps = inputs(tmp_path, anchor_changes={"status": "timeout"})
    seen = []
    async with httpx.AsyncClient(
        transport=mock_judge(['{"score":7,"reason":"adequate"}'], seen)
    ) as client:
        await judge_requests(
            requests=reqs,
            responses=resps,
            out=tmp_path / "out",
            anchor=ANCHOR,
            judge_model=JUDGE,
            client=client,
        )
    rows = list(read_rows(tmp_path / "out"))
    assert rows[0]["quality"] == 0.7 and rows[0]["method"] == "absolute_rubric:v1"
    assert rows[1]["quality"] == 0 and len(seen) == 1


@pytest.mark.asyncio
async def test_verifier_precedes_judge_and_unavailable_is_not_failure(tmp_path):
    reqs, resps = inputs(
        tmp_path, request(verifier={"kind": "exact", "spec": {"expected": "4"}})
    )

    def never(req):
        pytest.fail("Verifier path must not call LLM")

    async with httpx.AsyncClient(transport=httpx.MockTransport(never)) as client:
        result = await judge_requests(
            requests=reqs,
            responses=resps,
            out=tmp_path / "out",
            anchor=ANCHOR,
            judge_model=JUDGE,
            client=client,
        )
    assert result["uncertain"] == 2
    assert all(
        r.get("quality") is None and r["detail"]["unsupported"]
        for r in read_rows(tmp_path / "out")
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("passed", [True, False])
async def test_verifier_result_mapping_without_llm(tmp_path, passed):
    # Orchestration fake only; real isolation is tested separately below.
    class FakeSandbox:
        rootfs = Path("/synthetic/rootfs")
        timeout_s = 30

        def verify(self, verifier, content):
            return float(passed), {"passed": passed}

    reqs, resps = inputs(
        tmp_path, request(verifier={"kind": "exact", "spec": {"expected": "4"}})
    )
    await judge_requests(
        requests=reqs,
        responses=resps,
        out=tmp_path / "out",
        anchor=ANCHOR,
        judge_model=JUDGE,
        sandbox=FakeSandbox(),
    )
    assert all(r["quality"] == float(passed) for r in read_rows(tmp_path / "out"))


@pytest.mark.asyncio
async def test_cache_resume_and_content_invalidation(tmp_path):
    reqs, resps = inputs(tmp_path)
    seen = []
    async with httpx.AsyncClient(
        transport=mock_judge(
            ['{"winner":"A","reason":"ok"}', '{"winner":"B","reason":"ok"}'], seen
        )
    ) as client:
        options = {
            "requests": reqs,
            "responses": resps,
            "anchor": ANCHOR,
            "judge_model": JUDGE,
            "client": client,
            "cache": tmp_path / "cache",
        }
        await judge_requests(**options, out=tmp_path / "first")
        assert (await judge_requests(**options, out=tmp_path / "first"))["skipped"] == 2
        assert (await judge_requests(**options, out=tmp_path / "second"))["cached"] == 2
        assert len(seen) == 2
        write(
            resps,
            [response("synthetic/candidate", content="changed"), response(ANCHOR[1])],
        )
        with pytest.raises(DataError, match="resume_judgment_input_changed"):
            await judge_requests(**options, out=tmp_path / "first")
    assert (tmp_path / "cache").stat().st_mode & 0o777 == 0o600


def test_cache_identity_includes_model_settings_content_verifier():
    req, resp, ref = request(), response("synthetic/candidate"), response(ANCHOR[1])
    baseline = cache_identity(req, resp, ref, JUDGE, {})
    assert baseline != cache_identity(req, {**resp, "content": "5"}, ref, JUDGE, {})
    assert baseline != cache_identity(req, resp, {**ref, "content": "5"}, JUDGE, {})
    assert baseline != cache_identity(req, resp, ref, "synthetic/new-judge", {})
    assert baseline != cache_identity(req, resp, ref, JUDGE, {"temperature": 1})
    assert baseline != cache_identity(
        {**req, "verifier": {"kind": "exact", "spec": {"expected": "5"}}},
        resp,
        ref,
        JUDGE,
        {},
    )


def test_sandbox_command_is_real_namespace_isolation_and_missing_root_fails_closed(
    tmp_path,
):
    rootfs = tmp_path / "rootfs"
    rootfs.mkdir(mode=0o700)
    sandbox = Sandbox(rootfs)
    command = sandbox.command()
    assert "--unshare-all" in command and "--clearenv" in command
    assert "--disable-userns" in command
    assert command[command.index("--size") + 1] == "16777216"
    assert command[command.index("--uid") + 1] == "65534"
    assert command[command.index("--ro-bind") + 1 : command.index("--ro-bind") + 3] == [
        str(rootfs),
        "/",
    ]
    quality, detail = sandbox.verify(
        {"kind": "pytest", "spec": {"tests": "raise AssertionError"}}, ""
    )
    assert quality is None and detail["unsupported"]
    assert not (rootfs / "test_solution.py").exists()


@pytest.mark.skipif(
    not os.environ.get("LRP_TEST_ROOTFS")
    and not os.environ.get("LRP_REQUIRE_SANDBOX_TESTS"),
    reason="requires operator-provisioned isolated Python/pytest rootfs and namespace support",
)
def test_real_sandbox_network_host_denial_and_wall_timeout(tmp_path, monkeypatch):
    assert os.environ.get("LRP_TEST_ROOTFS"), (
        "mandatory sandbox test requires LRP_TEST_ROOTFS"
    )
    sandbox = Sandbox(Path(os.environ["LRP_TEST_ROOTFS"]))
    assert sandbox.verify({"kind": "exact", "spec": {"expected": "4"}}, "4")[0] == 1
    assert sandbox.verify({"kind": "exact", "spec": {"expected": "4"}}, "5")[0] == 0
    assert (
        sandbox.verify({"kind": "regex", "spec": {"pattern": r"[0-9]+"}}, "123")[0] == 1
    )
    assert (
        sandbox.verify(
            {"kind": "json_schema", "spec": {"schema": {"type": "integer"}}}, "123"
        )[0]
        == 1
    )
    assert (
        sandbox.verify(
            {"kind": "json_schema", "spec": {"schema": {"type": "integer"}}}, '"bad"'
        )[0]
        == 0
    )
    host_file = tmp_path / "host-only"
    host_file.write_text("synthetic sentinel")
    host_network = os.readlink("/proc/self/ns/net")
    monkeypatch.setenv("LRP_SYNTHETIC_HOST_SECRET", "must-not-reach-worker")
    tests = f"""import os, socket, pathlib, pytest
import solution
def test_isolation():
    assert solution.value == 42
    assert not pathlib.Path('/home').exists()
    assert not pathlib.Path({str(host_file)!r}).exists()
    assert 'LRP_SYNTHETIC_HOST_SECRET' not in os.environ
    assert os.getuid() == 65534
    assert os.readlink('/proc/self/ns/net') != {host_network!r}
    assert len(pathlib.Path('/proc/net/route').read_text().splitlines()) <= 1
    with pytest.raises(OSError):
        socket.create_connection(('192.0.2.1', 443), timeout=1)

def test_scratch_limit():
    assert os.statvfs('/work').f_blocks * os.statvfs('/work').f_frsize <= 16777216
    with pytest.raises(OSError):
        for i in range(32):
            pathlib.Path('/work/fill-' + str(i)).write_bytes(b'x' * 1048576)
"""
    assert (
        sandbox.verify({"kind": "pytest", "spec": {"tests": tests}}, "value = 42")[0]
        == 1
    )
    short = Sandbox(sandbox.rootfs, timeout_s=0.2)
    quality, detail = short.verify(
        {"kind": "pytest", "spec": {"tests": "import time; time.sleep(60)"}}, ""
    )
    assert quality == 0 and detail["timeout"]
