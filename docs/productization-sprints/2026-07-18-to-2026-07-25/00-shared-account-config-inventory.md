# 00 — Shared Account, Credential, and Configuration Inventory

Plan version: **v1.0.0**  
Applies to: **#520–#533**

This is the canonical planning registry. It names required configuration without
containing real account IDs, endpoints, tokens, keys, hashes, secrets, or complete
production configuration. Implementation must create an environment-specific
inventory in the approved secret/config system and record only safe secret
references in deployment config.

## Environment and ownership model

Use separate `dev`, `stage`, and `prod` trust boundaries. Prefer separate AWS and
Stripe accounts where feasible; at minimum use distinct credentials, webhook
endpoints, KMS keys, databases, domains, queues, and IAM roles. Production access
requires SSO/MFA, least privilege, time-bounded elevation, and audit.

Each registry item must record: owner team, backup owner, environment, account/
resource safe ID, secret-manager path (never secret value), creation date,
rotation/expiry, consumers, allowed operations, break-glass procedure, and last
successful validation. A CI check rejects placeholder or secret values in source.

## External accounts required

| Service/account | Purpose | Secret or credential type | Storage/identity | Human setup |
| --- | --- | --- | --- | --- |
| AWS Organizations accounts | EKS, ECR, RDS, cache/queue, KMS, DNS, email | no static keys for workloads/CI | GitHub OIDC, EKS Pod Identity/IRSA, SSO roles | Cloud/Security create dev/stage/prod boundaries and budgets |
| Stripe test/live accounts | Checkout, Portal, refunds, disputes, webhooks | restricted API secret, webhook signing secret | Secrets Manager; workload identity reads exact path | Finance creates products/prices, Portal config, tax/refund settings, restricted roles |
| Managed OIDC/identity account | signup, verified email, sessions, MFA | client secret only if flow requires it | Secrets Manager and issuer/audience config | Security configures pool, callback/logout URLs, email templates, MFA/admin policy |
| Transactional email account | verification, receipts, alerts, support | API/SMTP credential if SES role not used | prefer AWS workload identity; otherwise Secrets Manager | Product/Legal approve sender/domain/templates and unsubscribe rules |
| Domain/DNS account | console/API/status/support hostnames | DNS change role; no static key | Route 53 IAM role/ExternalDNS scope | SRE proves domain ownership, hosted zones, TTL and incident access |
| Certificate service | HTTPS | ACM/cert-manager issuer reference | IAM/workload identity | SRE configures issuance/renewal/alerting |
| Provider accounts | pooled upstream inference | provider API keys | data-plane-only Secrets Manager/External Secrets paths | Provider owner funds accounts, configures limits and rotates keys |
| GitHub environments | CI/CD and approvals | GitHub OIDC; no AWS key | GitHub/AWS trust policy | Maintainer configures protected environments and required checks |
| Status/support system | incidents and cases | integration token if API used | Secrets Manager | Support creates queues, severities, on-call and public status components |
| AWS Marketplace seller account | later SaaS/AMI/container offer | seller/role access; metering via workload role | AWS SSO/IAM | Commercial owner completes onboarding and offer approvals |

## Secret references

Exact paths are implementation-owned; use a pattern such as
`/<product>/<environment>/<component>/<secret-name>` and validate that a workload
can read only its entries.

* Control plane: session key, OIDC client secret when applicable, PostgreSQL DSN,
  Stripe restricted key, Stripe webhook secret, email credential, support/status
  integration, and internal service mTLS/JWT key.
* Ledger: PostgreSQL DSN and backup identity; no provider or router credential.
* Grant issuer: dedicated KMS asymmetric signing key ID/alias; private material is
  non-exportable. Router receives allowlisted public verifier keys only.
* License issuer: separate KMS/key custody from grants and Stripe. Existing test
  license keys remain test-only; real private material is never committed.
* Provisioner: environment-scoped IAM role for EKS, Route 53/ACM, and exact secret
  reference paths/resources it owns.
* Data plane: provider API keys, caller-token hashes from controlled activation,
  license/lease, usage/reservation backend identity, and verifier public keys. It
  has no Stripe API key.

Never place secrets in GitHub variables, issues, ConfigMaps, committed Helm
values, screenshots, shell history, browser storage, URLs, analytics, logs,
traces, metrics, or evidence bundles.

## Non-secret configuration registry

Version and review these values per environment:

* public console/API/status/docs/support origins and allowed CORS/callback origins;
* OIDC issuer, audience/client ID, claim mapping and terms version;
* region/cell catalog, AWS account/cluster safe aliases, namespace strategy;
* ECR image repositories/digests, chart/config/schema versions;
* Route 53 zone safe ID, record templates, TLS mechanism, ingress class;
* Stripe mode, Product/Price IDs, Portal configuration ID, payment methods,
  currency and webhook event allowlist;
* plan/SKU IDs, entitlements, allowed deployment-defined model groups, RPM/TPM/
  concurrency/traffic-shape/quota/budget defaults;
* price-book version, unit scale, markup/minimum/rounding, grant expiry/size/
  refresh lead/buffer, reservation/outbox bounds;
* database pool/timeout/retention/backup, queues/retries/dead letters;
* telemetry sampling/retention, SLOs/alerts/on-call destinations;
* email template IDs, legal-document, refund/trial/abuse policy versions;
* canary thresholds, rollout weights/soak and rollback rules;
* Marketplace product code/dimensions only if D5 selects that channel.

## Account and key bootstrap checklist

1. A human owner creates the resource under enterprise ownership, not a personal
   account, and assigns a backup owner.
2. Security reviews role scope and ensures SSO/MFA/break-glass audit.
3. Workload identity is created before any fallback secret. Static cloud keys are
   prohibited unless an approved exception has expiry and rotation.
4. Secret is written directly to Secrets Manager/KMS; never passed through chat,
   issue, PR, or terminal output.
5. Consumer uses the secret reference and logs only success/failure plus safe key
   ID/version during readiness.
6. Stage validation rotates the credential/key: overlap if required, verify new,
   revoke old, prove old rejection, and capture redacted evidence.
7. Inventory and next-rotation date are recorded in the governed asset system.

## Evidence redaction checklist

Reject or redact bearer/API/provider keys, token hashes, Stripe secrets/card
fields, session cookies, signing material/full artifacts, prompts/responses/images,
tool schemas/outputs, customer PII, full config/env, connection strings, and any
internal account/hostname data restricted by policy.
