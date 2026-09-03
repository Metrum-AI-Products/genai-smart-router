# Self-managed license operations

GenAI Smart Router is OSS software. Operators generate, retain, rotate, and use
their own Ed25519 runtime-policy keypairs. No external issuer, signing service,
entitlement record, payment record, or hosted dependency is required.

## Key custody

- Generate a local pair with `metrum-genai-smartrouter-license generate-keypair`.
- Store the private key mode `0600` outside Git, Kubernetes, container images, logs, and tickets.
- The public key is safe to mount with `license.json` in the runtime Secret.
- Back up the private key. Loss requires a trust rotation: new pair, replacement license, and replacement public key.

## Automated deployment

Use `scripts/helm_install_with_license.sh`. It creates a keypair on first use, issues a time-bounded license, atomically refreshes the runtime Secret, and runs Helm. Only `license.json` and `license.pub` are uploaded to Kubernetes.

## Reissue and rotation

Reissue from the same local key for normal renewal. Rotate both the public key and license together when changing trust. Do not edit signed license JSON by hand.

## Safe evidence

Record only license ID, key ID, expiry, feature names, configured limits, status, reason, and request IDs. Never record signing material, caller tokens, provider keys, or full runtime configuration.
