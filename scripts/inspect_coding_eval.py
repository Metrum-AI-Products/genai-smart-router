#!/usr/bin/env python3
"""Run and summarize bounded Inspect AI coding evaluations without exposing content.

Raw Inspect logs stay in an ignored caller-selected directory. This program writes
only aggregates; it never echoes environment values, request headers, prompts, or
model output. It intentionally runs Inspect from an empty temporary working
directory so repository Dockerfiles cannot become the evaluator sandbox.
"""
from __future__ import annotations

import argparse, json, os, shutil, subprocess, sys, tempfile, time
from pathlib import Path
from typing import Any

SECRET_WORDS = ("secret", "authorization", "api_key", "apikey", "password", "token_hash", "prompt", "message", "raw_response", "raw_output", "content")
SUITES = {"humaneval": "inspect_evals/humaneval", "bigcodebench": "inspect_evals/bigcodebench"}
SAFE_AGGREGATE_FIELDS = {
    "status", "status_reason", "suite", "model", "model_kind", "api", "reasoning", "limit", "concurrency", "wall_seconds", "exit_code",
    "completed", "partial", "skipped", "blocked", "score", "correct", "scored", "unscored", "unscored_error_rate",
    "p50_sample_time_ms", "p95_sample_time_ms", "input_tokens", "output_tokens", "total_tokens", "token_cost", "attempts", "errors", "fallbacks",
    "reasoning_tokens", "reasoning_attempt_count", "reasoning_successful_attempt_count", "reasoning_reported_attempt_count", "reasoning_provider_model_dialect_coverage",
    "inbound_dialect", "tools_present", "tool_count_bucket", "streaming", "caller_output_cap_field", "request_size_bucket", "tool_schema_bytes_bucket",
}
COVERAGE_TEXT_FIELDS = ("provider", "model", "dialect")
COVERAGE_NUMBER_FIELDS = ("attempts", "reported_attempts", "reasoning_tokens")

def env(name: str, default: str = "") -> str: return os.environ.get(name, default)
def bounded_int(value: str, name: str, low: int, high: int) -> int:
    try: number = int(value)
    except ValueError: raise ValueError(f"{name} must be an integer")
    if not low <= number <= high: raise ValueError(f"{name} must be between {low} and {high}")
    return number

def safe(value: Any) -> Any:
    if isinstance(value, dict): return {str(k): safe(v) for k,v in value.items() if not any(w in str(k).lower() for w in SECRET_WORDS)}
    if isinstance(value, list): return [safe(v) for v in value]
    if isinstance(value, (str, int, float, bool)) or value is None: return value
    return str(value)

def safe_aggregate(value: Any) -> dict[str, Any]:
    if not isinstance(value, dict):
        return {}
    aggregate = {key: safe(value[key]) for key in SAFE_AGGREGATE_FIELDS if key in value and key != "reasoning_provider_model_dialect_coverage"}
    if "reasoning_provider_model_dialect_coverage" in value:
        aggregate["reasoning_provider_model_dialect_coverage"] = safe_reasoning_coverage(value["reasoning_provider_model_dialect_coverage"])
    return aggregate

def safe_reasoning_coverage(value: Any) -> list[dict[str, Any]]:
    """Allow only bounded provider/model/dialect coverage scalars from protected exporters."""
    if not isinstance(value, list):
        return []
    rows: list[dict[str, Any]] = []
    for item in value:
        if not isinstance(item, dict):
            continue
        row: dict[str, Any] = {}
        for field in COVERAGE_TEXT_FIELDS:
            if isinstance(item.get(field), str):
                row[field] = item[field]
        for field in COVERAGE_NUMBER_FIELDS:
            if isinstance(item.get(field), (int, float)) and not isinstance(item[field], bool):
                row[field] = item[field]
        rows.append(row)
    return rows

def load_policy(path: Path | None = None) -> dict[str, Any]:
    path = path or Path(env("EVAL_POLICY", "config/evaluation-policy.example.json"))
    try:
        policy = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"evaluation policy unavailable: {type(exc).__name__}") from exc
    if not isinstance(policy, dict):
        raise ValueError("evaluation policy must be an object")
    return policy

def authoritative_router_cost() -> float:
    """Read the safe scalar exported from stored request-time router usage."""
    raw = env("EVAL_ROUTER_USAGE_JSON")
    if not raw:
        raise ValueError("authoritative router request-time usage aggregate is unavailable")
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValueError("authoritative router request-time usage aggregate is invalid") from exc
    cost = number(value(payload, "stored_request_time_cost_usd"))
    if cost is None or cost < 0:
        raise ValueError("authoritative router request-time cost is unavailable")
    return cost

