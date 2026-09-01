---
title: License Enforcement
doc_type: explanation
---

# License Enforcement

License enforcement is self-managed and offline. The router verifies an operator-generated signed `license.json` with the corresponding public key configured in `server.license.public_keys`.

License terms can contain expiry, feature, volume, concurrency, and deployment limits chosen by the operator. A failed term produces structured `license-*` errors; repair it by reissuing a license with the protected local signing key and updating the runtime Secret through the deployment wrapper.

The router has no billing, payment, procurement, marketplace, or hosted license-issuer dependency.
