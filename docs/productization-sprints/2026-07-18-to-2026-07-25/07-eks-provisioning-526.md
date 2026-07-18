# 07 — EKS Tenant Provisioning, DNS, Licensing, and Teardown (#526)

Plan version: **v1.0.0**  
Issue: [#526](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/526)  
Classification: **launch blocking**

## Tenancy-dependent design

### Recommended shared Prosumer path

Run one highly available router fleet per supported region/cell behind one stable
HTTPS API hostname. A tenant is logical: organization/project/caller records,
model-group allowlist, quota/traffic shape, active online lease/entitlement,
balance grant scope, and tenant-scoped usage/report access. There is no per-tenant
pod, namespace, load balancer, certificate, DNS record, provider key, or license
file. This is the fast and economical default but requires #530 isolation proof.

### Dedicated managed path

Provision a namespace and Helm/Kustomize release per tenant (or approved tenant
cell), dedicated service account/network policy/resource quota/PDB/PVC, secret
references, signed license, config set, grant-verifier keys, ingress/load balancer,
DNS/TLS, and observability scope. The AWS Load Balancer Controller owns the EKS
Ingress/ELB integration; Route 53/ExternalDNS and ACM or cert-manager own approved
records/certificates. Do not create one ELB per small tenant unless cost policy
explicitly approves it; a shared ingress with host routing may still be dedicated
at namespace/router level.

AWS reference: [AWS Load Balancer Controller for EKS](https://docs.aws.amazon.com/eks/latest/userguide/aws-load-balancer-controller.html).

## Provisioning saga

Every step writes a scalar operation record with desired version, status, attempt,
external resource ID, compensating action, and safe error. Idempotency key is the
tenant plus desired deployment version.

1. Validate tenant is funded/risk-approved and region/plan is allowed.
2. Reserve a globally unique safe tenant slug; never use raw email/org text in
   Kubernetes names or telemetry.
3. Shared: create config/caller/lease/grant scope in a draft #7 config set.
   Dedicated: also create namespace/IAM/service account/storage/network/quota.
4. Store provider/license/router secrets only through Secrets Manager/External
   Secrets references. The provisioner cannot read back unrelated tenants.
5. Render and validate config, activate atomically, deploy exact signed image.
6. Dedicated: create ingress, TLS certificate, and DNS record; wait for health.
7. Run `/version`, `/readyz`, authenticated `/v1/models`, Chat/Responses/Messages
   shape smokes, grant reservation/settlement smoke, and metrics-admin smoke.
8. Mark tenant active only after all gates pass; notify control plane/console.

The [checkout/provisioning prototype](previews/checkout-provisioning.html) shows
the customer-visible stages and failure/retry language.

## Human interaction and configuration

D1/D6/D7/D9 must be approved. SRE configures cell catalog, cluster/namespace,
node class, quotas, ingress class, hosted zone, certificate mechanism, API/console
domains, TTL, image digests, secret stores, backup/storage classes, timeouts,
smoke caller/model groups, and teardown retention. Security approves IRSA/IAM,
network/egress, secret, and support access policies.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). The provisioner
uses environment-scoped AWS workload identity with least-privilege EKS, Helm/
GitOps, Route 53, ACM/cert-manager, External Secrets, ECR read, and tagging rights;
it has no Stripe key and cannot read values outside owned secret paths. Dedicated
data-plane references include provider keys, caller-token hash/config activation,
license/lease, usage/reservation backend, and grant verifier keys. Shared tenancy
uses fleet-owned versions. Non-secrets include cluster/cell, namespace/slug,
resource quota/node class, digest/chart/config version, ingress, zone/record/cert
templates, TTL, health smokes, model/quota templates, timeouts/tags/retention and
teardown delay.

## Failure, rollback, and cancellation

Each failed step retries with backoff or compensates resources created by that
operation only. Never use broad namespace/workspace deletion from an unresolved
variable. A failed activation restores the prior config/release. DNS does not
switch until health passes; lower TTL before planned cutover.

Cancellation first blocks new grants and caller access, then waits for in-flight
requests/outboxes, snapshots required evidence, and deletes ephemeral compute/DNS/
secrets only after legal/finance retention approval. Ledger, payment receipts,
audit events, and required usage records remain. A restore window and irreversible
deletion confirmation are explicit.

## Test plan

* Unit-test desired-state planning, naming, idempotency, transition/compensation,
  safe error mapping, and deletion target resolution.
* Kind/local fixture validates manifests, RBAC, network policies, resource quotas,
  readiness and rollback without AWS.
* EKS staging provisions the same tenant twice concurrently; assert one logical
  tenant/release/DNS/cert/token and no leaked temporary secret.
* Inject failure after each saga step and verify retry or exact compensation.
* Isolation suite reads/writes secrets, services, usage, config, grants, and admin
  APIs across two tenants; all cross-boundary attempts fail.
* DNS/TLS test verifies hostname, certificate chain/renewal alert, routing, no
  dangling record after canceled failed provision, and rollback to prior target.
* Quota/noisy-neighbor load test exhausts one tenant while another maintains SLO.
* Full terminal recording: fund fixture -> provision -> issue license/lease ->
  DNS if applicable -> API shapes -> settlement -> suspend -> restore -> cancel.

## Monitoring and definition of done

Track operation duration/age/retry by step, active desired-vs-observed version,
EKS/ingress/DNS/cert health, secret sync, quota saturation, smoke result, rollback,
and orphan resource count/cost. Done when both the selected launch path and its
cancellation/restore flow pass in staging with redacted Kubernetes/AWS evidence.
