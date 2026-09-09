# Fixed-model OCR outcome baseline

This synthetic example preregisters an aggregate exact-match gate for an OCR-style workload. Replace the placeholder model IDs and scalar results in an operator-controlled copy. Keep images, prompts, extracted text, and model responses outside this public tree.

Run the checked-in example:

```bash
python3 scripts/fixed_model_outcome_gate.py \
  --preregistration examples/fixed-model-ocr-baseline/preregistration.json \
  --results examples/fixed-model-ocr-baseline/results.example.json \
  --out examples/fixed-model-ocr-baseline/live-output/gate.json
```

The strict schema accepts only aggregate counts, latency, cost, and identifiers. An unknown field is an error, and a candidate quality regression beyond the preregistered tolerance exits nonzero. `live-results/` and `live-output/` are ignored because real evaluation evidence belongs in the deployment's protected validation boundary.
