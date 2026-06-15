---
title: User Key Generation
---

# User Key Generation

Router caller tokens authenticate applications, users, or evaluation jobs to Smart LLM Router. Tokens use a readable prefix for traceability plus a random secret suffix. The router stores only token hashes.

<div class="contactBanner">
  <p>For production token design and rollout support, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Token

```bash
router-token-gen \
  --user chetan \
  --project metrum-insights \
  --environment prod \
  --allow default,fast,small,big-coder
```

The tool prints:

- The full token to give to the caller once.
- A public `token_id` used in logs and reports.
- A `callers:` YAML entry containing `token_sha256`.

## Access Patterns

```yaml
allow: [default, fast, small]
```

Use this for standard users or applications.

```yaml
allow: [default, fast, small, medium, high, big-coder]
```

Use this for coding agents, evaluations, or approved heavier workloads.

## Rotation

Generate a new token, add its hashed caller entry to config, reload or restart the router, then remove the old caller entry after clients have switched.
