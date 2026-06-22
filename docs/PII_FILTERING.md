# PII Filtering Design Notes

GenAI Smart Router supports model-group-level PII filtering through `models.<group>.pii_filter`.

## Design

- Filtering is configured per model group.
- Rules are deployment-owned regular expressions with a typed `placeholder_prefix`.
- The router applies filtering after caller auth, model access, and admission checks, and before target selection, cache-key generation, upstream encoding, routing-policy inputs, and provider calls.
- Cache keys are computed from the redacted request.
- Placeholder mappings are kept in memory for the request lifecycle only.
- Restored responses are never stored in the shared response cache; cache entries keep redacted upstream text.
- Tool passthrough requests redact the raw request body used for upstream passthrough while preserving tool-call IDs, tool schemas, model names, roles, image URLs by default, and provider metadata.

## Modes

- `redact_only`: replace configured matches with placeholders before upstream calls.
- `redact_and_restore`: redact before upstream and restore placeholders in downstream text responses.
- `fail_on_match`: reject matching requests before target selection and upstream calls.

`fail_on_match: true` forces blocking behavior regardless of mode.

## Persistence

Usage JSONL and DB rows store only safe scalar metadata:

- `pii_filter_applied`
- `pii_filter_mode`
- `pii_filter_replacements`
- `pii_filter_rule_count`

Do not store raw matched values, regex capture groups, placeholder-to-original mappings, raw prompts, raw images, raw tool outputs, bearer tokens, provider keys, or token hashes in usage rows, diagnostics rows, metrics, or ordinary logs.

If an enterprise needs durable request/response content capture, use the governed content-capture design separately. Do not turn diagnostics or PII filtering into accidental content archives.

## Validation

Before rollout:

1. Validate config startup rejects invalid regexes, missing placeholder prefixes, duplicate rule names, and negative replacement limits.
2. Smoke OpenAI Chat, OpenAI Responses, and Anthropic Messages shapes.
3. Smoke tool-result text when the group supports tools.
4. Confirm upstream receives placeholders, not raw matched values.
5. Confirm usage DB, JSONL logs, diagnostics, and metrics contain only safe metadata.
6. Confirm cached redacted responses restore to the current request's placeholders and do not leak previous caller values.

## Limitations

Regex filtering is not complete PII detection. Customer deployments should review each expression, set `max_replacements_per_request`, and use external DLP/privacy services for high-assurance detection. Keep external services trusted and document whether filtering happens before or after any deployment-owned routing policy.
