#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Operator-gated, budget-capped live API compatibility runner (issue #94).

This is deliberately outside every-PR `make test`. Makefile gates must pass
before this script runs. Missing credentials report disposition blocked and
exit nonzero for operator runs — never a green certification claim.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MATRIX_DIR = ROOT / "tests" / "api_compat" / "testdata" / "live" / "matrices"
SCHEMA_VERSION = "api-compat-live/v1"

# Absolute ceilings. Matrix files may only tighten these.
HARD_CEILINGS = {
    "max_requests": 20,
    "max_tokens": 8192,
    "max_wall_seconds": 300,
    "max_spend_usd": 2.0,
}

FORBIDDEN_ENVIRONMENTS = {"", "production", "default", "prod", "prd"}
FORBIDDEN_MATRIX_NAMES = {"", "default", "all"}
FORBIDDEN_CALLERS = {"", "default", "all"}


class LiveCompatError(Exception):
    """Fail-closed operator or matrix error."""

    def __init__(self, message: str, *, exit_code: int = 2) -> None:
        super().__init__(message)
        self.exit_code = exit_code


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def git_sha() -> str:
    try:
        completed = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=ROOT,
            check=False,
            capture_output=True,
            text=True,
        )
    except OSError:
        return "unknown"
    if completed.returncode != 0:
        return "unknown"
    return completed.stdout.strip() or "unknown"


def resolve_matrix_path(matrix_id: str) -> Path:
    if matrix_id in FORBIDDEN_MATRIX_NAMES:
        raise LiveCompatError("API_COMPAT_LIVE_MATRIX must name an approved matrix")
    if "/" in matrix_id or "\\" in matrix_id or ".." in matrix_id:
        raise LiveCompatError("API_COMPAT_LIVE_MATRIX must be a bare approved matrix id")
    path = MATRIX_DIR / f"{matrix_id}.json"
    if not path.is_file():
        raise LiveCompatError(f"approved live matrix not found: {matrix_id}")
    return path


