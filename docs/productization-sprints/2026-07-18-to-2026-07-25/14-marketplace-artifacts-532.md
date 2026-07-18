# 14 — AWS Marketplace and Hardened Delivery Artifacts (#532)

Plan version: **v1.0.0**  
Issue: [#532](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/532)  
Classification: **fast follow unless D5 selects Marketplace as launch channel**

## Decision and design approach

Produce a reviewed channel ADR before implementation:

* **SaaS listing:** buyer subscribes/registers, control plane resolves the customer,
  maps entitlement to organization/tenant, provisions hosted service, and submits
  usage asynchronously to AWS. Best fit for Metrum-managed Prosumer/private offers.
* **AMI:** Packer builds a hardened binary-install image for buyer-managed EC2;
  signed offline/online license and BYOK fit, but this is not a managed EKS instance.
* **Container/EKS:** buyer deploys signed image/Helm artifacts in their account;
  use AWS entitlement/metering or contract license appropriate to selected pricing.
* **Parallel BYOC:** enterprise path; do not conflate with pooled-provider prepaid
  hosted credits.

Recommendation remains SaaS/private offers after direct Stripe launch, with AMI
or container artifacts for enterprise BYOC. Marketplace billing/metering is a
commercial-control-plane integration and never an inference hot-path dependency.

AWS documents `BatchMeterUsage` for SaaS usage and its hourly deduplication model:
[SaaS metering](https://docs.aws.amazon.com/marketplace/latest/userguide/metering-for-usage.html).
Container products use Metering Service/License Manager according to pricing:
[Container billing integration](https://docs.aws.amazon.com/marketplace/latest/userguide/container-products-billing-integration.html).

## Customer flows

### SaaS

Buyer accepts public/private offer -> Marketplace registration callback -> server
resolves/validates buyer -> sign in/create or link Metrum organization -> create
entitlement source -> fund/limit policy according to offer -> provision via #526
-> issue key -> use hosted endpoint -> ledger aggregates Marketplace dimensions ->
asynchronous metering/reconciliation -> unsubscribe stops new entitlement/grants
per contract while preserving required access/notice/records.

### AMI/container BYOC

Buyer subscribes -> launches signed artifact with IAM role -> entitlement check/
license bootstrap -> supplies own provider secret references and runtime config ->
starts router -> validates `/version`, `/readyz`, `/docs`, `/v1/models` and API
smokes -> reports required metering outside request handler -> upgrades/rolls back
using documented immutable artifacts.

## Human interaction and configuration

Commercial/Product/Finance approve D5/D7, listing type, dimensions, currency/
pricing/contract, private offers, free trial, refunds, taxes, seller terms and
support. Security/Compliance approve seller/account roles, artifact scan, buyer
data exchange, IAM and claims. SRE owns artifact, regional availability, metering
retries/reconciliation and buyer deployment support. Legal approves listing copy.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Required accounts
are AWS Marketplace seller/management, dev/test buyer, ECR/AMI build, KMS signing,
S3 artifact/evidence and support. SaaS uses Marketplace product code, registration
URL, ResolveCustomer/entitlement/metering service IAM role, pricing dimension IDs,
offer/agreement/customer safe IDs, SQS/outbox and reconciliation identity. AMI uses
Packer build role, base AMI allowlist, scanner/signing, instance profile and license
bootstrap. Container uses ECR product repos and EKS IRSA as required by current AWS
rules. No long-term AWS keys, provider keys, router tokens or license private keys
are baked into artifact or Helm values. Non-secrets include product code/dimensions,
supported regions/architectures, image/AMI digest/version, support URLs, entitlement
cache/grace, metering hour/batch/retry, license template and offer mapping.

## Artifact requirements

Packer/containers start from allowlisted patched base, non-root where supported,
minimal packages/ports, encrypted storage guidance, IMDSv2, least-privilege role,
no secrets/state/logs, deterministic version metadata, SBOM/provenance/signature,
critical-CVE gate, package validation and documented backup/upgrade/rollback. AMI
derives from the checked-in binary-install path; container derives from the
release Dockerfile/package, not an ad hoc fork.

## Test plan

* ADR scorecard validates customer experience, unit economics, operations,
  entitlement, metering, private offers, BYOK/pooled-key and regional constraints.
* Marketplace sandbox/dev buyer flow: subscribe/register/link/provision/use/meter/
  reconcile/unsubscribe; repeat callbacks/events and simulate unordered/delayed data.
* SaaS metering aggregates the exact approved dimension/hour, retries with stable
  identity, reconciles AWS reports to ledger, and never calls AWS in router handler.
* Packer builds twice from pinned inputs; validate boot, IMDSv2, ports/users,
  no-secret scan, SBOM/signature/CVE, license/BYOK config, API smokes, reboot,
  backup/upgrade/rollback and deprovision.
* Container/Helm install on clean EKS buyer fixture with least-privilege IRSA,
  network/secret isolation, entitlement/metering, smokes, upgrade/rollback/uninstall.
* Tampered/unsigned image/AMI, unauthorized product code, expired entitlement,
  metering outage, duplicate usage and buyer configuration mistakes fail according
  to documented safe behavior.

## Monitoring, rollback, and definition of done

Track registration/entitlement/metering success/lag/rejection, agreement changes,
reconciliation delta, artifact scan/build, buyer smokes and support incidents.
Unpublish/limit new offers before destructive rollback; preserve current buyer
contract rights and metering evidence. Done only after Marketplace validation,
Finance reconciliation, Security artifact review and one end-to-end test buyer
flow. Do not publish a listing merely because artifacts build.
