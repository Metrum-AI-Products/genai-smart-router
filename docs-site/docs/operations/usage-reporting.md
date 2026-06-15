---
title: Usage Reporting
---

# Usage Reporting

Smart LLM Router records durable usage data for cost management, auditability, and model-group evaluation.

`router-usage-report` is an Enterprise Edition administrative CLI. It is intended for platform administrators and is run from a secure server console, deployment host shell, or controlled admin workstation with access to the usage database. It is not exposed through the public browser documentation site as an interactive tool.

<div class="contactBanner">
  <p>For dashboards, reports, or evaluation design, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Markdown Report

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out usage-24h.md
```

## Report Dimensions

Reports include:

- Calls, errors, status codes, latency, and upstream attempts.
- Input tokens, output tokens, total tokens, and throughput.
- Usage by public router token ID, user, project, and environment.
- Usage by caller IP and hour.
- Usage by router model group.
- Usage by external provider and model.
- Cache hits, misses, bypasses, occupancy, and hit rate.
- Streaming and non-streaming request counts.

## Filtering

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-project harbor-algotune-pca \
  --resolved-group big-coder \
  --client codex \
  --out harbor-big-coder-codex.md
```

Reports use public token IDs and aggregated usage fields. They do not expose raw router tokens or raw provider API keys.