def reasoning_coverage_text(result: dict[str, Any]) -> str:
    """Render presence-aware reasoning usage; zero is reported, never absence."""
    reported = int(result.get("reasoning_reported_attempt_count", 0) or 0)
    attempts = int(result.get("reasoning_attempt_count", 0) or 0)
    successful = int(result.get("reasoning_successful_attempt_count", 0) or 0)
    if reported == 0:
        return "not reported"
    tokens = result.get("reasoning_tokens", 0)
    if attempts > 0 and reported == attempts:
        return f"{tokens} (complete coverage)"
    # Prefer the total upstream-attempt denominator specified by the evaluation contract.
    return f"{reported} reported / {attempts} upstream attempts ({tokens} tokens; {successful} successful)"

def status_for_error(error: str) -> str:
    text = error.lower()
    if any(x in text for x in ("credential", "api key", "unauthorized", "forbidden", "entitlement", "not configured")): return "blocked"
    if any(x in text for x in ("docker", "inspect", "not found", "unavailable")): return "skipped"
    return "blocked"

def require_run_inputs() -> tuple[str,str,str,int,int,int,Path]:
    model, base_url = env("EVAL_MODEL"), env("EVAL_BASE_URL")
    if not model: raise ValueError("EVAL_MODEL is required (a caller-visible router group or approved direct baseline)")
    if not base_url: raise ValueError("EVAL_BASE_URL is required")
    if not env("EVAL_API_KEY") and not env("OPENAI_API_KEY"):
        raise ValueError("evaluation credential is unavailable; set EVAL_API_KEY in protected environment")
    model_kind = env("EVAL_MODEL_KIND", "router-group")
    if model_kind not in {"router-group", "direct-baseline"}:
        raise ValueError("EVAL_MODEL_KIND must be router-group or direct-baseline")
    if model_kind == "direct-baseline":
        approved = load_policy().get("approved_direct_baselines", [])
        if not isinstance(approved, list) or model not in approved:
            raise ValueError("direct baseline is not in approved_direct_baselines")
    return model, base_url, model_kind, bounded_int(env("EVAL_LIMIT","8"),"EVAL_LIMIT",1,200), bounded_int(env("EVAL_CONCURRENCY","1"),"EVAL_CONCURRENCY",1,16), bounded_int(env("EVAL_TIMEOUT","300"),"EVAL_TIMEOUT",30,3600), Path(env("EVAL_LOG_DIR","tmp/inspect-evals")).expanduser().resolve()

def write_status(log_dir: Path, payload: dict[str,Any]) -> None:
    log_dir.mkdir(parents=True, exist_ok=True)
    (log_dir / "inspect-eval-status.json").write_text(json.dumps(safe(payload), indent=2, sort_keys=True)+"\n", encoding="utf-8")

def value(source: Any, name: str, default: Any = None) -> Any:
    return source.get(name, default) if isinstance(source, dict) else getattr(source, name, default)

def number(source: Any) -> float | None:
    if isinstance(source, str):
        categorical = source.strip().upper()
        if categorical in {"C", "CORRECT"}:
            return 1.0
        if categorical in {"I", "INCORRECT"}:
            return 0.0
    return float(source) if isinstance(source, (int, float)) and not isinstance(source, bool) else None

def nested_number(source: Any, names: tuple[str, ...]) -> float | None:
    """Find a scalar metric in Inspect's summary/stats/usage shapes only."""
    for name in names:
        candidate = number(value(source, name))
        if candidate is not None:
            return candidate
    for child_name in ("stats", "usage", "model_usage", "tokens"):
        child = value(source, child_name)
        if isinstance(child, dict):
            direct = nested_number(child, names)
            if direct is not None:
                return direct
            for item in child.values():
                nested = nested_number(item, names)
                if nested is not None:
                    return nested
    return None

def percentile95(values: list[float]) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, max(0, int((len(ordered) * 0.95 + 0.999999)) - 1))]

