---
title: External Policy Conversation Key, Feedback, and Verifier Hints
doc_type: reference
---

# External Policy Conversation Key, Feedback, and Verifier Hints

This page describes caller-visible and admin-configured extensions to
[External Routing Policy](./external-routing-policy) for conversation pinning,
post-completion feedback, and offline verifier metadata. Operator detail lives in
the repository runbook `docs/EXTERNAL_POLICY_CONTEXT.md`.

## Conversation key

When a model group uses `strategy: external`, the policy request includes
`context.conversationKey`. Callers can keep multi-turn pins stable by sending a
deployment-defined conversation id in request metadata:

```json
{
  "model": "adaptive",
  "messages": [{"role": "user", "content": "Continue the previous task."}],
  "metadata": {
    "conversation_id": "app-thread-123"
  }
}
```

Accepted metadata keys: `conversation_id`, `conversationId`, `session_id`,
`sessionId`, and `metrum_conversation_id`. Values must be 1–128 characters of
letters, numbers, and `._:-`.

If no conversation id is present, the router hashes the first user-turn text
locally and still emits a pseudonymous key. The policy service does not receive
that raw text unless the operator separately enables `include_request`.

Keys are scoped by caller identity and model group. They never include bearer
tokens or token hashes. Treat them as routing pin identifiers, not authorization
secrets.

## Completion feedback

Operators may enable an authenticated post-completion callback on the policy
service. Callers do not configure this; it is deployment-owned. The callback
carries only request ID, selected target, status, stored usage/cost/latency, and
related safe scalars. Prompts, tools, and credentials are never included.

## Verifier hints

When the operator enables `external_policy.verifier_hints`, callers may attach
bounded offline-evaluation metadata:

```json
{
  "model": "adaptive",
  "messages": [{"role": "user", "content": "What is 2+2?"}],
  "metadata": {
    "metrum_verifier_hint": {
      "kind": "exact",
      "version": "v1",
      "spec": {"expected": "4"}
    }
  }
}
```

Supported kinds are `none`, `exact`, `regex`, and `json_schema`. Hints are
metadata only and are never executed by the router. Executable kinds are
rejected. Size and version limits are configured by the operator. Hint ground
truth is trusted policy/evaluation metadata; do not place secrets in verifier
specs.
