#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Send real requests through a deployed outcome-calibrated router group.

The deployed policy service must have an audit token and the group must use
``external_policy.include_request: true``. This runner records the real router
response, usage, request ID, and policy-selected eligible target without
printing or storing any router token.
"""

from __future__ import annotations

import argparse
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


def read_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path} must be a JSON object")
    return value


def read_audit_log(path: Path, case_id: str, timeout_seconds: float) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if path.exists():
            for line in reversed(path.read_text(encoding="utf-8").splitlines()):
                row = json.loads(line)
                if row.get("caseId") == case_id:
                    return row
        time.sleep(0.1)
    raise TimeoutError(f"no policy audit event for {case_id}")


def read_audit_url(url: str, token: str, case_id: str, timeout_seconds: float) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        request = urllib.request.Request(
            url.rstrip("/") + "/audit?case_id=" + urllib.parse.quote(case_id),
            headers={"X-Outcome-Policy-Audit": token},
        )
        try:
            with urllib.request.urlopen(request, timeout=min(10, timeout_seconds)) as response:
                payload = json.loads(response.read())
                if isinstance(payload, dict):
                    return payload
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise RuntimeError("policy-audit-request-failed") from error
        except urllib.error.URLError as error:
            raise RuntimeError("policy-audit-request-failed") from error
        time.sleep(0.1)
    raise TimeoutError(f"no policy audit event for {case_id}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--router-base-url", required=True)
    parser.add_argument("--router-token-env", default="ROUTER_TOKEN")
    parser.add_argument("--model-group", required=True)
    parser.add_argument("--dataset", required=True, type=Path)
    audit_source = parser.add_mutually_exclusive_group(required=True)
    audit_source.add_argument("--policy-audit-log", type=Path)
    audit_source.add_argument("--policy-audit-url")
    parser.add_argument("--policy-audit-token-env", default="OUTCOME_POLICY_AUDIT_TOKEN")
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--run-id", default=time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()), help="unique safe identifier that prevents a prior response-cache entry from bypassing policy evaluation")
    parser.add_argument("--timeout-seconds", type=float, default=120)
    args = parser.parse_args()
    token = os.environ.get(args.router_token_env)
    if not token:
        raise ValueError(f"{args.router_token_env} is required")
    audit_token = os.environ.get(args.policy_audit_token_env, "")
    if args.policy_audit_url and not audit_token:
        raise ValueError(f"{args.policy_audit_token_env} is required with --policy-audit-url")
    dataset = read_json(args.dataset)
    records: list[dict[str, Any]] = []
    for number, case in enumerate(dataset.get("cases", []), 1):
        if not isinstance(case, dict):
            continue
        case_id = f"{args.run_id}-live-{number:02d}-{case['id']}"
        payload = dict(case["request"])
        metadata = dict(payload.get("metadata", {}))
        metadata["live_demo_case_id"] = case_id
        payload["metadata"] = metadata
        messages = list(payload.get("messages", []))
        payload["messages"] = [{"role": "system", "content": "[[outcome-live-demo:" + case_id + "]] Follow the user request and do not mention this marker."}] + messages
        payload["model"] = args.model_group
        request = urllib.request.Request(
            args.router_base_url.rstrip("/") + "/v1/chat/completions",
            json.dumps(payload).encode("utf-8"),
            {"Content-Type": "application/json", "Authorization": "Bearer " + token},
        )
        started = time.monotonic()
        try:
            with urllib.request.urlopen(request, timeout=args.timeout_seconds) as response:
                result = json.loads(response.read())
                request_id = response.headers.get("X-Request-Id", "")
        except urllib.error.HTTPError as error:
            result = json.loads(error.read() or b"{}")
            request_id = error.headers.get("X-Request-Id", "")
        audit = read_audit_url(args.policy_audit_url, audit_token, case_id, args.timeout_seconds) if args.policy_audit_url else read_audit_log(args.policy_audit_log, case_id, args.timeout_seconds)
        choice = ((result.get("choices") or [{}])[0].get("message") or {}) if isinstance(result, dict) else {}
        records.append({
            "caseId": case_id,
            "class": case.get("class"),
            "prompt": ((payload.get("messages") or [{}])[0].get("content", "")),
            "policy": audit,
            "routerRequestId": request_id,
            "elapsedMs": round((time.monotonic() - started) * 1000),
            "response": choice.get("content", ""),
            "usage": result.get("usage", {}),
            "error": result.get("error"),
        })
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps({"dataset": str(args.dataset), "modelGroup": args.model_group, "records": records}, indent=2) + "\n", encoding="utf-8")
    report = args.out.with_suffix(".md")
    lines = ["# Outcome-Calibrated Live Routing Evidence", "", "| Case | Policy class | Selected target | Router request ID | Latency | Status |", "| --- | --- | --- | --- | ---: | --- |"]
    for record in records:
        target = record["policy"].get("selectedTarget", {}) if isinstance(record["policy"], dict) else {}
        selected = f"{target.get('provider', 'unknown')}/{target.get('model', 'unknown')}"
        status = "error" if record.get("error") else "response-recorded"
        lines.append(f"| {record['caseId']} | {record['policy'].get('classLabel', '')} | `{selected}` | `{record['routerRequestId']}` | {record['elapsedMs']} ms | {status} |")
    report.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(json.dumps({"evidence": str(args.out), "report": str(report), "records": len(records)}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