def export_inspect_aggregate(log_dir: Path) -> None:
    """Read only Inspect headers and sample summaries into a safe aggregate."""
    try:
        from inspect_ai.log import list_eval_logs, read_eval_log, read_eval_log_sample_summaries
        logs = list_eval_logs(str(log_dir))
        scores: list[float] = []; sample_times: list[float] = []; costs: list[float] = []; scored = unscored = errors = 0; wall_seconds = 0.0
        for info in logs:
            header = read_eval_log(info, header_only=True)
            wall_seconds += number(value(value(header, "stats"), "total_time")) or 0.0
            for summary in read_eval_log_sample_summaries(info):
                if value(summary, "error") is not None:
                    errors += 1
                sample_scores = value(summary, "scores", {}) or {}
                values = sample_scores.values() if isinstance(sample_scores, dict) else []
                sample_values = [number(value(score, "value")) for score in values]
                sample_values = [score for score in sample_values if score is not None]
                if sample_values:
                    scored += 1; scores.append(sum(sample_values) / len(sample_values))
                else:
                    unscored += 1
                sample_time = nested_number(summary, ("total_time", "time", "duration", "elapsed_time"))
                if sample_time is not None:
                    sample_times.append(sample_time * 1000)
                cost = nested_number(summary, ("total_cost", "cost", "cost_usd"))
                if cost is not None:
                    costs.append(cost)
        total = scored + unscored
        payload: dict[str, Any] = {"status": "completed", "completed": True, "partial": bool(unscored or errors), "scored": scored, "unscored": unscored, "errors": errors, "wall_seconds": round(wall_seconds, 3)}
        if total:
            payload["unscored_error_rate"] = round((unscored + errors) / total, 6)
        if scores:
            payload["score"] = round(sum(scores) / len(scores), 6)
            payload["correct"] = sum(score >= 1.0 for score in scores)
        p95 = percentile95(sample_times)
        if p95 is not None:
            payload["p95_sample_time_ms"] = round(p95, 3)
        if costs:
            payload["token_cost"] = round(sum(costs), 9)
        (log_dir / "inspect-aggregate.json").write_text(json.dumps(safe_aggregate(payload), indent=2, sort_keys=True)+"\n", encoding="utf-8")
    except Exception as exc:
        (log_dir / "inspect-aggregate.json").write_text(json.dumps({"status": "partial", "status_reason": "inspect-aggregate-unavailable", "completed": False, "partial": True, "skipped": False, "blocked": False}, indent=2, sort_keys=True)+"\n", encoding="utf-8")
        print(f"evaluation aggregate unavailable: {type(exc).__name__}", file=sys.stderr)

def run(args: argparse.Namespace) -> int:
    try: model, base_url, model_kind, limit, concurrency, timeout, log_dir = require_run_inputs()
    except ValueError as exc:
        print(f"evaluation blocked: {exc}", file=sys.stderr); return 2
    inspect = env("EVAL_INSPECT", "inspect")
    suite_dir = log_dir / args.suite
    if not shutil.which(inspect):
        write_status(suite_dir, {"status":"skipped", "status_reason":"inspect-unavailable", "suite":args.suite, "completed":False, "partial":False, "skipped":True, "blocked":False})
        print("evaluation skipped: Inspect executable unavailable", file=sys.stderr); return 3
    if not shutil.which(env("DOCKER", "docker")):
        write_status(suite_dir, {"status":"skipped", "status_reason":"docker-unavailable", "suite":args.suite, "completed":False, "partial":False, "skipped":True, "blocked":False})
        print("evaluation skipped: Docker unavailable", file=sys.stderr); return 3
    suite_dir.mkdir(parents=True, exist_ok=True)
    api, reasoning = env("EVAL_API", "openai"), env("EVAL_REASONING")
    # Model-group names are deployment-owned strings and may contain '/'. The
    # explicit kind prevents a router group from being mistaken for a provider.
    model_spec = f"{api}/{model}"
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="smart-router-inspect-") as scratch:
        command = [inspect, "eval", SUITES[args.suite], "--model", model_spec, "--model-base-url", base_url, "--limit", str(limit), "--max-connections", str(concurrency), "--log-dir", str(suite_dir)]
        if reasoning:
            command.extend(["-M", f"reasoning_effort={reasoning}"])
        # Inspect provider configuration is inherited, but no secret is included in argv.
        process_env = os.environ.copy()
        process_env.setdefault("OPENAI_BASE_URL", base_url)
        if env("EVAL_API_KEY"):
            process_env["OPENAI_API_KEY"] = env("EVAL_API_KEY")
        process = subprocess.run(command, cwd=scratch, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, env=process_env)
    state = "completed" if process.returncode == 0 else status_for_error(process.stderr)
    write_status(suite_dir, {"status":state,"suite":args.suite,"model":model,"model_kind":model_kind,"api":api,"reasoning":reasoning,"base_url_configured":bool(base_url),"limit":limit,"concurrency":concurrency,"wall_seconds":round(time.monotonic()-started,3),"exit_code":process.returncode,"completed":state == "completed","partial":False,"skipped":state == "skipped","blocked":state == "blocked"})
    if state == "completed":
        export_inspect_aggregate(suite_dir)
    print(f"evaluation {state}: suite={args.suite} model={model} limit={limit}")
    return 0 if process.returncode == 0 else (3 if state == "skipped" else 2)

