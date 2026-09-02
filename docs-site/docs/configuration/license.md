---
title: License
doc_type: reference
---

# License

All GenAI Smart Router first-party content is licensed under Apache-2.0. The
`license.json` described here is an operator runtime-policy input, not a
copyright license or commercial-use condition, and it does not limit
Apache-2.0 rights.

License configuration controls offline signed JSON license enforcement. Normal release builds verify a Metrum-issued license file with embedded public verification keys, periodically recheck it, and fail closed when the license is invalid outside any configured grace period.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml title="config.example.yaml"
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    instance_fingerprint: ""
    instance_fingerprint_file: ""
    instance_fingerprint_env: ""
    recheck_interval: 1h
    grace_period_on_validation_error: 24h
    revocation:
      mode: off
      path: /app/config/revocations.json
      require_current_bundle: false
      fail_closed_on_bundle_error: true
    fail_open_for_dev: false
```

## Schema

`server.license.path` points at the signed runtime license. `state_path` stores safe local license state. The instance-fingerprint fields are mutually exclusive inputs for deployments with instance-bound licenses. `revocation` stays disabled unless Metrum provides a signed revocation bundle.

When enforcement blocks serving, `/readyz` fails and caller APIs return documented `license-*` errors. Logs, metrics, reports, and admin status APIs may expose only safe scalar license metadata such as status, license ID, customer ID, SKU, key ID, expiry, and grace flag.

## Rollback

Runtime YAML cannot disable licensing in normal packaged deployments. To recover from an invalid renewal, restore the previous signed license file or revocation bundle, restart or wait for recheck, and smoke `/readyz` plus one authenticated caller request.

## Related

See [License-Protected Deployments](../operations/license-protected-deployments), [Commercial Access And Licensing](../licensing/), and [Router Configuration](./router-config).
