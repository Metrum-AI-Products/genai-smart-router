# LRP extensible isolated verifiers

**Audience:** operators running the offline LRP training pipeline.
**Related issues:** [#37](https://github.com/metrum-ai/router/issues/37).

Deterministic verifiers beat LLM judges when ground truth exists. GenAI Smart
Router LRP runs them **only** inside the bubblewrap isolated worker described in
[`services/learned-routing-policy/lrp/judge/README.md`](../services/learned-routing-policy/lrp/judge/README.md).
There is no host-process fallback, no dataset-supplied shell, and no network or
secret passthrough into the worker.

## Allowlisted contracts

Each request may carry:

```json
{
  "verifier": {
    "kind": "exact|regex|json_schema|pytest|sql_result|plugin|none",
    "version": "<kind>.v1",
    "spec": {}
  }
}
```

| Kind | Version | Spec | Candidate `content` |
|---|---|---|---|
| `exact` | `exact.v1` | `expected` string | Exact string match |
| `regex` | `regex.v1` | `pattern` | Full-match regex |
| `json_schema` | `json_schema.v1` | `schema` object (no remote `$ref`) | JSON document |
| `pytest` | `pytest.v1` | `tests` string | Written to `solution.py` under `/work` |
| `sql_result` | `sql_result.v1` | `expected_rows` (list of objects); optional `columns`, `ignore_row_order` | JSON rows or `{"rows":[...],"columns":[...]}` |
| `plugin` | `plugin.v1` | `plugin_id` plus optional `params` | Interpreted only by an allowlisted plugin |

Unknown kinds, versions, spec fields, or plugin IDs produce missing evidence
(`quality: null`, `detail.unsupported: true`), never host execution.

## SQL-result verifier

Use when the workload answer is a tabular query result. Compare normalized row
tuples after optional column projection. With `ignore_row_order: true`, row
multisets are compared; otherwise order is significant. Example:

```json
{
  "kind": "sql_result",
  "version": "sql_result.v1",
  "spec": {
    "expected_rows": [{"id": 1, "name": "alpha"}, {"id": 2, "name": "beta"}],
    "columns": ["id", "name"],
    "ignore_row_order": true
  }
}
```

## Governed plugin verifiers

Plugins are **not** dataset code. Allowlisted implementations ship in
`lrp/judge/plugins_runtime.py` and are concatenated into the isolated `-c`
payload with the worker. Current IDs:

| Plugin ID | Params | Behavior |
|---|---|---|
| `contains_v1` | `needle` (string) | Substring presence |
| `json_equals_v1` | `expected` (JSON value) | Parsed content deep-equals `expected` |
| `numeric_equals_v1` | `expected` number; optional `abs_tol` | Float compare with absolute tolerance |

To add a product acceptance check, extend the allowlist and runtime in a reviewed
change, rebuild verifier rootfs evidence, and re-run the mandatory sandbox gate.
Do not mount host modules or accept callable blobs from a dataset.

## Isolation and evidence

Synthetic wiring tests cover contract validation, orchestration, and plugin
bodies without claiming production isolation. Provider-backed or operator CI
evidence requires `LRP_TEST_ROOTFS` and `LRP_REQUIRE_SANDBOX_TESTS=1` as in the
judge README. Passing synthetic tests alone does not authorize live routing
activation.

## Cross-links

- Human judge audit sample and agreement gates: [LRP_HUMAN_JUDGE.md](LRP_HUMAN_JUDGE.md)
- Pipeline overview: [LEARNED_ROUTING_POLICY.md](LEARNED_ROUTING_POLICY.md)
- Worker/rootfs runbook: [`lrp/judge/README.md`](../services/learned-routing-policy/lrp/judge/README.md)
