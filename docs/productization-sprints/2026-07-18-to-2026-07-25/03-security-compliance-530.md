# 03 — Security, Compliance, and Tenant Isolation (#530)

Plan version: **v1.0.0**  
Issue: [#530](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/530)  
Classification: **launch blocking; starts before implementation and closes last**

## Design approach

Threat-model the new system before schema/API work, then rerun validation against
the built staging system. Trust boundaries are browser/OIDC, console/API,
Stripe/webhooks, commercial database/ledger, signer/grant issuer, provisioner/AWS,
router/data plane, provider egress, observability, and support/admin access.

Use tenant ID derived from authenticated server-side membership, never a client
supplied authoritative header. Every repository/query method accepts a scoped
tenant context. Financial postings and audit rows are append-only. Raw caller
tokens are write-only; card data remains Stripe-hosted; signing keys and provider
credentials live in separate KMS/Secrets Manager policies.

For shared tenancy, enforce row/query scope, caller/model/budget scope, Kubernetes
service-account/network boundaries, and no tenant labels on public/global metrics.
For dedicated tenancy, add namespace, resource quota, secret, network policy,
service account, DNS, and teardown isolation tests.

## Human interaction and configuration

Security approves the IdP, MFA/break-glass policy, cookie/session settings,
allowed console/API origins, CSP, webhook tolerance, KMS key policies, signer
separation, support roles, retention, privacy region, vulnerability SLAs, and pen
test scope. Compliance maps evidence to existing SOC 2 Type 2 and ISO 27001
controls and confirms Stripe-hosted capture preserves the intended SAQ-A boundary.

## Account, API-key, secret, and configuration inventory

Audit every item in the [shared registry](00-shared-account-config-inventory.md).
This issue owns access matrices for AWS SSO/IAM, GitHub environments, Stripe
roles/restricted keys/webhook secrets, OIDC admins/clients, grant/license KMS keys,
Route 53/ACM, email, provider accounts, databases/backups, support/status, and
Marketplace. For each key/role record owner/backup, scope, consumers, rotation/
expiry, last-used visibility, break-glass, and revocation test. Config includes
CSP/CORS/callbacks, session/CSRF, webhook tolerance/event allowlist, key overlap,
network egress, retention/sampling, scanner thresholds, and forbidden evidence.
Tests use synthetic credentials and never dump real values.

## Required controls

* OIDC authorization-code flow with PKCE, verified email, short sessions, secure
  HttpOnly SameSite cookies, CSRF protection, and reauthentication for key/billing
  changes.
* Deny-by-default Casbin/IAM/RBAC, audited support impersonation, MFA break-glass,
  and separation of Stripe admin from license/grant signing.
* Envelope/key rotation with overlapping public verification keys, bounded grant
  expiry, nonce/version, replay detection, and clock-skew policy.
* TLS, encryption at rest, backups, restore testing, secret scanning, dependency/
  container/IaC scanning, signed artifacts, and egress allowlists.
* No prompt, raw image, raw tool payload/output, raw token/hash, provider key,
  full config, PAN/CVC, or unsanitized webhook body in ordinary telemetry.

## Test plan

| Test | Action | Expected result/evidence |
| --- | --- | --- |
| Tenant matrix | exercise every role/action for tenant A against A and B | allowed matrix only; cross-tenant 404/403 and safe audit |
| Session | CSRF, fixation, stale role, revoked user, origin and cookie attacks | fail closed; reauth on sensitive operations |
| Secrets | scan git, images, logs, traces, analytics, DB snapshots | no forbidden values; synthetic canaries detected |
| Stripe | invalid/old/replayed signature, unordered duplicate events | 400 or idempotent no-op; no credit without paid state |
| Grants | tamper, replay, over-reserve, expired key/clock skew | denial before upstream; bounded safe reason |
| Kubernetes | service-account, secret, network, namespace escape | denied and alerted; unrelated tenants healthy |
| Metrics | ordinary caller requests `/metrics` and admin data | 403 `metrics-forbidden`; metrics-admin still succeeds |
| Supply chain | unsigned/tampered artifact and vulnerable threshold | deployment blocked |
| Recovery | rotate IdP, webhook, DB, signing, and provider secrets | bounded interruption; old secret invalid after overlap |

Run SAST/DAST, dependency/container/IaC scanners and an independent staging pen
test. Record redacted terminal replays, tool versions, findings/remediations,
negative authorization screenshots, and a signed exception register.

## Incident and rollback

Prepare playbooks for customer credential theft, signing-key compromise, Stripe
webhook compromise, provider-key compromise, cross-tenant exposure, ledger
integrity alarm, and unauthorized support access. The emergency order is contain
new grants/keys, preserve audit/financial evidence, rotate affected trust, keep
unaffected tenants serving when safe, notify by approved policy, and validate
recovery through the same isolation suite.

## Definition of done

Security and Compliance approve the threat model, control mapping, pen-test
closure, metrics-admin isolation, PCI boundary, incident drills, and residual-risk
register. No certification or residency claim is published without owner approval.
