# 04 — Commercial Control Plane and Tenant Lifecycle (#521)

Plan version: **v1.0.0**  
Issue: [#521](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/521)  
Classification: **launch blocking**

## Design approach

Add a separately deployed Go control-plane API and worker in this repository for
the first release (`cmd/control-plane`, `cmd/control-plane-worker`, and typed
`internal/controlplane` packages). Keep it separate from `cmd/router` at runtime.
Use PostgreSQL with explicit migrations from #507. The public/customer API is
typed Go HTTP; PostgREST, if retained for #7 operator CRUD, stays private behind
the admin boundary and is never the router's hot-path dependency.

Core relational entities are `users`, `organizations`, `memberships`, `tenants`,
`projects`, `api_credentials`, `tenant_state_transitions`, `entitlement_versions`,
`provisioning_operations`, `idempotency_keys`, `outbox_events`, and append-only
`control_audit_events`. Structured repetition uses child tables, not JSON/JSONB,
arrays, or packed multi-value text. Secrets are references; the API credential
table holds hash/public ID/status only.

Tenant state is explicit:

```text
pending_email -> pending_funding -> provisioning -> active
                       |                |             |
                       v                v             v
                    canceled      provision_failed  past_due
                                                      |
                                            suspended <-> active
                                                      |
                                                   canceled
```

Every transition has an actor/event, expected prior version, idempotency key,
reason code, timestamp, and resulting outbox work. Invalid or stale transitions
fail without partial state.

## Human interaction and configuration

Product/Security choose D8 identity provider and whether an organization may have
multiple owners at launch. Legal approves required terms-version capture.
Operators configure OIDC issuer/client/audience, console origins, email sender,
region/cell catalog, plan IDs, entitlement templates, KMS signing key references,
provisioner queue, state timeouts, and support roles. Provider credentials and raw
router tokens are not control-plane configuration responses.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Required accounts
are OIDC/email, AWS runtime, PostgreSQL, and support/status; Stripe is referenced
only through fulfillment APIs owned by #524. Secrets are PostgreSQL DSN, session
key, OIDC client secret if required, email credential or SES role, internal
service-auth certificate/key reference, and bounded queue credentials. Non-secrets
are issuer/audience/client ID, callback/logout/CORS origins, claim mapping,
region/cell/plan/SKU/terms versions, state timeouts, pagination, retries,
entitlement template IDs, verifier key IDs, and support roles. Raw customer API
keys use a CSPRNG, are returned once, hashed before persistence, excluded from all
telemetry/events, and rotated/revoked by public token ID.

Support UI requirements are deliberately minimal: search by safe tenant/org/user
ID, see state/transition timeline, resend verification, request suspension or
resume with reason, retry failed provisioning, revoke a public token ID, and open
linked audit/evidence. Destructive actions require reauthentication and two-step
confirmation. The visual language follows the operations prototype in
[operations-dashboard.html](previews/operations-dashboard.html).

## API and worker boundaries

* `/v1/account`, `/v1/organizations`, `/v1/tenants`, `/v1/projects` — scoped CRUD.
* `/v1/api-keys` — create/show once/list safe metadata/rotate/revoke.
* `/v1/tenants/{id}/state` — server-authorized transitions with optimistic version.
* `/internal/fulfillment/*` — signed service identity only; payment, grant, lease,
  provisioning and notification events.
* Transactional outbox — database mutation and event enqueue in one transaction;
  workers claim with bounded leases and process idempotently.

Online lease/entitlement artifacts contain only runtime enforcement facts. They
do not contain payment status, invoices, refunds, card details, or Stripe IDs.
Router keeps the last valid artifact during bounded control-plane outage.

## End-to-end flow

1. OIDC callback establishes a secure session and creates/reconciles the user.
2. Verified user creates an organization/tenant with chosen supported region.
3. Tenant enters `pending_funding`; trial/risk or Stripe later emits a fulfillment.
4. A transaction moves to `provisioning` and enqueues one provisioning operation.
5. Provisioner writes the resulting deployment/config/lease references, runs
   health gates, then atomically activates the tenant.
6. API key creation generates entropy in memory, persists only its hash/public ID,
   returns raw value once, and creates/activates a caller through #7.
7. Suspension revokes new grants/lease renewal and caller access; cancellation
   schedules governed teardown without deleting ledger/audit history.

## Test plan

* Exhaustive unit transition table for legal, illegal, repeated, and racing events.
* PostgreSQL migration and repository tests for constraints, tenant scoping,
  optimistic version, outbox atomicity, and worker lease recovery.
* API contract tests for OIDC claims, RBAC, CSRF/origin, pagination, idempotency,
  create/show-once/rotate/revoke, and sanitized errors.
* Two-tenant integration suite attempts every cross-tenant read/write and support
  action; assert denial and no existence leak.
* Fault-inject after every transaction/outbox/worker boundary; retry must converge
  once with one tenant, one credential, one provisioning job, and one transition.
* Run the full create -> fund fixture -> provision -> active -> suspend -> resume ->
  cancel flow against staging and capture safe DB/audit/API evidence.
* Scan log/trace/DB/HTML captures for raw tokens/hashes, provider credentials,
  prompts, card fields, and full entitlement payloads.

## Monitoring, recovery, and rollback

Measure API latency/error by safe route class, auth denial, transition failure,
outbox age/depth, worker retries/dead letters, provisioning state age, lease/grant
issuance, and notification failure. Alert on any tenant stuck beyond its state SLO.
Rollback uses compatible schema and previous images; workers stop claiming before
rollback and safely resume expired leases afterward. The router continues on its
last valid entitlement only within documented grace.

## Definition of done

Schema/API/state machine, signer boundary, support actions, runbooks, threat-model
review, and all tests/evidence pass. No control-plane outage causes an unbounded
provider-cost window or a synchronous commercial call in the router handler.
