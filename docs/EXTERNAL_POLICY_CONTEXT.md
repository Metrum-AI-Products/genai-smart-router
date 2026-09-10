# External Policy Conversation Key, Feedback, and Verifier Hints

Operator runbook for the three external-policy context extensions introduced for
issues #21, #22, and #23. Caller-facing product docs live in
`docs-site/docs/configuration/external-policy-context.md`. Keep both pages
aligned when changing payload fields, auth, or retention behavior.

For the base external-policy contract see [EXTERNAL_ROUTING_POLICY.md](EXTERNAL_ROUTING_POLICY.md).
For Learned Routing Policy serving that consumes these fields see
[LEARNED_ROUTING_POLICY.md](LEARNED_ROUTING_POLICY.md).

## Conversation key (`context.conversationKey`)

Every external-policy request includes a stable pseudonymous `context.conversationKey`
and a safe `context.conversationKeySource` of `conversation_id`, `turn0_hash`, or
`anonymous`.

Derivation:

1. If the caller supplies a deployment-defined conversation id in request
   `metadata` (`conversation_id`, `conversationId`, `session_id`, `sessionId`, or
   `metrum_conversation_id`), the router scopes by
   `salt || caller_id || group || id:<sanitized_id>`.
2. Otherwise it hashes the first user-turn text locally and scopes by
   `salt || caller_id || group || turn0:<sha256>`. The raw text is never placed on
   the policy payload unless `include_request: true` is separately approved.
3. Keys never incorporate raw bearer tokens, token hashes, or full request text.

Collision and multi-turn semantics:

- Scope is caller + model group (+ optional deployment salt). The same
  conversation id from two callers yields different keys.
- Explicit conversation ids keep the key stable across turns even when later
  user text changes.
- First-turn hashing is stable for Chat/Anthropic multi-turn histories that
  retain the original first user message. Stateless Responses turns without an
  explicit conversation id mint a new key each call.
- The key is a 128-bit truncated SHA-256 hex digest with a `ck_` prefix.
  Birthday collisions are theoretically possible across large populations; treat
  the key as a routing pin identity, not an access-control boundary.

Optional config:

```yaml
external_policy:
  conversation_key:
    salt: deployment-owned-salt
```

## Completion feedback (`external_policy.feedback`)

Opt-in authenticated POST after the router finishes a request. Default URL is
the policy URL with a final `/feedback` path segment (`.../route` becomes
`.../feedback`). The callback inherits policy `allow_hosts` / headers when
feedback-specific values are omitted.

```yaml
external_policy:
  feedback:
    enabled: true
    # url: https://routing-policy.internal.example/feedback
    timeout_ms: 500
    max_retries: 2
    retry_backoff_ms: 100
    max_request_bytes: 4096
    on_delivery_failure: log   # or ignore
```

Payload fields (schema `external_policy.feedback.v1`): request ID, group,
conversation key, HTTP status, optional error class, actual selected
provider/model/dialect, stored usage token counts, stored cost USD scalars,
latency/TTFB, optional class label, and completion timestamp.

Never sent: prompts, message bodies, tool schemas, tool outputs, images,
credentials, token hashes, provider keys, policy headers, or full config.

Delivery behavior:

- Retries use linear backoff up to `max_retries` (0–5).
- In-flight work is canceled when the router process closes the external-policy
  strategy; failed payloads are not retained on disk.
- `on_delivery_failure: log` emits a sanitized `request_id` + `error_class`
  line; `ignore` stays quiet. Delivery failure never changes the caller response.

## Verifier hints (`external_policy.verifier_hints`)

Opt-in metadata passthrough for offline evaluation tagging. Authorized only when
`verifier_hints.enabled: true` on the model group.

```yaml
external_policy:
  verifier_hints:
    enabled: true
    allowed_kinds: [exact, regex, json_schema]
    max_spec_bytes: 4096
    max_version_len: 64
```

Callers may attach a hint through:

- `metadata.metrum_verifier_hint` as a JSON object or JSON string
- top-level `verifier_hint` / `verifierHint` on the request body when the dialect
  preserves unknown fields in `Raw`

Allowed kinds are metadata only: `none`, `exact`, `regex`, `json_schema`.
Executable kinds such as `pytest`, shell, or command payloads are rejected at
config validation and at request time. Invalid authorized hints fail closed with
a routing-policy error.

Content-capture governance: verifier ground truth in `spec` is request metadata.
It is forwarded only to the trusted policy service when enabled. Usage/diagnostics
rows do not persist hint bodies. If governed content capture is enabled for the
deployment, treat hint specs as sensitive captured content subject to the same
encryption, retention, and purge controls as other request metadata present in
the captured body.

## Validation

```bash
go test ./internal/router -run 'ExternalRoutingPolicy|Config|Conversation|Feedback|Verifier'
make lrp-test
```

Synthetic wiring evidence (httptest policy/feedback servers and LRP schema parse)
is distinct from provider-backed promotion evidence. These issues do not authorize
live routing activation.
