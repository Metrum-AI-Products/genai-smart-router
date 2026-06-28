---
title: Licensing Troubleshooting
---

# Licensing Troubleshooting

License failures can block readiness or individual licensed features. Use safe status fields and request IDs for support; do not share full license payloads or signing material.

## Checks

```bash
export ROUTER_BASE_URL="https://llm-api.example.com"

curl -i "$ROUTER_BASE_URL/readyz"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/license/status"
```

If metrics are enabled for a metrics-admin subject, inspect safe license gauges through `/metrics`.

## Common Errors

| Error | Meaning | Operator action |
|---|---|---|
| `license-missing` | The configured license file is not readable. | Mount the issued file at `server.license.path` and check permissions. |
| `license-invalid` | The file is malformed, unverifiable, uses an unknown key, or has been edited. | Restore or replace it with a valid Metrum-issued file. |
| `license-expired` | The valid license is past expiry. | Install a renewed license and restart or wait for recheck. |
| `license-not-yet-valid` | The license start time is in the future. | Check system time and issued dates. |
| `license-product-mismatch` | The license was not issued for GenAI Smart Router. | Install the correct product license. |
| `license-feature-forbidden` | The request or config uses an unlicensed feature. | Disable the feature or obtain an updated license. |
| `license-limit-exceeded` | A licensed deployment limit is exceeded. | Reduce configured usage or install an updated license. |
| `license-volume-exceeded` | License-wide volume is exhausted. | Install a top-up or replacement license. |
| `license-window-exceeded` | A rolling licensed request or token window is at its ceiling. | Wait for the window or update the license. |
| `license-concurrency-exceeded` | Licensed in-flight request limit is reached. | Reduce concurrency or update the license. |
| `license-clock-rollback` | Local wall clock moved backwards beyond tolerance. | Correct system time and inspect durable license state. |

## Recovery

1. Confirm `server.license.path` points to the mounted runtime license.
2. Confirm the router process can read the file.
3. Confirm `server.license.state_path` is on durable writable storage.
4. Check system time synchronization.
5. Install the corrected or renewed license if one was issued.
6. Restart the router or wait for `recheck_interval`.
7. Verify `/readyz`, `/admin/license/status`, metrics, and one caller smoke.

Do not set `server.license.enabled: false` to recover a normal release deployment. Runtime YAML cannot disable licensing in normal release builds.

For planned replacement, see [Renewal And Top-Up](../licensing/renewal).
