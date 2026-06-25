# Model Group Contracts

Model group contracts are optional deployment config. They describe what a caller-facing group is intended to guarantee while preserving the existing rule that callers request one allowed model group and routing stays inside that group.

## Config Fields

Contracts live at `models.<group>.contract`:

- `display_name` and `caller_visible_notes`: safe descriptive text.
- `intended_workloads`: safe workload labels such as `support_chat`, `agent_coding`, `receipt_ocr`, or `private_gpu`.
- `supported_api_shapes`: `openai_chat`, `openai_responses`, or `anthropic_messages`.
- `required_capabilities`: hard requirements for tools, forced tool choice, structured outputs, input/output modalities, context tokens, and max-token cap safety.
- `quality_floor`: required target tags, validation status, quality score, pass rate, and optional validation age.
- `operational_targets`: error, timeout, p95 latency, and output-throughput thresholds when observations exist.
- `reporting`: enables safe workload and quality-floor buckets in usage/reporting surfaces.

Targets may include:

```yaml
validation:
  status: passed
  workload: support_chat
  validated_at: "2026-06-25"
  quality_score: 0.94
  pass_rate: 0.98
  harness: golden-support-set
  notes: safe non-sensitive note
```

Do not put prompts, images, tool outputs, bearer tokens, token hashes, provider keys, private headers, or full config snippets in validation notes.

## Enforcement Order

1. Authenticate the caller.
2. Check the requested group against the caller allow list.
3. Load only the requested group.
4. Apply normal request eligibility for dialect, tools, modalities, structured outputs, and explicit output caps.
5. Apply the group contract and target validation floor.
6. Run the configured strategy on the remaining targets.

`static`, `weighted`, `failover`, `dynamic_score`, `script`, and `external` all receive the same contract-filtered target list. TypeScript and external policies receive safe contract and target validation metadata, but their decisions are still validated against eligible targets.

## Validation

Startup fails for unsupported API shapes, invalid modalities, negative thresholds, out-of-range scores/pass rates, invalid validation dates, impossible validation statuses, required tags no target has, declared API shapes no target serves, or contracts no target can satisfy.

Use `rtk go test ./cmd/... ./internal/...` after contract edits. For production-bound changes, validate the sample config, the local production snapshot, and the live production config with structured YAML parsing before restart.

## Rollout And Rollback

Roll out on a deployment-defined test group first. Add at least two interchangeable validated targets, then run representative text, tool, image, structured-output, and low-token-cap smokes according to the declared contract. Confirm in usage reports that selected targets stay inside the requested group and that no-eligible failures use safe buckets such as `contract-quality-floor` or `contract-validation-expired`.

Rollback is config-only: remove the `contract` block, relax a specific `quality_floor` or `operational_targets` field, remove stale validation age checks, or restore the previous strategy and weights. Restart/reload with the normal deployment process and rerun the smoke that failed.

## Reporting

Usage rows store scalar contract fields only:

- `contract_present`
- `contract_bucket`
- `contract_failure_reason`
- `contract_workload`
- `target_validation_status`
- `target_validation_workload`
- `target_validation_age_bucket`

Browser admin reports expose contract buckets, contract workloads, and target validation buckets through scalar report APIs. Public docs and customer-facing examples must stay deployment-defined and must not expose private target names from a managed production deployment.
