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
SUITES = {"humaneval": "humaneval", "bigcodebench": "bigcodebench"}
SAFE_AGGREGATE_FIELDS = {
    "status", "status_reason", "suite", "model", "model_kind", "api", "reasoning", "limit", "concurrency", "wall_seconds", "exit_code",
    "completed", "partial", "skipped", "blocked", "score", "correct", "scored", "unscored", "unscored_error_rate",
    "p50_sample_time_ms", "p95_sample_time_ms", "input_tokens", "output_tokens", "total_tokens", "token_cost", "attempts", "errors", "fallbacks",
    "inbound_dialect", "tools_present", "tool_count_bucket", "streaming", "caller_output_cap_field", "request_size_bucket", "tool_schema_bytes_bucket",
}

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
    return {key: safe(value[key]) for key in SAFE_AGGREGATE_FIELDS if key in value}

def status_for_error(error: str) -> str:
    text = error.lower()
    if any(x in text for x in ("credential", "api key", "unauthorized", "forbidden", "entitlement", "not configured")): return "blocked"
    if any(x in text for x in ("docker", "inspect", "not found", "unavailable")): return "skipped"
    return "blocked"

def require_run_inputs() -> tuple[str,str,int,int,int,Path]:
    model, base_url = env("EVAL_MODEL"), env("EVAL_BASE_URL")
    if not model: raise ValueError("EVAL_MODEL is required (a caller-visible router group or approved direct baseline)")
    if not base_url: raise ValueError("EVAL_BASE_URL is required")
    if not env("EVAL_API_KEY") and not env("OPENAI_API_KEY"):
        raise ValueError("evaluation credential is unavailable; set EVAL_API_KEY in protected environment")
    return model, base_url, bounded_int(env("EVAL_LIMIT","8"),"EVAL_LIMIT",1,200), bounded_int(env("EVAL_CONCURRENCY","1"),"EVAL_CONCURRENCY",1,16), bounded_int(env("EVAL_TIMEOUT","300"),"EVAL_TIMEOUT",30,3600), Path(env("EVAL_LOG_DIR","tmp/inspect-evals"))

def write_status(log_dir: Path, payload: dict[str,Any]) -> None:
    log_dir.mkdir(parents=True, exist_ok=True)
    (log_dir / "inspect-eval-status.json").write_text(json.dumps(safe(payload), indent=2, sort_keys=True)+"\n", encoding="utf-8")

def run(args: argparse.Namespace) -> int:
    try: model, base_url, limit, concurrency, timeout, log_dir = require_run_inputs()
    except ValueError as exc:
        print(f"evaluation blocked: {exc}", file=sys.stderr); return 2
    inspect = env("EVAL_INSPECT", "inspect")
    if not shutil.which(inspect):
        write_status(log_dir, {"status":"skipped", "status_reason":"inspect-unavailable", "suite":args.suite, "completed":False, "partial":False, "skipped":True, "blocked":False})
        print("evaluation skipped: Inspect executable unavailable", file=sys.stderr); return 3
    if not shutil.which(env("DOCKER", "docker")):
        write_status(log_dir, {"status":"skipped", "status_reason":"docker-unavailable", "suite":args.suite, "completed":False, "partial":False, "skipped":True, "blocked":False})
        print("evaluation skipped: Docker unavailable", file=sys.stderr); return 3
    log_dir.mkdir(parents=True, exist_ok=True)
    api, reasoning = env("EVAL_API", "openai"), env("EVAL_REASONING")
    model_spec = model if "/" in model else f"{api}/{model}"
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="smart-router-inspect-") as scratch:
        command = [inspect, "eval", SUITES[args.suite], "--model", model_spec, "--model-base-url", base_url, "--limit", str(limit), "--max-connections", str(concurrency), "--log-dir", str(log_dir)]
        if reasoning:
            command.extend(["-M", f"reasoning_effort={reasoning}"])
        # Inspect provider configuration is inherited, but no secret is included in argv.
        process_env = os.environ.copy()
        process_env.setdefault("OPENAI_BASE_URL", base_url)
        if env("EVAL_API_KEY"):
            process_env.setdefault("OPENAI_API_KEY", env("EVAL_API_KEY"))
        process = subprocess.run(command, cwd=scratch, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, env=process_env)
    state = "completed" if process.returncode == 0 else status_for_error(process.stderr)
    write_status(log_dir, {"status":state,"suite":args.suite,"model":model,"api":api,"reasoning":reasoning,"base_url_configured":bool(base_url),"limit":limit,"concurrency":concurrency,"wall_seconds":round(time.monotonic()-started,3),"exit_code":process.returncode,"completed":state == "completed","partial":False,"skipped":state == "skipped","blocked":state == "blocked"})
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
    checks=(("score","max_score_delta",lambda a,b: b-a),("unscored_error_rate","max_unscored_error_rate",lambda a,b:a-b),("p95_sample_time_ms","max_p95_sample_time_growth",lambda a,b:(a-b)/b if b else 0),("token_cost","max_token_cost_growth",lambda a,b:(a-b)/b if b else 0))
    for metric,limit,delta in checks:
        if metric in current and metric in baseline and limit in limits and delta(float(current[metric]),float(baseline[metric])) > float(limits[limit]): failures.append(f"{metric} regression exceeds policy")
    return failures

def report(args: argparse.Namespace) -> int:
    log_dir=Path(args.log_dir); result=aggregate(log_dir); policy=json.loads(Path(args.policy).read_text())
    baseline_path=log_dir/"inspect-baseline-aggregate.json"; baseline=safe(json.loads(baseline_path.read_text())) if baseline_path.exists() else None
    failures=compare(result,baseline,policy) if baseline else []
    result.update({"baseline_present":bool(baseline),"regression_failures":failures})
    out_json=log_dir/"evaluation-summary.json"; out_md=log_dir/"evaluation-summary.md"; out_json.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    lines=["# Inspect coding evaluation", "", f"Status: **{result.get('status','blocked').upper()}**", "", "This sanitized aggregate excludes prompts, responses, schemas, raw logs, credentials, and headers.", ""]
    for key in ("suite","model","api","reasoning","inbound_dialect","tools_present","tool_count_bucket","streaming","caller_output_cap_field","request_size_bucket","tool_schema_bytes_bucket","completed","partial","skipped","blocked","limit","score","correct","scored","unscored","unscored_error_rate","p95_sample_time_ms","token_cost","wall_seconds"):
        if key in result: lines.append(f"- {key}: {result[key]}")
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
    return 1 if failures else 0

def smoke(args: argparse.Namespace) -> int:
    os.environ.setdefault("EVAL_LIMIT", "2"); args.suite="humaneval"; code=run(args)
    if code: return code
    return report(argparse.Namespace(log_dir=env("EVAL_LOG_DIR","tmp/inspect-evals"), policy=env("EVAL_POLICY","config/evaluation-policy.example.json")))

def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__); subs=parser.add_subparsers(dest="command",required=True)
    r=subs.add_parser("run"); r.add_argument("--suite", choices=sorted(SUITES)); r.set_defaults(func=run)
    q=subs.add_parser("report"); q.add_argument("--log-dir",required=True); q.add_argument("--policy",required=True); q.set_defaults(func=report)
    s=subs.add_parser("smoke"); s.set_defaults(func=smoke)
    args = parser.parse_args()
    return args.func(args)
if __name__ == "__main__": raise SystemExit(main())
