---
title: Licensing
---

# Licensing

Normal GenAI Smart Router release builds enforce an offline Metrum-issued signed JSON license. The deployed router verifies the license locally and does not need private signing keys or a network call to Metrum at startup.

Use this section for customer-facing install, renewal, and support workflows. It intentionally does not describe signing-key custody, signing-service internals, or development bypass mechanics.

## What Operators Install

Operators install one Metrum-issued runtime file:

```text
license.json
```

Mount it at the configured `server.license.path` and keep the license state file at a durable `server.license.state_path`.

```yaml
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    recheck_interval: 1h
```

Do not edit `license.json`. Any payload change invalidates the signature.

## What The Router Exposes

When authorized, operators can inspect safe license status fields such as:

- status and reason;
- license ID;
- customer ID;
- SKU;
- key ID;
- expiry;
- grace-active flag;
- enabled feature gates and safe limit status.

The router does not expose private signing keys, provider API keys, raw router tokens, token hashes, full config files, raw prompts, raw images, raw signatures, or full license payloads through ordinary caller APIs.

## Customer Journey

1. Metrum issues a license for the contracted evaluation, pilot, annual deployment, renewal, replacement, private offer, or volume top-up.
2. The customer installs the release package and mounts `license.json`.
3. The operator validates `/readyz`, `/admin/license/status`, and one licensed caller workflow.
4. The operator monitors license status through metrics, admin status, and request errors.
5. Renewal, replacement, and top-up are handled by replacing `license.json` and restarting the router or waiting for the configured recheck interval.

Metrum may provide a safe summary alongside the issued file. That summary is for support handoff and should include only scalar metadata such as license ID, customer ID, SKU, key ID, issuer, issue time, not-before time, expiry, feature names, and configured limits. It is not a substitute for the signed `license.json`.

For the step-by-step replacement workflow, see [Renewal And Top-Up](/docs/licensing/renewal). For operational failure modes, see [Troubleshooting Licensing](/docs/troubleshooting/licensing).

## Support Boundaries

Share request IDs and safe status fields with support. Do not send provider keys, router tokens, token hashes, raw prompts, raw images, raw tool outputs, private host details, full production config, private signing material, or full customer-specific license payloads through ordinary support channels.

For license issuance, renewal, volume top-up, or commercial plan changes, contact [contact@metrum.ai](mailto:contact@metrum.ai).
