# LRP read-only relational usage import

Follow-up to issue #15 / #34. Operators can recover explore-tagged routing
metadata from the router's relational usage database without tailing JSONL and
without reconstructing prompts.

## What this imports

`lrp import-usage` opens a **read-only** SQLite path/URI or PostgreSQL DSN and
runs one join across:

- `request_usage` — safe scalars such as request ID, timestamps, group, dialect,
  caller project/environment, selected provider/model, status, latency, token
  counts, and stored request-time cost
- `request_policy_executions` — policy kind, outcome, duration, candidate counts,
  and `class_label` (filter default `lrp:explore`)
- selected `request_attempts` — provider/model/dialect, status, duration, and
  sanitized error class for the selected attempt

The SQL projection is explicit. It never selects `token_id`, caller identity
beyond project/environment, IPs, prompts, tool payloads, encrypted content,
credentials, or token hashes. Connections use SQLite `mode=ro` /
`PRAGMA query_only` or PostgreSQL `default_transaction_read_only=on`, and the
importer rejects mutating statements.

SQLite and PostgreSQL share the same join shape; only parameter placeholders
differ (`?` vs `%s`).

## What this cannot do

Usage and policy rows are metadata. They **cannot** reconstruct messages,
tools, verifier ground truth, or a PII restoration map. Training collection
still requires a governed content join:

1. Export explore metadata:
   ```bash
   lrp import-usage \
     --db "$USAGE_DSN" \
     --out "$LRP_DATA_DIR/explore-meta.ndjson"
   ```
2. Join with an approved content-capture or dataset export:
   ```bash
   lrp collect \
     --usage-db "$USAGE_DSN" \
     --content-capture "$LRP_DATA_DIR/approved-export.ndjson" \
     --approved-content \
     --out "$LRP_DATA_DIR/requests.ndjson"
   ```
   Or pass the metadata file as `--router-log` after `import-usage`.

Passing `--usage-db` or `--router-log` alone records `missing_content` and
writes no request rows.

Optional `--content-ids` on `import-usage` limits written metadata to request
IDs present in a governed content JSONL and reports how many explore IDs still
lack content.

## Evidence boundary

- Synthetic wiring: unit tests build a local SQLite fixture and assert
  explore-only selection, forbidden-column absence, read-only enforcement, and
  collect join behavior.
- Provider-backed evidence stays in the operator boundary: live DSNs, real
  explore traffic, and approved content exports are never committed.

PostgreSQL support uses an optional `psycopg` driver at runtime. SQLite uses the
Python standard library. No live routing activation is authorized by this
feature alone.

See the [operator runbook](LEARNED_ROUTING_POLICY.md) for the full offline
pipeline and [usage DB design](USAGE_DB_DESIGN.md) for the scalar schema
contract.
