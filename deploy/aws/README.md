# AWS EKS Delivery Identity Templates

These are reviewed examples, not deployable account infrastructure. Replace
only the clearly marked account, repository, environment, region, and approved
resource identifiers through an approved infrastructure-as-code system. Never
put AWS keys, cluster endpoints, repository policies, secret values, or actual
role ARNs in this directory.

`github-oidc-trust-policy.example.json` is a staging-role trust template. Give
production promotion its own role and replace the subject with the exact
`repo:<OWNER>/<REPOSITORY>:environment:production` subject. Do not broaden a
trust policy to `repo:*`, a branch wildcard, tags, pull requests, or a generic
repository claim. The workflow must use a pinned action revision and request
only `id-token: write` plus the minimum repository permissions.

`genai-smart-router-eks-discovery-role.example.json` defines the separately
named, read-only EKS discovery role. Replace the account and approved
IAM-user placeholder through reviewed infrastructure as code. The approved
local operator is named `smartrouter`; an organization may substitute a
federated role only through a separately reviewed trust-policy change.
Do not trust the account root, issue long-lived access keys, or use a root
session as the role source: AWS root credentials cannot assume roles. The
bootstrap administrator must create or nominate an MFA-protected non-root
principal, grant it only `sts:AssumeRole` for this role, and then replace any
temporary bootstrap trust before discovery begins.

The discovery role intentionally grants no EKS/Kubernetes mutation. EKS access
entries and namespace-scoped Kubernetes RBAC are separately required for
read-only API discovery. A tenant provisioner is a distinct workload identity;
it must be granted only the approved tenant namespace and bootstrap resources.

Define separate policies/roles for:

- discovery: `sts:GetCallerIdentity`, approved `eks:DescribeCluster`,
  `eks:ListAccessEntries`, `eks:ListNodegroups`, ECR repository inspection,
  and only the Kubernetes access entry/RBAC required for reads;
- image push: only the approved ECR repository upload and digest-read actions;
- staging reconciliation and production promotion: `eks:DescribeCluster` plus
  namespace-scoped Kubernetes RBAC, in separate environment-gated roles;
- workload identity: only the approved Secrets Manager ARN path and optional
  CloudWatch log/metric resources. Never use node-role credentials.

Require ECR tag immutability, private repository access, retention lifecycle,
scan visibility, and digest retrieval before a deployment pipeline consumes an
image. The pipeline story owns provenance, SBOM, signature enforcement, and the
shared command contract.

## EKS Staging Target Bootstrap

`genai-smart-router-eks-staging-target.json` is a checked-in bootstrap policy
for the delivery-contract test suite. Its ECR repository URI is **not** proof
of a currently approved live AWS/EKS target.

Before any EKS delivery command can be used outside the offline contract tests:

1. Renew a least-privilege, non-root AWS session through the secure login
   flow and confirm the approved non-production account, region, ECR
   repository, cluster, namespace, runtime Secret, non-secret Secret
   attestation ConfigMap, overlay, workload, and delivery role.
2. Update the reviewed JSON and the exact protected SSM Parameter named by the
   policy through approved infrastructure-as-code in one reconciliation
   change. The delivery role may read that Parameter but must not update it.
3. Run `make eks-preflight` with the approved role. Any schema, value, or hash
   mismatch fails closed before cluster selection, rendering, or mutation.

The current schema is version 5. It pins the full tagless source image name
that Kustomize must replace, the single runtime Secret name, the separately
bootstrap-owned runtime Secret attestation ConfigMap, and the workload target.
The delivery identity receives name-scoped `get` access only to that ConfigMap
and must have **no** Secret verbs. Kubernetes RBAC cannot make a Secret `get`
metadata-only: JSONPath filters output after the API has authorized and
returned the complete Secret.

The bootstrap identity alone creates a fresh immutable attestation ConfigMap
after it creates or updates the Secret. Its `data` must contain exactly the
safe scalar fields `schema_version: v1`, `secret_name`, `secret_uid`, and
`secret_resource_version`; it must have no `binaryData` and exactly one
same-namespace `v1` `Secret` owner reference matching the attested name and
UID. Delete the old attestation before changing the Secret, then create the
fresh immutable ConfigMap after the Secret change. Any absent, malformed, or
in-progress attestation blocks delivery. The delivery contract records only
the attested UID/resourceVersion and the ConfigMap UID/resourceVersion, never
Secret contents or the raw ConfigMap payload.

Reconcile the reviewed JSON and protected Parameter together before using this
contract against a cluster. Do not add the attestation ConfigMap to the
Kustomize delivery inventory: it is independent bootstrap state, not workload
desired state.

Do not infer authorization from an account number, repository URI, local AWS
profile, or this file alone. The protected Parameter and least-privilege
identity are the live approval boundary.
