---
title: Caller Tokens
doc_type: reference
---

# Caller Tokens

Caller token configuration binds router-issued bearer tokens to explicit users, projects, environments, allowed model groups, rate limits, quotas, and administrative roles. `/v1/models` is the caller-facing source of truth for the model groups allowed by the presented caller token.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml
users:
  - id: example-standard
    name: Example Standard Caller
    type: service_account
    status: active
projects:
  - id: example-project
    name: Example Project
    status: active
project_memberships:
  - user_id: example-standard
    project: example-project
    role: developer
    status: active
callers:
  - id: example-standard-prod
    owner_user: example-standard
    project: example-project
    environment: prod
    status: active
    token_sha256: SHA256_HEX_OF_STANDARD_ROUTER_TOKEN
    token_id: rtr_metrum_example-standard_example-project_prod_k20260614
    metrics_admin: false
    content_admin: false
    allow:
    - default
    - fast
    - small
    rate:
      rpm: 120
      tpm: 200000
      concurrent: 8
```

## Schema

Users and projects are account records, not values inferred from token names. Project memberships bind active users to active projects. A caller token config references an owner user, project, environment, token hash, public token ID, allow-list, and limit policy.

User IDs, project IDs, membership pairs, caller IDs, token hashes, and non-empty `token_id` values must be unique after normalization. Legacy `callers[].user` is accepted as a deprecated alias for `owner_user` only when both normalize to the same ID. Duplicate-hash validation errors identify caller IDs without printing hash values.

Disallowed model-group requests return `403 model-not-allowed`. Inactive caller tokens return status-specific safe errors such as `key-disabled`, `key-suspended`, `key-expired`, or `key-rotated`. `/metrics` remains global operational telemetry and is restricted to metrics-authorized subjects.

## Rollback

Disable or rotate a caller token by changing caller `status`, removing the model group from `allow`, or replacing the token hash with a new generated router token. Keep old public token IDs available in historical reports; never reuse a token hash or expose raw router tokens in docs, tickets, or logs.

## Related

See [Available Models And Access](../getting-started/available-models), [Key Generation And Rotation](../operations/key-generation), [Admin Authorization](./admin-authorization), and [Router Configuration](./router-config).
