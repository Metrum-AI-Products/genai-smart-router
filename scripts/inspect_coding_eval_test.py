#!/usr/bin/env python3
"""Contract tests for the opt-in Inspect evaluation wrapper."""
from __future__ import annotations
import json, os, subprocess, sys, tempfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]; SCRIPT=ROOT/"scripts/inspect_coding_eval.py"
def need(ok: bool, message: str) -> None:
    if not ok: raise AssertionError(message)
def main() -> int:
  with tempfile.TemporaryDirectory() as raw:
    root=Path(raw); bin_dir=root/"bin"; bin_dir.mkdir(); capture=root/"capture.json"
    for name, body in {"docker":"#!/bin/sh\nexit 0\n", "inspect":"#!/bin/sh\nprintf '%s\\n' \"$PWD|$*\" > \"$CAPTURE\"\nexit 0\n"}.items():
      path=bin_dir/name; path.write_text(body); path.chmod(0o755)
    logs=root/"logs"; base=os.environ|{"PATH":str(bin_dir),"CAPTURE":str(capture),"EVAL_MODEL":"router-group","EVAL_API":"openai","EVAL_REASONING":"low","EVAL_BASE_URL":"https://router.invalid/v1","EVAL_API_KEY":"not-a-real-secret","EVAL_LOG_DIR":str(logs),"EVAL_LIMIT":"2","EVAL_CONCURRENCY":"1"}
    done=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env=base,text=True,capture_output=True)
    need(done.returncode==0, done.stderr); invoked=capture.read_text()
    need(str(ROOT) not in invoked.split("|",1)[0], "Inspect ran in repository rather than scratch directory")
    need("--limit 2" in invoked and "--model openai/router-group" in invoked and "--model-base-url https://router.invalid/v1" in invoked and "reasoning_effort=low" in invoked and f"--log-dir {logs}" in invoked, invoked)
    status=json.loads((logs/"inspect-eval-status.json").read_text()); need(status["status"]=="completed" and status["api"]=="openai" and status["reasoning"]=="low" and status["wall_seconds"] >= 0 and status["completed"],status)
    (logs/"inspect-aggregate.json").write_text(json.dumps({"status":"completed","score":0.8,"correct":8,"scored":10,"unscored":0,"p95_sample_time_ms":100,"token_cost":1,"inbound_dialect":"openai_chat","tools_present":False,"tool_count_bucket":"0","streaming":False,"caller_output_cap_field":"max_tokens","request_size_bucket":"small","tool_schema_bytes_bucket":"0","authorization":"leak","response":"leak"}))
    policy=root/"policy.json"; policy.write_text(json.dumps({"regression_thresholds":{"max_score_delta":0.05,"max_unscored_error_rate":0.1,"max_p95_sample_time_growth":0.25,"max_token_cost_growth":0.25}}))
    report=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--policy",str(policy)],text=True,capture_output=True)
    need(report.returncode==0,report.stderr); rendered=(logs/"evaluation-summary.md").read_text(); need("leak" not in rendered and "openai_chat" in rendered and "caller_output_cap_field" in rendered,"secret/raw content leaked or request-shape missing")
    ci_reports=root/"ci-reports"; ci_env=os.environ|{"EVAL_SAVE_CI_REPORT":"true","EVAL_CI_REPORT_DIR":str(ci_reports),"EVAL_CI_REPORT_TIMESTAMP":"20260723T191500Z-123-1"}
    saved=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--policy",str(policy)],env=ci_env,text=True,capture_output=True)
    need(saved.returncode==0,saved.stderr); need((ci_reports/"20260723T191500Z-123-1"/"evaluation-summary.md").exists(),"timestamped CI Markdown report missing")
    (logs/"inspect-baseline-aggregate.json").write_text(json.dumps({"score":0.9,"unscored_error_rate":0,"p95_sample_time_ms":100,"token_cost":1}))
    failed=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--policy",str(policy)],text=True,capture_output=True)
    need(failed.returncode==1,"score regression did not fail")
    blocked=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env={"PATH":str(bin_dir)},text=True,capture_output=True)
    need(blocked.returncode==2 and "blocked" in blocked.stderr,blocked.stderr)
  print("inspect coding evaluation tests passed"); return 0
if __name__=="__main__": raise SystemExit(main())
