---
title: User Key Generation
---

# User Key Generation

Router caller tokens authenticate applications, users, or evaluation jobs to Smart LLM Router. Tokens use a readable prefix for traceability plus a random secret suffix. The router stores only token hashes.

`router-token-gen` is an Enterprise Edition administrative CLI. It is intended for platform administrators and is run from a secure server console, deployment host shell, or controlled admin workstation. It is not a browser feature and should not be distributed to ordinary application users.

<div class="contactBanner">
  <p>For production token design and rollout support, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Token

```bash
router-token-gen generate \
  --user chetan \
  --project metrum-insights \
  --env prod \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

The tool prints:

- The full token to give to the caller once.
- A public `token_id` used in logs and reports.
- A `callers:` YAML entry containing `token_sha256`.

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

## Rotation

Generate a new token, add its hashed caller entry to config, reload or restart the router, then remove the old caller entry after clients have switched.
