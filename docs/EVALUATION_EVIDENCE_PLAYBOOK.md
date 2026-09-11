# Evaluation Evidence Playbook

Use this playbook when a customer, evaluator, or operator says a routed group "felt worse" than a fixed model, previous policy, or named provider. Treat the report as a useful bug signal, then convert it into repeatable evidence before changing routing policy.

## Support Response Template

Suggested response:

> Thanks for the report. A single bad answer can identify a possible issue, but routing changes need repeatable evidence so we do not optimize for one anecdote and regress the workload distribution. Please send safe evaluation artifacts for the affected workload: task IDs or anonymized cases, expected outcomes or rubric, client/agent version, model group, fixed-model or prior-policy control, timestamps, request IDs if available, and the metric you want optimized. Do not send raw prompts, raw images, provider keys, bearer tokens, token hashes, raw tool outputs, private repository contents, or full production config through an ungoverned support path.

Then propose a comparison:

1. hold workload, client, agent, tools, prompt shape, seed policy, versions, token caps, and timeouts constant;
2. change only the model selection, such as routed group versus fixed model;
3. report task-level outcomes, cost, latency, reliability, compatibility, and uncertainty where valid;
4. decide whether to promote, hold, split, rollback, or collect more evidence.

## Safe Artifacts To Request

Request only artifacts that can be shared safely:

| Artifact | Use |
|---|---|
| Evaluation window | Joins external results to router usage by timestamp. |
| Caller/project/client/model group | Narrows usage reports without exposing raw tokens. |
| Request IDs or run labels | Joins router rows to Harbor or external evaluation rows. |
| Task IDs and expected outcomes | Supports repeatable scoring without raw private content. |
| Rubric or verifier version | Explains how pass/fail or reward was assigned. |
| Client, agent, SDK, and app versions | Controls for client behavior changes. |
| Fixed-model or previous-policy control | Provides a baseline. |
| Config version or safe routing/config summary | Describes candidate group, target classes, weights, and capability filters without provider keys or full config. |
| Aggregate usage report | Shows selected provider/model, tokens, cost, latency, throughput, attempts, fallbacks, and errors. |

Never request provider keys, bearer tokens, router token hashes, raw prompts, raw images, raw tool outputs, private repo contents, private hostnames, full production config, signing keys, or license private material unless a governed support path explicitly allows that content.

## Experiment Template

Record these fields before the run:

| Field | Required note |
|---|---|
| Hypothesis | What the router group should match or improve. |
| Workload/task set | Dataset, task IDs, app flow, or acceptance suite. |
| Metric and threshold | Pass/reward/resolution/business metric plus cost, latency, and reliability thresholds. |
| Control | Fixed model or previous routing policy. |
| Candidate | Router group, policy label, config version, or safe routing/config summary. |
| Versions | Router build, client/agent/app/evaluator versions, provider entitlement state. |
| Request shape | API dialect, tools, images, structured output, reasoning controls, token caps, and timeouts. |
| Attempts/seeds | Number of attempts, seed policy, retry policy, and independence assumptions. |
| Comparison method | Distribution, confidence interval, paired comparison, bootstrap, or other justified method. |
| Decision rule | Promote, hold, split, rollback, or gather more evidence. |

## Harbor As One Agent-Eval Harness

Harbor is an open-source, sandboxed agent-evaluation framework that can run coding agents against reproducible tasks and score the produced artifact with a verifier. Use it when the workload depends on the whole agent loop: repository navigation, tool use, file edits, terminal commands, and final artifact quality. Public Terminal-Bench tasks are one useful benchmark family that uses the Harbor task format/harness, but they do not replace customer-specific acceptance tests.

For router-versus-fixed comparisons, hold the task, agent, agent version, tools, seed policy, token caps, timeouts, and verifier constant. Change only the model endpoint/group, then compare outcome, cost, latency, selected upstream provider/model, fallback behavior, cache behavior, and token usage. The public source-dated example is `docs-site/docs/evaluation/harbor-case-study.mdx`; keep `docs/harbor-case-study.md` aligned when updating it.

Claim boundary: a Harbor result proves only the specific task set, agent/client version, router build, model group, run window, and scoring method tested. It is not a universal model ranking, and it should be interpreted with repeated runs and confidence intervals when the sample design supports them.

## Joining Router Reports With External Evaluations

Use safe scalar dimensions to join router data with Harbor or other harness output:

- time window: UTC `[from,to)` around the run;
- caller user, caller project, environment, client, or public token ID;
- requested model group and resolved group;
- request ID, run label, task ID, or trace correlation ID when available;
- selected provider/model/dialect;
- status, attempts, fallback count, error type, timeout, and cancellation;
- input/output/image token counts, cache fields, request-time cost, upstream-reported billed cost, latency, TTFB, duration, and throughput.

Example filtered report:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --from <run-start-utc> \
  --to <run-end-utc> \
  --caller-project <project> \
  --client <client> \
  --resolved-group <model-group> \
  --out /app/logs/eval-<run-label>.md
