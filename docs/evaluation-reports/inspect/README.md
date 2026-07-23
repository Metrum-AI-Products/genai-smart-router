# Inspect CI evaluation reports

The protected scheduled and manual Inspect evaluation workflow writes one timestamped subdirectory here for each successful bounded run. Each directory contains only `evaluation-summary.md` and `evaluation-summary.json`, which are sanitized aggregates suitable for source control.

Raw Inspect logs, prompts, model responses, schemas, headers, and credentials remain outside this directory in protected CI artifacts. A report is quality evidence, not an automatic routing-promotion decision.