def aggregate(log_dir: Path) -> dict[str,Any]:
    status_file = log_dir / "inspect-eval-status.json"
    data: dict[str,Any] = safe_aggregate(json.loads(status_file.read_text())) if status_file.exists() else {"status":"skipped", "status_reason":"no-evaluation-status", "completed":False, "partial":False, "skipped":True, "blocked":False}
    # Support a normalized, protected exporter file; raw Inspect logs are not parsed or copied.
    source = log_dir / "inspect-aggregate.json"
    if source.exists(): data.update(safe_aggregate(json.loads(source.read_text())))
    return safe_aggregate(data)

def compare(current: dict[str,Any], baseline: dict[str,Any], policy: dict[str,Any]) -> list[str]:
    limits=policy.get("regression_thresholds",{}); failures=[]
    def growth(current_value: float, baseline_value: float) -> float:
        if baseline_value == 0:
            return 0 if current_value == 0 else float("inf")
        return (current_value - baseline_value) / baseline_value
    checks=(("score","max_score_delta",lambda a,b: b-a),("unscored_error_rate","max_unscored_error_rate",lambda a,b:a-b),("p95_sample_time_ms","max_p95_sample_time_growth",growth),("token_cost","max_token_cost_growth",growth))
    for metric,limit,delta in checks:
        if limit not in limits:
            continue
        if metric not in current or metric not in baseline:
            failures.append(f"{metric} unavailable for regression policy")
        elif delta(float(current[metric]),float(baseline[metric])) > float(limits[limit]):
            failures.append(f"{metric} regression exceeds policy")
    return failures

def report(args: argparse.Namespace) -> int:
    log_dir=Path(args.log_dir) / args.suite; result=aggregate(log_dir); policy=load_policy(Path(args.policy))
    baseline_path=log_dir/"inspect-baseline-aggregate.json"
    baseline_text=env("EVAL_BASELINE_JSON")
    baseline_supplied = bool(baseline_text) or baseline_path.exists()
    baseline=safe_aggregate(json.loads(baseline_text)) if baseline_text else (safe_aggregate(json.loads(baseline_path.read_text())) if baseline_path.exists() else None)
    failures=["protected baseline aggregate is empty or unsafe"] if baseline_supplied and not baseline else (compare(result,baseline,policy) if baseline else [])
    if result.get("model_kind") == "router-group":
        try:
            result["token_cost"] = authoritative_router_cost()
        except ValueError as exc:
            failures.append(str(exc))
        else:
            # Cost comparison must use stored router usage, never Inspect's
            # estimator for a weighted router group.
            failures = [item for item in failures if not item.startswith("token_cost ")]
            if baseline:
                if "token_cost" not in baseline:
                    failures.append("token_cost unavailable for regression policy")
                else:
                    failures.extend(compare({"token_cost": result["token_cost"]}, {"token_cost": baseline["token_cost"]}, {"regression_thresholds": {"max_token_cost_growth": policy.get("regression_thresholds", {}).get("max_token_cost_growth")}}))
    result.update({"baseline_present":bool(baseline),"regression_failures":failures})
    out_json=log_dir/"evaluation-summary.json"; out_md=log_dir/"evaluation-summary.md"; out_json.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    lines=["# Inspect coding evaluation", "", f"Status: **{result.get('status','blocked').upper()}**", "", "This sanitized aggregate excludes prompts, responses, schemas, raw logs, credentials, and headers.", ""]
    for key in ("suite","model","model_kind","api","reasoning","inbound_dialect","tools_present","tool_count_bucket","streaming","caller_output_cap_field","request_size_bucket","tool_schema_bytes_bucket","completed","partial","skipped","blocked","limit","score","correct","scored","unscored","unscored_error_rate","p95_sample_time_ms","token_cost","wall_seconds"):
        if key in result: lines.append(f"- {key}: {result[key]}")
    lines.append(f"- reasoning tokens: {reasoning_coverage_text(result)}")
    lines.append("- reasoning-token counts are a subset of output tokens and are not additive.")
    coverage = result.get("reasoning_provider_model_dialect_coverage")
    if isinstance(coverage, list) and coverage:
        lines += ["", "## Upstream reasoning-token coverage", "", "| Provider | Model | Dialect | Reported / attempts | Tokens |", "| --- | --- | --- | ---: | ---: |"]
        for item in coverage:
            if not isinstance(item, dict): continue
            lines.append(f"| {item.get('provider','')} | {item.get('model','')} | {item.get('dialect','')} | {item.get('reported_attempts',0)} / {item.get('attempts',0)} | {item.get('reasoning_tokens','not reported')} |")
    if failures: lines += ["", "## Regression policy", ""] + [f"- {x}" for x in failures]
    out_md.write_text("\n".join(lines)+"\n")
    if env("EVAL_SAVE_CI_REPORT").lower() == "true":
        timestamp = env("EVAL_CI_REPORT_TIMESTAMP")
        if not timestamp or not timestamp.replace("T", "").replace("Z", "").replace("-", "").replace("_", "").replace(":", "").isdigit():
            raise ValueError("EVAL_CI_REPORT_TIMESTAMP must be a safe timestamp such as 20260723T191500Z")
        report_dir = Path(env("EVAL_CI_REPORT_DIR", "docs/evaluation-reports/inspect")) / timestamp
        report_dir.mkdir(parents=True, exist_ok=False)
        (report_dir / "evaluation-summary.md").write_text(out_md.read_text(encoding="utf-8"), encoding="utf-8")
        (report_dir / "evaluation-summary.json").write_text(out_json.read_text(encoding="utf-8"), encoding="utf-8")
        print(f"evaluation CI report: {report_dir}")
    print(f"evaluation report: status={result.get('status')} summary={out_md}")
    return 1 if failures or not result.get("completed", False) or result.get("partial", False) or result.get("skipped", False) or result.get("blocked", False) else 0

