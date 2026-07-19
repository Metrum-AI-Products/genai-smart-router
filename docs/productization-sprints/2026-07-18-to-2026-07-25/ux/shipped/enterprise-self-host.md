# Shipped: Enterprise self-host

**Personas:** P5 Enterprise operator

**HTML:** [enterprise-self-host.html](enterprise-self-host.html)

## Happy path

| Step | User sees | User does | System |
| --- | --- | --- | --- |
| Download | Package matrix amd64/arm64 | Fetch tarball | Release artifact |
| Install | Binary/Compose/K8s docs | Unpack, place config | — |
| License | Path for license.json | Install signed license | Local verify public keys |
| Keys | env example | Set provider env refs | api_key_env |
| Smoke | Commands | /readyz, /v1/models, API shapes | Pass/fail |
| Renew | Expiry banner | Replace license.json | Recheck |

## Expiry

License grace per config; after grace fail-closed. Renewal via commercial channel — replacement file.
