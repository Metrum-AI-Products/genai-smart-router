---
title: User Key Generation
---

# User Key Generation

Router caller tokens authenticate applications, users, or evaluation jobs to GenAI Smart Router. Tokens use a readable prefix for traceability plus a random secret suffix. The router stores only token hashes.

`router-token-gen` is an Enterprise Edition administrative CLI for platform administrators. Run it from a secure server console, deployment host shell, or controlled administrator workstation, and distribute only the generated router tokens to approved callers.

<div class="contactBanner">
  <p>For production token design and rollout support, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Token

```bash
router-token-gen generate \
  --owner-user chetan \
  --project metrum-insights \
  --env prod \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

The tool prints:

- The full token to give to the caller once.
- A public `token_id` used in logs and reports.
- A `callers:` YAML entry containing `owner_user`, `project`, and `token_sha256`.

Before the key can authenticate, `owner_user` must exist in `users`, `project` must exist in `projects`, and `project_memberships` must contain an active membership for that user/project pair. Each configured user id, project id, caller `id`, `token_sha256`, and non-empty `token_id` must be unique after normalization. Token hashes are checked case-insensitively, and duplicate-hash validation errors identify the caller IDs without printing hash values. User, project, and membership statuses support `active`, `disabled`, `suspended`, `removed`, and `archived`; caller key statuses also support `expired` and `rotated`. `--username` is an alias for `--owner-user`; `--user` remains a deprecated compatibility alias.

## Access Patterns

```yaml
allow: [example-general, example-low-cost]
```

Use this pattern for users or applications that should only see a smaller set of deployment-defined groups.

```yaml
allow: [example-general, example-low-cost, example-coding]
```

Use this pattern for coding agents, evaluations, or approved heavier workloads.

Model group names are deployment-defined. Names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, and `vision` are examples from a reference or hosted deployment, not names required by the product.

Callers see only allowed groups when they call `/v1/models`. See [Available Models And Access](../getting-started/available-models) for the caller-facing behavior administrators should expect after issuing a token.

## Rotation

Generate a new token, add its hashed caller entry to config, reload or restart the router, then remove the old caller entry after clients have switched.

To suspend access without deleting history, set the caller key `status: suspended`, `disabled`, `expired`, or `rotated` and restart or reload the deployment. Disabling a user, project, or project membership should be done by removing or correcting active key references first, because config validation requires active account references for all enabled keys.
