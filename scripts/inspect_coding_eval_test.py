#!/usr/bin/env python3
"""Contract tests for the opt-in Inspect evaluation wrapper."""
from __future__ import annotations
import importlib.util, json, os, subprocess, sys, tempfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]; SCRIPT=ROOT/"scripts/inspect_coding_eval.py"
SPEC=importlib.util.spec_from_file_location("inspect_coding_eval", SCRIPT); assert SPEC and SPEC.loader
EVAL=importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(EVAL)
def need(ok: bool, message: str) -> None:
    if not ok: raise AssertionError(message)
def main() -> int:
  need(EVAL.number("C")==1.0 and EVAL.number("CORRECT")==1.0 and EVAL.number("I")==0.0 and EVAL.number("INCORRECT")==0.0,"categorical Inspect scores were not normalized")
  sample={"stats":{"total_time":1.25},"model_usage":{"route":{"total_cost":0.003}}}
  need(EVAL.nested_number(sample,("total_time",))==1.25 and EVAL.nested_number(sample,("total_cost",))==0.003 and EVAL.percentile95([10,20,30,40])==40,"Inspect metric extraction failed")
  missing=EVAL.compare({"score":1.0},{"score":1.0},{"regression_thresholds":{"max_p95_sample_time_growth":0.25}})
  need(missing==["p95_sample_time_ms unavailable for regression policy"],f"missing policy metric did not fail closed: {missing}")
  zero_growth=EVAL.compare({"p95_sample_time_ms":1},{"p95_sample_time_ms":0},{"regression_thresholds":{"max_p95_sample_time_growth":0.25}})
  need(zero_growth==["p95_sample_time_ms regression exceeds policy"],f"zero baseline growth did not fail: {zero_growth}")
  probes=subprocess.run(["make","-s","-f",str(ROOT/"Makefile"),"--eval","probe-a: ; @printf '%s\\n' \"$$EVAL_LOG_DIR\"","--eval","probe-b: ; @printf '%s\\n' \"$$EVAL_LOG_DIR\"","probe-a","probe-b"],cwd=ROOT,text=True,capture_output=True)
  need(probes.returncode==0,probes.stderr); probe_dirs=[line for line in probes.stdout.splitlines() if line]
  need(len(probe_dirs)==2 and probe_dirs[0]==probe_dirs[1] and "/tmp/inspect-evals/" not in probe_dirs[0],f"Make targets did not share one isolated evaluation directory: {probe_dirs}")
  with tempfile.TemporaryDirectory() as raw:
    root=Path(raw); bin_dir=root/"bin"; bin_dir.mkdir(); capture=root/"capture.json"
    for name, body in {"docker":"#!/bin/sh\nexit 0\n", "inspect":"#!/bin/sh\nprintf '%s\\n' \"$PWD|$*|$OPENAI_API_KEY\" > \"$CAPTURE\"\nexit 0\n"}.items():
      path=bin_dir/name; path.write_text(body); path.chmod(0o755)
    policy=root/"policy.json"; policy.write_text(json.dumps({"approved_direct_baselines":["vendor/model"],"regression_thresholds":{"max_score_delta":0.05,"max_unscored_error_rate":0.1,"max_p95_sample_time_growth":0.25,"max_token_cost_growth":0.25}}))
    logs=root/"logs"; base=os.environ|{"PATH":str(bin_dir),"CAPTURE":str(capture),"EVAL_MODEL":"team/coding","EVAL_MODEL_KIND":"router-group","EVAL_API":"openai","EVAL_REASONING":"low","EVAL_BASE_URL":"https://router.invalid/v1","EVAL_API_KEY":"not-a-real-secret","OPENAI_API_KEY":"ambient-wrong-key","EVAL_LOG_DIR":str(logs),"EVAL_POLICY":str(policy),"EVAL_LIMIT":"2","EVAL_CONCURRENCY":"1"}
    done=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env=base,text=True,capture_output=True)
    need(done.returncode==0, done.stderr); invoked=capture.read_text()
    need(str(ROOT) not in invoked.split("|",1)[0], "Inspect ran in repository rather than scratch directory")
    suite_logs=logs/"humaneval"
    need("inspect_evals/humaneval" in invoked and "--limit 2" in invoked and "--model openai/team/coding" in invoked and "--model-base-url https://router.invalid/v1" in invoked and "reasoning_effort=low" in invoked and f"--log-dir {suite_logs}" in invoked and invoked.rstrip().endswith("|not-a-real-secret"), invoked)
    status=json.loads((suite_logs/"inspect-eval-status.json").read_text()); need(status["status"]=="completed" and status["model_kind"]=="router-group" and status["api"]=="openai" and status["reasoning"]=="low" and status["wall_seconds"] >= 0 and status["completed"],status)
    bigcodebench=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","bigcodebench"],env=base,text=True,capture_output=True)
    need(bigcodebench.returncode==0 and "inspect_evals/bigcodebench" in capture.read_text(),"BigCodeBench task was not qualified")
    need((logs/"humaneval"/"inspect-eval-status.json").exists() and (logs/"bigcodebench"/"inspect-eval-status.json").exists(),"suite runs overwrote each other's status")
    direct=base|{"EVAL_MODEL":"vendor/model","EVAL_MODEL_KIND":"direct-baseline","EVAL_LOG_DIR":str(root/"direct-logs"),"CAPTURE":str(root/"direct.txt")}
    direct_run=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env=direct,text=True,capture_output=True)
    need(direct_run.returncode==0 and "--model openai/vendor/model" in (root/"direct.txt").read_text(),"direct baseline was not explicitly provider-qualified")
    unapproved=base|{"EVAL_MODEL":"unapproved/model","EVAL_MODEL_KIND":"direct-baseline"}
    unapproved_run=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env=unapproved,text=True,capture_output=True)
    need(unapproved_run.returncode==2 and "approved_direct_baselines" in unapproved_run.stderr,"unapproved direct baseline was not blocked")
    invalid=base|{"EVAL_MODEL_KIND":"unknown"}
    invalid_run=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env=invalid,text=True,capture_output=True)
    need(invalid_run.returncode==2 and "EVAL_MODEL_KIND" in invalid_run.stderr,"invalid model kind did not block")
    ci_missing=subprocess.run([sys.executable,str(SCRIPT),"ci-full"],env=base,text=True,capture_output=True)
    need(ci_missing.returncode==2 and "EVAL_BASELINE_HUMANEVAL_JSON" in ci_missing.stderr,"full CI lane did not require protected baselines")
    coverage_leak="coverage-response-must-not-persist"
    (suite_logs/"inspect-aggregate.json").write_text(json.dumps({"status":"completed","completed":True,"score":0.8,"correct":8,"scored":10,"unscored":0,"p95_sample_time_ms":100,"token_cost":1,"inbound_dialect":"openai_chat","tools_present":False,"tool_count_bucket":"0","streaming":False,"caller_output_cap_field":"max_tokens","request_size_bucket":"small","tool_schema_bytes_bucket":"0","reasoning_tokens":0,"reasoning_attempt_count":2,"reasoning_successful_attempt_count":2,"reasoning_reported_attempt_count":1,"reasoning_provider_model_dialect_coverage":[{"provider":"safe-provider","model":"safe-model","dialect":"openai-chat","attempts":2,"reported_attempts":1,"reasoning_tokens":0,"response":coverage_leak,"headers":{"authorization":coverage_leak}}],"authorization":"leak","response":"leak"}))
    usage=json.dumps({"stored_request_time_cost_usd":1})
    report=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],env=base|{"EVAL_ROUTER_USAGE_JSON":usage},text=True,capture_output=True)
    need(report.returncode==0,report.stderr); rendered=(suite_logs/"evaluation-summary.md").read_text(); summary_json=(suite_logs/"evaluation-summary.json").read_text(); need("leak" not in rendered and coverage_leak not in rendered and coverage_leak not in summary_json and "openai_chat" in rendered and "caller_output_cap_field" in rendered and "1 reported / 2 upstream attempts (0 tokens; 2 successful)" in rendered and "safe-provider" in rendered,"secret/raw content leaked or reasoning coverage missing")
    baseline_env=base|{"EVAL_ROUTER_USAGE_JSON":usage,"EVAL_BASELINE_JSON":json.dumps({"score":0.9,"unscored_error_rate":0,"p95_sample_time_ms":100,"token_cost":1})}
    baseline_failed=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],env=baseline_env,text=True,capture_output=True)
    need(baseline_failed.returncode==1,"protected baseline JSON did not enforce regression policy")
    empty_baseline=base|{"EVAL_ROUTER_USAGE_JSON":usage,"EVAL_BASELINE_JSON":"{}"}
    empty_failed=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],env=empty_baseline,text=True,capture_output=True)
    need(empty_failed.returncode==1,"empty protected baseline returned success")
    ci_reports=root/"ci-reports"; ci_env=os.environ|{"EVAL_SAVE_CI_REPORT":"true","EVAL_CI_REPORT_DIR":str(ci_reports),"EVAL_CI_REPORT_TIMESTAMP":"20260723T191500Z-123-1"}
    saved=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],env=ci_env|{"EVAL_ROUTER_USAGE_JSON":usage},text=True,capture_output=True)
    need(saved.returncode==0,saved.stderr); saved_dir=ci_reports/"20260723T191500Z-123-1"; need((saved_dir/"evaluation-summary.md").exists() and coverage_leak not in (saved_dir/"evaluation-summary.json").read_text(),"timestamped CI report leaked coverage content or is missing")
    (suite_logs/"inspect-aggregate.json").write_text(json.dumps({"reasoning_tokens":9,"reasoning_attempt_count":1,"reasoning_successful_attempt_count":1,"reasoning_reported_attempt_count":1}))
    complete=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],text=True,capture_output=True)
    need(complete.returncode==0 and "9 (complete coverage)" in (suite_logs/"evaluation-summary.md").read_text(),"complete reasoning coverage was not rendered")
    (suite_logs/"inspect-aggregate.json").write_text(json.dumps({"reasoning_attempt_count":1,"reasoning_successful_attempt_count":1,"reasoning_reported_attempt_count":0}))
    absent=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],text=True,capture_output=True)
    need(absent.returncode==0 and "reasoning tokens: not reported" in (suite_logs/"evaluation-summary.md").read_text(),"absent reasoning coverage was not rendered")
    (suite_logs/"inspect-aggregate.json").write_text(json.dumps({"score":0.8,"unscored_error_rate":0,"p95_sample_time_ms":100,"token_cost":1}))
    (suite_logs/"inspect-baseline-aggregate.json").write_text(json.dumps({"score":0.9,"unscored_error_rate":0,"p95_sample_time_ms":100,"token_cost":1}))
    failed=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],env=base|{"EVAL_ROUTER_USAGE_JSON":usage},text=True,capture_output=True)
    need(failed.returncode==1,"score regression did not fail")
    (suite_logs/"inspect-baseline-aggregate.json").unlink(missing_ok=True)
    (suite_logs/"inspect-aggregate.json").write_text(json.dumps({"status":"completed","completed":True,"partial":True,"skipped":False,"blocked":False}))
    incomplete=subprocess.run([sys.executable,str(SCRIPT),"report","--log-dir",str(logs),"--suite","humaneval","--policy",str(policy)],text=True,capture_output=True)
    need(incomplete.returncode==1,"incomplete aggregate returned success")
    blocked=subprocess.run([sys.executable,str(SCRIPT),"run","--suite","humaneval"],env={"PATH":str(bin_dir)},text=True,capture_output=True)
    need(blocked.returncode==2 and "blocked" in blocked.stderr,blocked.stderr)
  print("inspect coding evaluation tests passed"); return 0
if __name__=="__main__": raise SystemExit(main())
