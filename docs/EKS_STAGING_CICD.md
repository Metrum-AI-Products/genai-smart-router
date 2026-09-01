# Historical (superseded 2026-09-01): staging Make/CI delivery removed. Metrum
# production uses Fleet. See docs/EKS_PRODUCTION_OPERATIONS.md.

# EKS Staging CI/CD Foundation

This internal runbook defines the repository-side gate for a signed,
digest-pinned GenAI Smart Router staging deployment. It does not grant AWS or
Kubernetes access, create credentials, push an image, or apply a manifest.
Production promotion remains outside this document and #517.

This is the canonical internal home for source-control, workflow, and OIDC
delivery controls. Do not copy those implementation details into `docs-site/`;
customer-facing Kubernetes guidance stays focused on deployment requirements
and observable runtime behavior.

## Delivery boundary

The root `Makefile` is the only deployment interface for both an operator and
the future protected release workflow. A release must invoke these targets in
order:

1. `make eks-supply-chain-validate` — validate the immutable image digest and
   safe release-binding, SBOM, provenance, signature-verification, and scan
   evidence.
2. `make eks-preflight eks-plan` — establish the approved short-lived identity
   and run server-side dry-run against the explicit staging target.
3. `make eks-apply-staging EKS_CONFIRM=STAGING_APPLY` with the same
   `IMAGE_DIGEST` and `EKS_SUPPLY_CHAIN_DIR` as the supply-chain validation —
   only after protected staging-environment approval. The image architecture
   comes only from `image_architecture` in the reviewed checked-in staging
   target policy; delivery then requires that policy's canonical hash to match
   the protected Parameter copy. A caller or workflow must not supply or
   override it.
4. `make eks-rollout-status eks-smoke-staging` — retain scrubbed rollout and
   smoke evidence.
5. `make eks-promotion-plan` — after the validated live checks pass, write the
   raw operational record only to `EKS_EVIDENCE_DIR` and atomically emit the
   fixed `evidence-promotion-plan.safe.json` handoff only to the separate,
   non-overlapping `EKS_PROMOTION_EVIDENCE_DIR`. The protected release workflow
   copies only that safe file into its separately assembled production evidence
   bundle alongside the safe supply-chain and migration-rehearsal results; the
   production review identity never receives the raw EKS directory.

`make eks-rollback-staging` is also a mutating release path and has the same
`eks-supply-chain-validate` prerequisite. Its evidence bundle must bind the
same approved historical `IMAGE_DIGEST` and target-policy architecture as the
rollback request; `EKS_CONFIRM` and the approved pod-template hash do not
replace that validation.

The checked-in contract workflow must never duplicate `aws`, `kubectl`, or
`kustomize` deployment commands. It permits only its explicitly allowlisted
immutable checkout action; any other `uses:` action, including a pinned action,
is rejected so an action cannot hide deployment behavior. The verifier parses
valid YAML key spacing and quoted keys such as `run :`, `"run":`, `uses :`,
and `'uses':`, so formatting cannot hide a run body, action reference, or
permission override. It requires `make ci-eks-staging-contract` as the sole
direct, parsed executable command in a `run` step; a comment, step name,
echoed string, conditional, or failure-masking suffix does not satisfy the
gate. That command must be the literal argv `make ci-eks-staging-contract`:
leading assignments, wrappers, and path-qualified `make` executables are
rejected. It normalizes only simple block mappings and scalar lists, then
requires the complete non-comment workflow to match the reviewed fixed
grammar: exact triggers and watched paths, read-only permissions, one reviewed
runner/timeout job, and the two approved checkout/contract steps. It rejects
block scalars, flow values, explicit mappings, tags, anchors, aliases, merges,
document markers, and directives, so a second `run` or `uses` field cannot
bypass the narrow parser. It also fails closed on a shell expansion at
executable position. It rejects workflow, job, and step `env`, `defaults`,
`shell`, `working-directory`, `container`, `services`, `background`, `cancel`,
`wait`, `wait-all`, `parallel`, `if`, and `continue-on-error` settings, plus
all checkout `with` inputs and GitHub `environment` settings; it requires
exactly one reviewed GitHub-hosted `ubuntu-24.04` runner. A startup hook,
custom shell template, container entrypoint, unreviewed checkout source,
alternate Makefile location, injected environment context, background/cancel
lifecycle control, skipped gate, or masked failure therefore cannot execute or
bypass that parsed command. Its effective workflow permissions must be the
single top-level `contents: read` mapping with no job or step override. The Make
contract creates a temporary kubeconfig and resolves the account, region,
cluster, namespace, overlay, environment, and `image_architecture` only from
the reviewed checked-in target policy. Before delivery, it binds that complete
policy to the independently protected Parameter copy by canonical policy hash,
rejects mutable tags, and cannot mutate production. Those target properties are
never caller-set Make or workflow inputs.

