# LRP governed human judge audit sample

**Audience:** operators reviewing LLM judge quality before promotion.
**Related issues:** [#29](https://github.com/Metrum-AI-Products/genai-smart-router/issues/29).

Pairwise and absolute LLM judgments are sampled into a **protected** review queue
for human spot-check. Public artifacts and checked-in fixtures must never include
prompts, candidate text, tool payloads, or free-text reasons. Queue rows and
agreement reports carry only scalar identities and outcomes.

## Sampling

Default rate is **2%**, deterministic over `(request_id, target, method, seed)` so
reruns are stable. Only `pairwise_vs_anchor:v1` and `absolute_rubric:v1` rows are
eligible (verifier outcomes already have ground truth).

During judging:

```bash
lrp judge ... --out "$LRP_DATA_DIR/judgments.ndjson" \
  --audit-queue "$LRP_DATA_DIR/human-audit-queue.ndjson" \
  --audit-sample-rate 0.02 \
  --audit-seed "$LRP_AUDIT_SEED"
```

Or from an existing judgments file:

```bash
lrp judge-audit sample \
  --judgments "$LRP_DATA_DIR/judgments.ndjson" \
  --out "$LRP_DATA_DIR/human-audit-queue.ndjson" \
  --rate 0.02 \
  --seed "$LRP_AUDIT_SEED"
```

Queue fields include request/target identity, judge method/quality, optional
swap votes, cache key, and `content_included: false`. Operators keep any
content needed for review in a separate protected store keyed by those IDs.

## Human reviews

Reviewers write protected NDJSON rows (`lrp.human_audit_review.v1`) with:

- `request_id`, `target.provider`, `target.model`
- `human_quality` in `[0, 1]` (pass band is `>= 0.5`, matching pairwise encoding)
- `reviewer_id_hash` (hash only; never an email or raw identity)
- `reviewed_at` UTC `Z` timestamp
- `content_included: false`

Free-text fields such as `notes`, `reason`, `prompt`, or `content` are rejected.

## Agreement report and promotion gate

```bash
lrp judge-audit report \
  --queue "$LRP_DATA_DIR/human-audit-queue.ndjson" \
  --reviews "$LRP_DATA_DIR/human-reviews.ndjson" \
  --out "$LRP_DATA_DIR/human-audit-report.json" \
  --min-agreement 0.8 \
  --min-coverage 0.5 \
  --min-reviews 5
```

The report is scalar-only. Gate failure reasons:

| Reason | Meaning |
|---|---|
| `no_audit_sample` | Empty queue |
| `insufficient_reviews` | Fewer matched reviews than `--min-reviews` |
| `insufficient_coverage` | `reviewed_count / sampled_count` below `--min-coverage` |
| `no_comparable_reviews` | No rows with both human and judge qualities to compare |
| `agreement_below_floor` | Agreement rate below `--min-agreement` (default **0.8**) |

CLI exit status is non-zero when `gate_passed` is false. Treat a failed gate as a
promotion blocker for judge-dependent bundles. Verifier-labeled rows are out of
scope for this gate. Synthetic wiring tests prove sampling and gate math;
provider-backed evidence requires real judge outputs plus protected human
reviews outside the public tree.

## Cross-links

- Extensible verifiers: [LRP_VERIFIERS.md](LRP_VERIFIERS.md)
- Pipeline overview: [LEARNED_ROUTING_POLICY.md](LEARNED_ROUTING_POLICY.md)
- Isolated worker: [`lrp/judge/README.md`](../services/learned-routing-policy/lrp/judge/README.md)
