# Fixed-model OCR outcome baseline

This synthetic example preregisters an aggregate exact-match gate for an OCR-style workload. Replace the placeholder model IDs and scalar results in an operator-controlled copy. Keep images, prompts, extracted text, and model responses outside this public tree.

Run the checked-in example:

```bash
python3 scripts/fixed_model_outcome_gate.py \
  --preregistration examples/fixed-model-ocr-baseline/preregistration.json \
  --results examples/fixed-model-ocr-baseline/results.example.json \
  --out examples/fixed-model-ocr-baseline/live-output/gate.json
```

The preregistration fixes the workload style, case count, control and candidate
IDs, maximum pass-rate regression, maximum error-rate increase, and optional
latency/cost ratios before results are collected. Use the same cases and scoring
rule for both arms. This example uses exact-match OCR outcomes, but the schema
also accepts `unit` and `tool` workload labels.

The strict results schema accepts exactly two arms and only aggregate counts,
p95 latency, total cost, and identifiers. An unknown field is an error, so
prompt text, image data, extracted text, and model responses cannot be added
accidentally. Exit status is:

- `0`: every preregistered threshold passed;
- `1`: the candidate regressed on one or more thresholds;
- `2`: inputs were unreadable or violated the strict schema.

Run `python3 scripts/fixed_model_outcome_gate_test.py` in CI to verify both the
pass and regression paths. `live-results/` and `live-output/` are ignored
because real evaluation evidence belongs in the deployment's protected
validation boundary. Promotion evidence should pair the scalar gate with the
reviewed dataset/version, scorer identity, provider entitlement, request-shape
smokes, and rollback decision retained in that protected system.