## Supply-chain evidence

The build/signing system writes these JSON files to an ignored protected
directory, normally `tmp/eks-supply-chain/`:

| File | Required safe fields |
| --- | --- |
| `release-binding.intoto.json` | in-toto Statement v1 with predicate type `https://metrum.ai/attestations/eks-release-binding/v1`; its standard subject binds repository/name plus the `IMAGE_DIGEST` SHA-256, while its predicate records the exact policy-selected architecture and lower-case SHA-256 hashes of the raw `sbom.json`, `provenance.json`, and `scan.json` bytes |
| `sbom.json` | an unmodified supported SPDX 2.2/2.3 document (`SPDXRef-DOCUMENT`, `CC0-1.0` data license, name/namespace/creation info) with at least one non-empty recognized `packages` (`SPDXID`, `name`) or `files` (`SPDXID`, `fileName`) list, or an unmodified CycloneDX 1.4–1.6 BOM (`bomFormat: CycloneDX`, supported `specVersion`, positive integer `version`) with non-empty `components` (`type`, `name`) |
| `provenance.json` | an unmodified in-toto Statement v1 with SLSA provenance v1 predicate, structured build definition and run details, and a subject entry binding `subject[].name` to the image repository/name and `subject[].digest.sha256` to the digest portion of `IMAGE_DIGEST` |
| `signature-verification.json` | protected verifier result with `verified: true` and the exact lower-case `binding_sha256` of raw `release-binding.intoto.json` bytes; it attests to the separately signed binding statement rather than modifying an SBOM or SLSA provenance document |
| `scan.json` | approved-policy `verdict: pass`; its exact raw bytes are bound by `release-binding.intoto.json` |

`sbom.json` and `provenance.json` must remain standards-compatible documents:
do not add custom top-level `image` or `architecture` fields to either file.
The release-binding predicate is the required deployment-specific link between
the immutable image, approved target-policy architecture, and exact evidence
bytes.

Run the read-only verification before any EKS target:

```bash
make eks-supply-chain-validate \
  IMAGE_DIGEST='registry.example/smart-llmrouter@sha256:<64-hex>' \
  EKS_SUPPLY_CHAIN_DIR='tmp/eks-supply-chain'
```

The supply-chain verifier derives the expected architecture only from
`image_architecture` in the reviewed checked-in target policy. It validates the
release-binding subject, architecture, and raw artifact hashes, retains the
standard SLSA provenance subject check, and requires the independent verifier
result to bind the exact raw binding statement. This keeps deployment bindings
outside the standards-governed SBOM and SLSA provenance documents. Delivery
separately requires the complete policy's canonical hash to match the protected
Parameter copy before it can reconcile. Do not pass `EKS_IMAGE_ARCHITECTURE`,
or add an equivalent workflow input: caller-controlled architecture would
weaken the target-policy boundary.