def smoke(args: argparse.Namespace) -> int:
    os.environ.setdefault("EVAL_LIMIT", "2"); args.suite="humaneval"; code=run(args)
    if code: return code
    return report(argparse.Namespace(log_dir=env("EVAL_LOG_DIR","tmp/inspect-evals"), suite="humaneval", policy=env("EVAL_POLICY","config/evaluation-policy.example.json")))

def ci_full(args: argparse.Namespace) -> int:
    """Run the protected, per-suite CI contract without embedding policy in YAML."""
    required = ("EVAL_BASE_URL", "EVAL_API_KEY", "EVAL_MODEL", "EVAL_BASELINE_HUMANEVAL_JSON", "EVAL_BASELINE_BIGCODEBENCH_JSON", "EVAL_ROUTER_USAGE_HUMANEVAL_JSON", "EVAL_ROUTER_USAGE_BIGCODEBENCH_JSON")
    missing = [name for name in required if not env(name)]
    if missing:
        print(f"evaluation blocked: protected CI input unavailable ({', '.join(missing)})", file=sys.stderr)
        return 2
    original = {name: os.environ.get(name) for name in ("EVAL_SUITE", "EVAL_BASELINE_JSON", "EVAL_ROUTER_USAGE_JSON", "EVAL_CI_REPORT_TIMESTAMP")}
    label = env("EVAL_CI_REPORT_TIMESTAMP")
    timestamp = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    try:
        for suite, baseline_var in (("humaneval", "EVAL_BASELINE_HUMANEVAL_JSON"), ("bigcodebench", "EVAL_BASELINE_BIGCODEBENCH_JSON")):
            os.environ["EVAL_SUITE"] = suite
            os.environ["EVAL_BASELINE_JSON"] = env(baseline_var)
            os.environ["EVAL_ROUTER_USAGE_JSON"] = env(f"EVAL_ROUTER_USAGE_{suite.upper()}_JSON")
            os.environ["EVAL_CI_REPORT_TIMESTAMP"] = f"{timestamp}-{label}-{suite}" if label else f"{timestamp}-{suite}"
            code = run(argparse.Namespace(suite=suite))
            if code:
                return code
            code = report(argparse.Namespace(log_dir=env("EVAL_LOG_DIR", "tmp/inspect-evals"), suite=suite, policy=env("EVAL_POLICY", "config/evaluation-policy.example.json")))
            if code:
                return code
    finally:
        for name, prior in original.items():
            if prior is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = prior
    return 0

def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__); subs=parser.add_subparsers(dest="command",required=True)
    r=subs.add_parser("run"); r.add_argument("--suite", choices=sorted(SUITES)); r.set_defaults(func=run)
    q=subs.add_parser("report"); q.add_argument("--log-dir",required=True); q.add_argument("--suite",choices=sorted(SUITES),required=True); q.add_argument("--policy",required=True); q.set_defaults(func=report)
    s=subs.add_parser("smoke"); s.set_defaults(func=smoke)
    c=subs.add_parser("ci-full"); c.set_defaults(func=ci_full)
    args = parser.parse_args()
    return args.func(args)
if __name__ == "__main__": raise SystemExit(main())
