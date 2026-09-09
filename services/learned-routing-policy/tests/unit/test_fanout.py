import asyncio
import json

import httpx
import pytest
from lrp.collect import DataError, read_rows
from lrp.fanout import run_fanout, total_spend, usage_and_cost
from test_collect import request, write

TARGET = {
    "provider": "openrouter",
    "model": "synthetic/model",
    "model_ref": "test-alias",
}


def pricing(models=("synthetic/model",)):
    return {
        "data": [
            {"id": model, "pricing": {"prompt": "0.000002", "completion": "0.000003"}}
            for model in models
        ]
    }


def completion(**changes):
    result = {
        "choices": [
            {"message": {"content": "4", "extra": "not saved"}, "finish_reason": "stop"}
        ],
        "usage": {"prompt_tokens": 10, "completion_tokens": 2, "cost": 0.000028},
        "provider": "synthetic-backend",
    }
    result.update(changes)
    return result


@pytest.mark.asyncio
async def test_refresh_exact_prices_usage_and_resume(tmp_path):
    calls = []

    def handler(req):
        calls.append(req)
        return httpx.Response(
            200, json=pricing() if req.method == "GET" else completion()
        )

    source = write(tmp_path / "source", [request()])
    out = tmp_path / "responses"
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        assert (
            await run_fanout(requests=source, out=out, targets=[TARGET], client=client)
        )["written"] == 1
        assert (
            await run_fanout(requests=source, out=out, targets=[TARGET], client=client)
        )["skipped"] == 1
    assert len(calls) == 2
    row = next(iter(read_rows(out)))
    assert row["target"]["model"] == "synthetic/model"
    assert row["pricing"]["input_per_m_usd"] == 2
    assert row["cost_usd"] == pytest.approx(0.000026)
    assert row["billed_cost_usd"] == 0.000028
    assert row["serving_provider"] == "synthetic-backend"
    assert "not saved" not in out.read_text()
    assert row["attempts"][0]["sequence"] == 1
    assert row["ttfb_ms"] >= 0 and row["duration_ms"] >= 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "target,cap,expected",
    [
        ({**TARGET, "model": "synthetic/model:nitro"}, 32, "pricing_unavailable"),
        ({**TARGET, "honors_max_tokens": False}, 32, "output_cap_not_honored"),
        (TARGET, None, "uncapped_not_authorized"),
    ],
)
async def test_ineligible_never_calls_completion(tmp_path, target, cap, expected):
    def handler(req):
        assert req.method == "GET"
        return httpx.Response(200, json=pricing())

    source = write(tmp_path / "source", [request(max_tokens=cap)])
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        await run_fanout(
            requests=source, out=tmp_path / "out", targets=[target], client=client
        )
    row = next(iter(read_rows(tmp_path / "out")))
    assert row["status"] == "ineligible" and row["error_class"] == expected
    assert "cost_usd" not in row and row["attempts"] == []


@pytest.mark.asyncio
async def test_retry_jitter_and_per_target_concurrency(tmp_path, monkeypatch):
    active = peak = posts = 0
    delays = []
    original_sleep = asyncio.sleep

    async def sleep(delay):
        delays.append(delay)
        await original_sleep(0)

    monkeypatch.setattr("lrp.fanout.asyncio.sleep", sleep)

    async def handler(req):
        nonlocal active, peak, posts
        if req.method == "GET":
            return httpx.Response(200, json=pricing())
        posts += 1
        active += 1
        peak = max(peak, active)
        await original_sleep(0.001)
        active -= 1
        return (
            httpx.Response(429)
            if posts <= 2
            else httpx.Response(200, json=completion())
        )

    source = write(tmp_path / "source", [request(str(i)) for i in range(10)])
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        await run_fanout(
            requests=source,
            out=tmp_path / "out",
            targets=[TARGET],
            client=client,
            concurrency=2,
            seed=2,
        )
    assert peak == 2
    assert posts > 10 and delays and all(0 <= d <= 2 for d in delays)
    assert all(len(r["attempts"]) <= 3 for r in read_rows(tmp_path / "out"))


@pytest.mark.asyncio
async def test_timeout_400_and_unknown_usage_not_free(tmp_path):
    count = 0

    async def handler(req):
        nonlocal count
        if req.method == "GET":
            return httpx.Response(200, json=pricing())
        count += 1
        await asyncio.sleep(0.05)
        return httpx.Response(200, json=completion())

    source = write(tmp_path / "source", [request()])
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        await run_fanout(
            requests=source,
            out=tmp_path / "out",
            targets=[TARGET],
            client=client,
            timeout_s=0.01,
            retries=0,
        )
    row = next(iter(read_rows(tmp_path / "out")))
    assert row["status"] == "timeout" and count == 1 and total_spend(row) is None
    assert usage_and_cost({}, {"input_per_m_usd": 1, "output_per_m_usd": 1}) == (
        None,
        None,
        None,
    )
    assert (
        total_spend(
            {"status": "ok", "attempts": [{"cost_usd": 1}, {"billed_cost_usd": 2}]}
        )
        == 3
    )
    assert total_spend({"status": "ok", "attempts": [{"cost_usd": 1}, {}]}) is None


@pytest.mark.asyncio
async def test_router_requires_explicit_single_target_and_preserves_cap(tmp_path):
    posted = []

    def handler(req):
        if req.method == "GET":
            return httpx.Response(200, json=pricing())
        posted.append(json.loads(req.content))
        return httpx.Response(
            200, json=completion(), headers={"x-request-id": "synthetic-request"}
        )

    source = write(tmp_path / "source", [request(max_tokens=1)])
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        with pytest.raises(DataError, match="single_target"):
            await run_fanout(
                requests=source,
                out=tmp_path / "out",
                targets=[TARGET],
                client=client,
                via="router",
                base_url="http://127.0.0.1:18080/v1",
            )
        target = {**TARGET, "router_group": "test-one", "router_targets": [TARGET]}
        await run_fanout(
            requests=source,
            out=tmp_path / "out",
            targets=[target],
            client=client,
            via="router",
            base_url="http://127.0.0.1:18080/v1",
        )
    assert posted[0]["model"] == "test-one" and posted[0]["max_tokens"] == 1
    assert (
        next(iter(read_rows(tmp_path / "out")))["router_request_id"]
        == "synthetic-request"
    )


@pytest.mark.asyncio
async def test_spend_guard_stops_after_unknown_and_rejects_resume_changes(tmp_path):
    count = 0

    def handler(req):
        nonlocal count
        if req.method == "GET":
            return httpx.Response(200, json=pricing())
        count += 1
        return httpx.Response(200, json=completion(usage={}))

    source = write(tmp_path / "source", [request("r1"), request("r2")])
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        await run_fanout(
            requests=source,
            out=tmp_path / "out",
            targets=[TARGET],
            client=client,
            max_total_cost_usd=1,
        )
        with pytest.raises(DataError, match="resume_input_changed"):
            await run_fanout(
                requests=source,
                out=tmp_path / "out",
                targets=[TARGET],
                client=client,
                max_total_cost_usd=2,
            )
    assert count == 1
    assert (
        list(read_rows(tmp_path / "out"))[1]["error_class"]
        == "spend_evidence_unavailable"
    )