On success, the verifier writes one safe JSON result to standard output. Its
only fields are the passed outcome, immutable image digest, policy-derived architecture,
the raw SHA-256 values for the release binding, SBOM, provenance, and scan,
the passed signature/scan verdicts, and a UTC timestamp. The protected release
system—not a caller-controlled Make argument—must capture that exact output in
a protected evidence file and hash-reference it as
`supply_chain.validation_result` for the production review gate. Raw evidence
files, signature material, scanner output, credentials, and configuration stay
in the protected build/release boundary and never appear in that result.

Do not commit evidence bundles. They must not contain runtime secrets, caller
tokens, provider keys, DSNs, router config, or raw scan logs. The validator is
a completeness and redaction gate; the protected build/signing/verifier system
remains responsible for actual SBOM/provenance generation, scanner policy
enforcement, DSSE/signature handling, and cryptographic verification.

## Promotion binding

Every digest-pinned `eks-render`, `eks-plan`, `eks-apply-staging`, and
`eks-smoke-staging` evidence result records `configuration_fingerprint`. It is
the SHA-256 hash of the rendered manifest with the immutable image normalized,
not a copy of router configuration or a secret. A passed `eks-promotion-plan`
first writes its detailed operational evidence in `EKS_EVIDENCE_DIR`, then
reduces the already-verified result to the separate safe handoff
`EKS_PROMOTION_EVIDENCE_DIR/evidence-promotion-plan.safe.json`.

That file contains exactly `schema_version: 1`, `outcome: passed`, `timestamp`,
`environment: staging`, `action: promotion-plan`, `image_digest`,
`configuration_fingerprint`, and
`promotion_plan_result: review_required_no_production_apply`. It intentionally
does not contain events, cloud/RBAC/resource metadata, command text, rendered
configuration, or raw tool output. A failed or invalid subsequent
`promotion-plan` invocation removes the previous safe handoff before input
validation, so it cannot remain usable after a failed recheck. The production
manifest must hash-reference this fixed filename and use its fingerprint. The
protected staging workflow and its safe-directory upload are the trust root;
the reducer is a sanitizer, not a substitute for that workflow's authorization
or evidence integrity controls.

## GitHub controls required before enabling deploy

The checked-in GitHub workflow only tests the contract. It has no AWS
credentials or deployment capability. Before adding a protected staging
release job, an administrator must:

1. Create a protected GitHub `staging` environment with required reviewer(s),
   deployment-branch policy, and environment-scoped non-secret variables for
   the approved region, cluster, namespace, overlay, ECR repository, and role
   ARN. Never put runtime secrets there.
2. Create an AWS GitHub OIDC provider if absent, then create a dedicated role
   from `deploy/aws/github-oidc-staging-role-trust-policy.example.json` after
   replacing only `<account-id>`. Keep the exact audience and
   `repo:sysadmin-metrum-ai/genai-smart-router:environment:staging` subject
   conditions; the subject itself binds owner, repository, and environment.
   The example must retain exactly one OIDC trust statement: the approved
   GitHub OIDC federated principal, `sts:AssumeRoleWithWebIdentity`, and those
   exact conditions. Do not add a broader Allow statement for another
   principal, action, repository, ref, or environment.
   Attach only the
   ECR and namespace-scoped EKS permissions required by the Make contract.
3. Configure a protected release workflow with `id-token: write`, minimal
   repository permissions, pinned actions, a staging-only concurrency group,
   and no long-lived AWS credentials. It must run the Make target sequence
   above and upload only scrubbed evidence.
4. Test expected denials: a pull request, another repository, another owner,
   a non-staging environment, a mutable tag, missing/failed supply-chain
   evidence, invalid manifest, and namespace RBAC denial must all stop before
   reconcile.

Use `docs/EKS_STAGING_MIGRATION.md` for the runtime-secret, RDS, license,
smoke, rollback, and recovery requirements. #516 owns approved target and
least-privilege identity validation; #518 owns production promotion.