```

For Harbor, join by run matrix, client, model group, task ID, seed, attempt, timestamps, and request IDs when the client records them. Current production Harbor runs should use the reusable Harbor caller token and separate results by run labels and report filters rather than creating one token per `{agent, model_group}`.

For non-Harbor evaluations, keep the external result table normalized enough to join on task ID, seed, attempt, app cohort, and request ID. Store raw private content only in a governed customer system, not in router reports.

## Outcome Gate Command

Use `scripts/evaluate_workload_gate.py` to turn Harbor or equivalent workload results into a deterministic pass/fail artifact. The gate accepts Harbor `results.tsv` or JSON result rows, plus an optional safe usage-report JSON export. It reports pass rate, Wilson confidence interval, reward, p95 latency, cost per successful task, error rate, fallback rate, selected upstream distribution, outcome dimensions, failure classes, and request IDs for correlation.

Smoke mode (default) may use example matrices with empty `fixed_model_controls`. **Promotion mode** (`--promotion` or `"mode": "promotion"`) requires nonempty applicable `fixed_model_controls`; missing direct/fixed baseline, missing required task, or insufficient `min_attempts_per_cell` yields status `blocked` (exit `2`), not a silent pass. Checked-in example matrices must not be able to produce a promotion pass while controls remain empty.

Mock CI check:

```bash
python3 scripts/evaluate_workload_gate_test.py
python3 tests/harbor/p1/run_offline_tests.py
```

Example Harbor gate:

```bash
python3 scripts/evaluate_workload_gate.py \
  --matrix examples/harbor-algotune-pca/workload_gate_matrix.json \
  --results examples/harbor-algotune-pca/runs/<CASE_ID>/results.tsv \
  --usage-json examples/harbor-algotune-pca/reports/<CASE_ID>/usage-rows.json \
  --out-json examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.json \
  --out-md examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.md
```

Promotion example (after copying the matrix and filling `fixed_model_controls`):

```bash
python3 scripts/evaluate_workload_gate.py --promotion \
  --matrix /path/to/promotion_matrix.json \
  --results examples/harbor-algotune-pca/runs/<CASE_ID>/results.tsv \
  --out-json examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.json \
  --out-md examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.md
```

The matrix must define the reward/verifier, clients, model groups, attempts/seeds, fixed-model or previous-policy controls where practical, pass/fail criteria, cost and latency thresholds, and rollback criteria. A gate failure should block promotion unless the reviewer explicitly records why the failure is outside the changed route scope. Generated summaries should state evidence date, build, matrix completeness, and limitations; a prior passing run must not override a newer failing cell.

For a second, non-agent baseline, use the preregistered OCR-style example in
`examples/fixed-model-ocr-baseline/`. It compares one candidate with one named
fixed-model control using aggregate counts, p95 latency, and total cost:

```bash
python3 scripts/fixed_model_outcome_gate.py \
  --preregistration examples/fixed-model-ocr-baseline/preregistration.json \
  --results examples/fixed-model-ocr-baseline/results.example.json
```

The schema rejects unknown fields, including prompt or response fields. A
quality regression beyond the preregistered tolerance exits nonzero. Copy the
example into a protected validation environment for live runs; the example's
`live-results/` and `live-output/` paths are ignored and must not be committed.

## Metrics To Inspect

Report:

- primary outcome: pass rate, reward, resolution, extraction accuracy, acceptance-test result, or business metric;
- cost: request-time input, output, image, cache, calculated, and upstream-reported cost fields;
- latency: downstream latency, upstream duration, TTFB, output tokens/sec, and total tokens/sec;
- reliability: retries, fallbacks, provider errors, `no-eligible-target`, timeouts, cancellations, and agent errors;
- compatibility: API shape, tool dialect, modality, structured-output behavior, reasoning/thinking controls, and cap forwarding;
- distribution: task-level and seed-level rows, not only an aggregate;
- uncertainty: confidence interval or other uncertainty estimate when the sample design supports it.

## Rollback Or Route-Weight Change

Make routing changes only when the evidence maps to the model-group contract:

| Evidence | Action |
|---|---|
| Router group beats control | Promote access or increase weight; keep monitoring and rollback criteria. |
| Router ties control at lower cost or latency | Promote cautiously and watch distribution-level regressions. |
| Router underperforms | Keep fixed model, lower unsafe target weight, add stronger target weight, or create a workload-specific group. |
| Mixed results | Split by workload category, request shape, model group, caller/project, or policy label. |
| Inconclusive | Add tasks/seeds, improve rubric, run shadow mode, or delay rollout. |

For config-only rollback, restore the timestamped config backup, restart the router, verify `/readyz`, confirm local and remote config hashes when applicable, and rerun the failing smoke or evaluation slice. For a targeted weight change, patch the group with structured YAML, validate config, run router-level smokes for affected request shapes, and regenerate the usage/report excerpt for the rollout window.

## Same-Group Different-Upstream Proof

Offline gate: `make proof-routing` runs
`TestProofRoutingSameGroupDifferentUpstream` against mock upstreams and compares
projected `/admin/reports/api/request-evidence` fields to
`testdata/proof/expected.json`. The committed golden shows one requested group
(`proof-routing`) selecting `cheap-summarizer` for the trivial summarize fixture
and `validated-coder` for the code/tool fixture under `dynamic_score`.

Keep the public curl example labeled as illustrative against a pre-warmed
deployment. Cold-start weights and default affinity can pin both requests; the
Make target is the reproducible proof.

## Public Playbook

The public customer-facing version is in `docs-site/docs/evaluation/prove-router-quality.md`. Keep it generic, placeholder-safe, and clear that Harbor is one harness rather than a requirement.
