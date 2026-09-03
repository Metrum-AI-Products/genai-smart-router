# Segregation of Duties Evidence Record

## Scope

This record supports parent issue **#961** and subissue **#970**. It assesses
segregation between code review, deployment execution, production approval, and
production access using only sanitized, checked-in repository material collected
on **2026-09-03**. It does not attest to live GitHub, cloud, or organization
configuration.

## Evidence collected

| Source path / URL | Collection date | Evidence type |
| --- | --- | --- |
| `.github/workflows/inspect-coding-evaluation.yml` | 2026-09-03 | Checked-in GitHub Actions workflow definition |
| `deploy/aws/README.md` | 2026-09-03 | Checked-in deployment-identity guidance |
| `deploy/aws/genai-smart-router-eks-discovery-role.example.json` | 2026-09-03 | Checked-in, non-deployable IAM policy example |
| `deploy/kubernetes/bootstrap/eks-discovery-namespace-rbac.example.yaml` | 2026-09-03 | Checked-in Kubernetes RBAC example |
| Google issue tracker | 2026-09-03 | Anonymous access returned HTTP 401; no private URL or content was collected |

## Demonstrated facts

### Repository-defined workflow boundaries

- The inspection workflow runs its smoke job for `pull_request` events and
  grants the workflow default `contents: read` permission. The smoke job has no
  declared `environment` and does not contain a deployment step.
- The workflow's `full-or-promotion` job runs only on its scheduled trigger or
  a non-smoke `workflow_dispatch` invocation. It declares
  `environment: evaluation`; that is a repository-defined mapping to the
  GitHub environment named `evaluation`.
- The workflow states that `promotion-review` is a manual
  protected-environment decision and that a completed benchmark is not an
  automatic target-weight or membership change. This is a checked-in workflow
  statement, not proof that the corresponding GitHub environment protection is
  configured.
- The full-or-promotion job has `contents: write` permission and can commit a
  sanitized evaluation report on successful completion. This is a repository
  capability that must be considered when reviewing who may invoke the job and
  which branches it may update.

### Repository-contained access-separation templates

- The AWS discovery-role example is explicitly tagged `environment:
  non-production`. Its allow policy is limited to the listed EKS describe/list
  actions and ECR repository/policy read actions; it grants no EKS or ECR
  mutation action.
- The same IAM example names a dedicated discovery operator principal and
  requires MFA in its trust condition. Its account, principal, cluster, and
  repository values are placeholders, so the file is evidence of an intended
  policy shape rather than a deployed identity.
- The Kubernetes discovery example uses a namespaced `Role` and grants only
  `get` and `list` on the listed discovery resources. Its `RoleBinding` refers
  to a replacement Kubernetes group in one replacement tenant namespace; it
  contains no write verb.
- The deployment-identity guidance calls for separate policies or roles for
  discovery, image publication, staging reconciliation and production
  promotion, and workload identity. It specifies that staging reconciliation
  and production promotion use separate environment-gated roles. The same
  guidance identifies the protected parameter and least-privilege identity as
  the live approval boundary.

These facts demonstrate repository-defined separation patterns and
least-privilege templates. They do **not** demonstrate that any GitHub
protection, IAM policy, Kubernetes binding, or production approval boundary is
currently active.

## Evidence gaps and compliance-owner handoff

- **GitHub organization/repository administrator:** Provide an authorized,
  sanitized export or screenshots of branch/ruleset protections, required
  pull-request approvals, required reviewer/CODEOWNER enforcement, and the
  roles or teams allowed to merge. The repository contains no checked-in
  GitHub settings proving reviewer segregation or that authors cannot approve
  their own changes.
- **GitHub environment owner:** Provide the `evaluation` and `production`
  environment protection settings, required reviewers, self-approval policy,
  deployment-branch restrictions, environment-secret access rules, and the
  relevant deployment audit records. The workflow's `environment: evaluation`
  field alone cannot establish any of those live controls; no checked-in
  production deployment workflow or production environment setting was
  available for review.
- **Cloud platform/IAM owner:** Provide sanitized, current production OIDC
  trust policies, role-assumption policies, permission boundaries, EKS access
  entries, Kubernetes RBAC bindings, and CloudTrail or equivalent deployment
  audit records. The reviewed IAM and RBAC files are examples with placeholders
  and cannot prove deployed production access segregation.
- **Release/change-approval owner:** Provide approved change records that link
  a reviewed code change, a distinct deployment executor, and a distinct
  production approver to the deployment event. Also confirm whether the
  workflow's `contents: write` report-commit capability is restricted to the
  intended trusted branch and actors.
- **Compliance owner:** Obtain an authorized, sanitized export from the Google
  issue tracker if it is needed as evidence. Anonymous collection returned HTTP
  401, so no assertion is made about tracker content, approvals, or status.

## Sanitization

This record contains only repository-relative paths, workflow and policy
behavior, placeholder identifiers already present in examples, and an HTTP
status boundary. It excludes secrets, credentials, customer data, production
URLs, account identifiers, private tracker content, and cloud resource
identifiers.
