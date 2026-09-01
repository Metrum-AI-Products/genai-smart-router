---
title: Renewal and Rotation
doc_type: howto
---

# Renewal and Rotation

Operators own license issuance and key custody.

## Renew a License

Run the deployment wrapper again with the protected local keypair and an updated validity window. It issues a replacement `license.json`, atomically updates the runtime Secret, and upgrades the Helm release.

## Rotate a Signing Key

Generate a new local keypair, then deploy both the replacement `license.json` and `license.pub` in the same wrapper run. Do not edit a signed license by hand. Back up private keys securely; losing one requires a trust rotation and replacement license.

After either operation, verify `/readyz` and an authorized model request.
