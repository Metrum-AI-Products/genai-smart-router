---
title: Renewal And Top-Up
doc_type: explanation
---

# Renewal And Top-Up

License renewal, replacement, and volume top-up use the same customer-side workflow: install the newly issued `license.json`, then let the router recheck it.

## When To Replace A License

Replace the runtime license when:

- the current license is near expiry;
- Metrum issues an evaluation or pilot extension;
- the commercial plan changes;
- a volume top-up is purchased;
- instance scope changes;
- an issued license is corrected or reissued;
- a verification-key rotation requires a new license or release package.
- Metrum issues a replacement license after approved renewal, extension, or top-up fulfillment.

## Replacement Workflow

1. Receive the new Metrum-issued `license.json` through the approved delivery channel.
2. Back up the current runtime license according to the deployment's secret-handling policy.
3. Replace the file at `server.license.path` atomically where possible.
4. Restart the router or wait for `server.license.recheck_interval`.
5. Verify readiness, safe license status, metrics, and one caller smoke.

The new file should have a different license ID for renewals, replacements, and volume top-ups. For top-up licenses, the router treats the new license ID as a new license-wide volume budget; caller-token quotas remain controlled by the deployment's caller-key policy.

Example validation:

```bash
export ROUTER_BASE_URL="https://<router-host>"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/license/status"

curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

## Grace Behavior

`grace_period_on_validation_error` is for transient validation problems after a previously valid license was observed. It is not a renewal mechanism and does not permanently permit expired licenses, exhausted volume, unlicensed features, or deployment limits.

If a deployment enters grace, treat it as an operational incident:

- confirm the runtime file is readable;
- check system time;
- review the safe license reason;
- install the corrected license if one was issued;
- confirm `/readyz` returns success after replacement.

## Re-Download And Top-Up

Renewal and top-up packages result in a newly issued signed license. The runtime installation flow remains the same: replace `license.json`, restart or wait for recheck, and validate readiness plus one caller smoke. Commercial payment and procurement records remain outside the router request path.

## Rollback

Rollback is restoring the previous valid runtime license file and restarting the router or waiting for the next recheck interval. Roll back only to a license that is still valid for the deployment and commercial scope.

Do not work around a failed license by disabling enforcement in runtime YAML. Normal release builds require license enforcement.

## Support Handoff

For renewal or top-up support, share only safe status fields: license ID, SKU, key ID, expiry, status, reason, grace-active flag, router version, and request IDs. Do not send provider keys, router tokens, token hashes, raw prompts, raw images, full config files, private signing material, raw signatures, or full customer-specific license payloads through ordinary support channels.