def load_matrix(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise LiveCompatError(f"invalid live matrix file: {exc}") from exc
    if not isinstance(data, dict):
        raise LiveCompatError("live matrix root must be an object")
    if data.get("schema_version") != SCHEMA_VERSION:
        raise LiveCompatError(f"live matrix schema_version must be {SCHEMA_VERSION}")
    if data.get("approved") is not True:
        raise LiveCompatError("live matrix is not marked approved")
    if data.get("matrix_id") != path.stem:
        raise LiveCompatError("live matrix_id must match filename stem")
    cases = data.get("cases")
    if not isinstance(cases, list) or not cases:
        raise LiveCompatError("live matrix must declare a nonempty cases list")
    return data


def harden_caps(raw: dict[str, Any] | None) -> dict[str, float]:
    caps = dict(HARD_CEILINGS)
    if raw is None:
        return caps
    if not isinstance(raw, dict):
        raise LiveCompatError("live matrix caps must be an object")
    for key, ceiling in HARD_CEILINGS.items():
        if key not in raw:
            continue
        try:
            value = float(raw[key])
        except (TypeError, ValueError) as exc:
            raise LiveCompatError(f"live matrix cap {key} is invalid") from exc
        if value <= 0:
            raise LiveCompatError(f"live matrix cap {key} must be positive")
        caps[key] = min(value, float(ceiling))
    return caps


def load_credentials(path: str) -> dict[str, str]:
    credential_path = Path(path)
    if not credential_path.is_file():
        raise LiveCompatError(
            "blocked: API_COMPAT_LIVE_CREDENTIAL_FILE missing; not a certification pass",
            exit_code=2,
        )
    mode = credential_path.stat().st_mode & 0o777
    if mode != 0o600:
        raise LiveCompatError(
            "blocked: API_COMPAT_LIVE_CREDENTIAL_FILE must be mode 0600; not a certification pass",
            exit_code=2,
        )
    try:
        text = credential_path.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise LiveCompatError(f"blocked: cannot read credential file: {exc}", exit_code=2) from exc
    if not text:
        raise LiveCompatError(
            "blocked: credential file empty; live credentials unavailable; not a certification pass",
            exit_code=2,
        )
    token = ""
    model_group = ""
    if text.startswith("{"):
        try:
            payload = json.loads(text)
        except json.JSONDecodeError as exc:
            raise LiveCompatError(
                "blocked: credential file JSON invalid; not a certification pass",
                exit_code=2,
            ) from exc
        if not isinstance(payload, dict):
            raise LiveCompatError(
                "blocked: credential file JSON must be an object; not a certification pass",
                exit_code=2,
            )
        token = str(payload.get("token") or payload.get("api_key") or "").strip()
        model_group = str(payload.get("model_group") or payload.get("model") or "").strip()
    else:
        token = text.splitlines()[0].strip()
    if not token:
        raise LiveCompatError(
            "blocked: live credentials unavailable (empty token); not a certification pass",
            exit_code=2,
        )
    return {"token": token, "model_group": model_group}


def validate_operator_gates() -> dict[str, str]:
    matrix = env("API_COMPAT_LIVE_MATRIX")
    environment = env("API_COMPAT_LIVE_ENVIRONMENT")
    caller = env("API_COMPAT_LIVE_CALLER")
    base_url = env("API_COMPAT_LIVE_BASE_URL").rstrip("/")
    credential_file = env("API_COMPAT_LIVE_CREDENTIAL_FILE")
    confirm = env("API_COMPAT_LIVE_CONFIRM")

    if matrix in FORBIDDEN_MATRIX_NAMES:
        raise LiveCompatError("API_COMPAT_LIVE_MATRIX must name an approved matrix")
    if environment.lower() in FORBIDDEN_ENVIRONMENTS:
        raise LiveCompatError("API_COMPAT_LIVE_ENVIRONMENT must name a non-production environment")
    if caller in FORBIDDEN_CALLERS:
        raise LiveCompatError("API_COMPAT_LIVE_CALLER must name a least-privilege caller")
    if not base_url:
        raise LiveCompatError("API_COMPAT_LIVE_BASE_URL is required")
    if not (base_url.startswith("https://") or base_url.startswith("http://127.") or base_url.startswith("http://localhost")):
        raise LiveCompatError("API_COMPAT_LIVE_BASE_URL must be https or loopback http")
    if not credential_file:
        raise LiveCompatError("API_COMPAT_LIVE_CREDENTIAL_FILE is required")
    if confirm != f"{matrix}:{environment}":
        raise LiveCompatError("API_COMPAT_LIVE_CONFIRM must bind the selected matrix and environment")
    return {
        "matrix": matrix,
        "environment": environment,
        "caller": caller,
        "base_url": base_url,
        "credential_file": credential_file,
        "confirm": confirm,
    }


def estimate_tokens(payload: Any) -> int:
    if not isinstance(payload, dict):
        return 0
    usage = payload.get("usage")
    if isinstance(usage, dict):
        total = usage.get("total_tokens")
        if isinstance(total, int):
            return max(total, 0)
        prompt = usage.get("prompt_tokens") or usage.get("input_tokens") or 0
        completion = usage.get("completion_tokens") or usage.get("output_tokens") or 0
        try:
            return max(int(prompt), 0) + max(int(completion), 0)
        except (TypeError, ValueError):
            return 0
    return 0


def run_case(
    *,
    base_url: str,
    token: str,
    model_group: str,
    case: dict[str, Any],
    timeout: float,
) -> dict[str, Any]:
    case_id = str(case.get("id") or "unnamed")
    method = str(case.get("method") or "POST").upper()
    path = str(case.get("path") or "")
    if not path.startswith("/"):
        raise LiveCompatError(f"{case_id}: path must be absolute")
    body = case.get("body")
    if not isinstance(body, dict):
        raise LiveCompatError(f"{case_id}: body must be an object")
    request_body = json.loads(json.dumps(body))
    if model_group and "model" not in request_body:
        request_body["model"] = model_group
    if "model" not in request_body or not str(request_body["model"]).strip():
        raise LiveCompatError(f"{case_id}: model/model_group is required")
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "application/json",
    }
    extra = case.get("headers")
    if isinstance(extra, dict):
        for key, value in extra.items():
            headers[str(key)] = str(value)
    raw = json.dumps(request_body).encode("utf-8")
    request = urllib.request.Request(
        f"{base_url}{path}",
        data=raw,
        headers=headers,
        method=method,
    )
    started = time.monotonic()
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            status = int(response.status)
            response_body = response.read(1_048_576)
    except urllib.error.HTTPError as exc:
        status = int(exc.code)
        response_body = exc.read(1_048_576)
    except urllib.error.URLError as exc:
        raise LiveCompatError(f"{case_id}: request failed: {exc}") from exc
    elapsed = time.monotonic() - started
    parsed: Any
    try:
        parsed = json.loads(response_body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        parsed = None
    expected = int(case.get("expected_status") or 200)
    ok = status == expected
    return {
        "id": case_id,
        "status": status,
        "expected_status": expected,
        "ok": ok,
        "elapsed_seconds": round(elapsed, 3),
        "tokens": estimate_tokens(parsed),
        "token_budget": int(case.get("token_budget") or 0),
    }


def execute_matrix(
    *,
    gates: dict[str, str],
    matrix: dict[str, Any],
    credentials: dict[str, str],
) -> dict[str, Any]:
    caps = harden_caps(matrix.get("caps") if isinstance(matrix.get("caps"), dict) else None)
    cost_per_1k = float(matrix.get("cost_per_1k_tokens_usd") or 0.01)
    if cost_per_1k < 0:
        raise LiveCompatError("cost_per_1k_tokens_usd must be nonnegative")
    model_group = credentials.get("model_group") or env("API_COMPAT_LIVE_MODEL_GROUP")
    cases = matrix["cases"]
    if len(cases) > int(caps["max_requests"]):
        raise LiveCompatError(
            f"matrix declares {len(cases)} cases but max_requests cap is {int(caps['max_requests'])}"
        )
    started = time.monotonic()
    results: list[dict[str, Any]] = []
    tokens_used = 0
    spend = 0.0
    for case in cases:
        if not isinstance(case, dict):
            raise LiveCompatError("each live matrix case must be an object")
        wall = time.monotonic() - started
        if wall > caps["max_wall_seconds"]:
            raise LiveCompatError("blocked: live wall-clock cap exceeded; not a certification pass")
        remaining = max(1.0, caps["max_wall_seconds"] - wall)
        result = run_case(
            base_url=gates["base_url"],
            token=credentials["token"],
            model_group=model_group,
            case=case,
            timeout=min(60.0, remaining),
        )
        budget = int(result.get("token_budget") or 0)
        used = max(int(result.get("tokens") or 0), budget)
        tokens_used += used
        spend = (tokens_used / 1000.0) * cost_per_1k
        if tokens_used > caps["max_tokens"]:
            raise LiveCompatError("blocked: live token cap exceeded; not a certification pass")
        if spend > caps["max_spend_usd"]:
            raise LiveCompatError("blocked: live spend cap exceeded; not a certification pass")
        results.append(result)
        if not result["ok"]:
            break
    failures = [row for row in results if not row["ok"]]
    disposition = "passed" if results and not failures else "failed"
    return {
        "disposition": disposition,
        "schema_version": SCHEMA_VERSION,
        "matrix_id": matrix["matrix_id"],
        "environment": gates["environment"],
        "caller": gates["caller"],
        "git_sha": git_sha(),
        "date_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "caps": caps,
        "requests": len(results),
        "tokens_used": tokens_used,
        "estimated_spend_usd": round(spend, 6),
        "results": results,
        "certification": False,
        "limitations": [
            "Bounded operator smoke only; not Harbor, agent, or statistical certification.",
            "Historical synthetic Responses streaming in offline suites was not native streaming; native Responses streaming is covered by issue #103 offline contracts.",
        ],
    }


def ci_report() -> int:
    """PR-safe reporter: missing secrets yield blocked evidence with exit 0."""
    required = (
        "API_COMPAT_LIVE_BASE_URL",
        "API_COMPAT_LIVE_CREDENTIAL",
        "API_COMPAT_LIVE_MATRIX",
        "API_COMPAT_LIVE_ENVIRONMENT",
        "API_COMPAT_LIVE_CALLER",
        "API_COMPAT_LIVE_CONFIRM",
    )
    # Prefer distinct secret env names in CI; fall back to operator names.
    present = {
        "API_COMPAT_LIVE_BASE_URL": env("API_COMPAT_LIVE_BASE_URL"),
        "API_COMPAT_LIVE_CREDENTIAL": env("API_COMPAT_LIVE_CREDENTIAL") or env("API_COMPAT_LIVE_CREDENTIAL_FILE"),
        "API_COMPAT_LIVE_MATRIX": env("API_COMPAT_LIVE_MATRIX"),
        "API_COMPAT_LIVE_ENVIRONMENT": env("API_COMPAT_LIVE_ENVIRONMENT"),
        "API_COMPAT_LIVE_CALLER": env("API_COMPAT_LIVE_CALLER"),
        "API_COMPAT_LIVE_CONFIRM": env("API_COMPAT_LIVE_CONFIRM"),
    }
    missing = [name for name in required if not present[name]]
    report = {
        "disposition": "blocked" if missing else "ready",
        "schema_version": SCHEMA_VERSION,
        "git_sha": git_sha(),
        "date_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "missing": missing,
        "certification": False,
        "message": (
            "blocked: live API compatibility secrets unavailable; "
            "not a certification pass"
            if missing
            else "live secrets present; operator make api-compat-live still required"
        ),
    }
    print(json.dumps(report, indent=2, sort_keys=True))
    if missing:
        print(report["message"], file=sys.stderr)
        return 0
    print(report["message"], file=sys.stderr)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--ci-report",
        action="store_true",
        help="Emit blocked/ready CI evidence without failing the default PR gate on missing secrets",
    )
    args = parser.parse_args(argv)
    if args.ci_report:
        return ci_report()
    try:
        gates = validate_operator_gates()
        matrix = load_matrix(resolve_matrix_path(gates["matrix"]))
        credentials = load_credentials(gates["credential_file"])
        report = execute_matrix(gates=gates, matrix=matrix, credentials=credentials)
    except LiveCompatError as exc:
        blocked = {
            "disposition": "blocked",
            "schema_version": SCHEMA_VERSION,
            "git_sha": git_sha(),
            "date_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "message": str(exc),
            "certification": False,
        }
        print(json.dumps(blocked, indent=2, sort_keys=True))
        print(str(exc), file=sys.stderr)
        return exc.exit_code
    print(json.dumps(report, indent=2, sort_keys=True))
    if report["disposition"] != "passed":
        print("api-compat-live failed one or more bounded cases", file=sys.stderr)
        return 1
    print(
        "api-compat-live completed bounded non-production smoke "
        f"(matrix={report['matrix_id']} sha={report['git_sha']}); not full certification",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
