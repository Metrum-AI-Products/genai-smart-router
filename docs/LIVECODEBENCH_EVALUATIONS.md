# LiveCodeBench release_v6 evaluation

Issue #569 adds an opt-in, deterministic contract for the official
LiveCodeBench `code_generation_lite` loader. It is separate from the Inspect
evaluation lane and is never run by `make`, `make test`, builds, packages, or
deployments.

The checked-in [contract](../evaluators/livecodebench/contract.json) pins the
official LiveCodeBench revision, `datasets` version, `release_v6`, a stable
sampling seed, exactly 40 distinct task IDs, one generation at temperature
0.2, the output cap, request timeout, and the official scorer identity. The
contract validator runs before any inference. A revision mismatch, a changed
`datasets` version, an unavailable official loader, duplicate IDs, or fewer
than 40 tasks exits with status 2 and makes no model request.

## Prepare and validate

Use an ignored disposable directory. The official checkout's committed
`uv.lock` supplies its remaining dependency graph; the repository contract
additionally requires the exact `datasets` version listed in
`evaluators/livecodebench/requirements.txt`.

```sh
git clone https://github.com/LiveCodeBench/LiveCodeBench.git /protected/livecodebench
git -C /protected/livecodebench checkout 28fef95ea8c9f7a547c8329f2cd3d32b92c1fa24
cd /protected/livecodebench
uv venv --python 3.11
uv sync --frozen
uv pip install -r /path/to/genai-smart-router/evaluators/livecodebench/requirements.txt

cd /path/to/genai-smart-router
LCB_ROOT=/protected/livecodebench \
LCB_PYTHON=/protected/livecodebench/.venv/bin/python \
make livecodebench-validate
```

The command emits only a JSON aggregate with the selected task count and zero
inference/scoring counts. It intentionally never prints prompts, task IDs,
responses, credentials, headers, or raw scorer data.

## Runner contract and evidence

For a bounded approved run, create an owner-only (`0600`) command file that
reads one problem body from standard input and writes only its generation to
standard output. It receives credentials through its protected environment,
not through arguments or checked-in configuration. The runner invokes that
file once per selected task, calls the official
`lcb_runner.evaluation.codegen_metrics` scorer named by the contract, and
retains prompts, generations, and per-task scores only in process memory. It
rejects blank generation strings before scoring.

```sh
LCB_ROOT=/protected/livecodebench \
LCB_PYTHON=/protected/livecodebench/.venv/bin/python \
LCB_RUNNER_COMMAND_FILE=/protected/lcb-router-runner.sh \
make livecodebench-run
```

The only durable result is the sanitized aggregate:

```json
{"status":"completed","release_version":"release_v6","selected":40,"completed":40,"scored":40,"errors":0,"pass_at_1":0.0}
```

`pass_at_1` is a numeric aggregate; its example value is not a benchmark
result. A real endpoint run needs separate bounded-spend approval and protected
runtime credentials. Do not commit its prompts, model output, task IDs, raw
scorer reports, headers, or credentials. Roll back by removing the opt-in
runner invocation; no router configuration or deployed route changes are
needed.

The runner verifies the pinned official scorer's three-item result contract
(`metrics`, per-instance `results`, and metadata) before deriving each score.
For its one-task/one-generation call, exactly one extracted boolean is required.
Any shape change fails closed without emitting raw scorer data; the offline
regression exercises that pinned shape across all 40 selected tasks.

Run the offline regression before changing this lane:

```sh
python3 scripts/livecodebench_eval_test.py
```
