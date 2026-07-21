# Production Promotion Contract

This is the repository-side review gate for issue #518. It records only safe,
digest-pinned metadata and deliberately cannot deploy, access AWS/EKS, migrate a
database, copy state, edit DNS, or use credentials. The EC2/Compose router
remains the production authority until an explicitly approved cutover.

## Required evidence

Create a protected, non-committed release manifest using
`deploy/release/production-release-manifest.schema.json`. It binds an immutable
image digest to passed staging and migration-rehearsal evidence, one
hash-pinned safe staging supply-chain validation result, a redacted
configuration fingerprint, a bounded canary, a known-good rollback artifact,
and two distinct time-bounded approver references.

```bash
PRODUCTION_RELEASE_MANIFEST='tmp/release/production-manifest.json' \
PRODUCTION_EVIDENCE_ROOT='tmp/release/evidence' \
  make production-promotion-validate
```

Both variables are mandatory: no default fixture can pass a production review.
The Make target receives them as literal inherited environment values; it does
not interpolate either operator-supplied path into a shell recipe or echo it in
normal target output. This keeps a malformed or sensitive-looking path from
becoming shell syntax or appearing in a release log.
Each staging, rehearsal, or validation-result reference must resolve beneath the
protected evidence root and hash to the recorded SHA-256. Passed staging and
rehearsal evidence must record `outcome: passed`. The production validator
accepts exactly one safe supply-chain validation-result payload: its promoted
image digest, policy-derived architecture, release-binding/SBOM/provenance/scan
SHA-256 values, `signature_verified: true`, `scan_verdict: pass`, and an RFC3339
timestamp. It validates that result's exact shape, resolved-byte hash, promoted
image binding, approved-target architecture, and freshness; it does not
dereference or reinterpret the raw SBOM, provenance, scan, release-binding, or
signature artifacts.

The staging release-binding validator (the #557 contract) validates those raw
protected artifacts before emitting the safe result. That boundary requires the
release binding to connect the approved target policy, immutable image, and
artifact hashes; validates the supported SBOM and provenance structures; and
requires independent signature verification and a passing scan. Keep the raw
artifacts inside the protected build/release boundary: they are not promotion
manifest fields or promotion-gate inputs.

The validator rejects duplicate JSON members before it examines content, expired
or longer-than-one-hour approvals, mutable or changed evidence, absent evidence,
a failed signature or scan result, an unsafe canary, unapproved migration
compatibility, and any external payload with a missing, changed, or additional
field. Its only successful result is `review_required_no_production_apply`.
Approval expiry is evaluated against the validator host's UTC clock; the command
has no caller-supplied time override. The rollback pull reference must carry a
distinct, previously known-good SHA-256 content digest, never the image under
review through another registry alias.

Every passed staging, migration-rehearsal, and supply-chain validation-result
payload must include a safe RFC3339 timestamp that is not in the future and is
no more than 24 hours old relative to the validator host's UTC clock. A new
approval cannot refresh old staging or supply-chain evidence; regenerate and
hash-pin the current protected evidence bundle.

The approved staging target policy, rather than the promotion manifest or a
caller-supplied setting, selects the target architecture. The safe validation
result records that policy-derived architecture, and the promotion validator
checks it against the reviewed target policy. A multi-platform image digest does
not make evidence for another target platform interchangeable.

Raw evidence is never a promotion-gate input. `make eks-promotion-plan` requires
separate, non-overlapping `EKS_EVIDENCE_DIR` and
`EKS_PROMOTION_EVIDENCE_DIR` directories. The raw directory retains the detailed
staging record for the protected staging workflow. After a verified staging plan
succeeds, the reducer atomically writes only
`evidence-promotion-plan.safe.json` to the safe directory. A failed or
superseded plan removes that handoff. The protected release workflow copies
that one file into `PRODUCTION_EVIDENCE_ROOT` alongside the separately safe
supply-chain validation result and migration-rehearsal result. The production
gate reads that combined protected bundle, never the raw EKS directory.

The staging manifest reference is fixed to
`evidence-promotion-plan.safe.json`. Its referenced JSON must contain exactly:
`schema_version: 1`, `outcome: passed`, an RFC3339 `timestamp`,
`environment: staging`, `action: promotion-plan`, `image_digest`,
`configuration_fingerprint`, and
`promotion_plan_result: review_required_no_production_apply`. The migration
rehearsal payload likewise has an exact schema: `schema_version: 1`,
`outcome: passed`, `timestamp`, `image_digest`, `migration_id`, and
`configuration_fingerprint`. The supply-chain result retains its separately
defined exact safe schema. No arbitrary fields, event arrays, command text,
cloud metadata, RBAC results, rendered manifests, or raw provider output are
parsed by this gate.

The reducer is a sanitizer, not an authenticity mechanism. The protected
workflow that performs the staging checks, controls the separate safe directory,
and uploads the hash-pinned bundle is the trust root. The promotion gate checks
the exact payload schemas, byte hashes, freshness, and cross-artifact bindings;
it cannot make an untrusted writer trustworthy. Fixed manifest identifiers and
references are additionally rejected when they look like bare credentials, so a
syntactically valid secret cannot reach successful validator output.

For an EKS staging delivery, use the non-secret `configuration_fingerprint`
written in the safe promotion projection as the manifest's
`configuration.fingerprint`. It is a SHA-256 hash of the rendered manifest with
the image normalized; the separately bound immutable image and that fingerprint
together prevent reuse of earlier staging evidence.

Never commit the real manifest or include provider keys, caller tokens/token
hashes, Basic credentials, DSNs, database exports, raw configs, customer data,
DNS records, cloud identities, or command transcripts. The checked-in fixture
is synthetic CI test data and cannot authorize a promotion.

## Human approval defaults

Use a protected production environment with two distinct approvers: release
owner and operations owner. The manifest records distinct safe identifiers for
both roles; the protected environment must independently authenticate and
enforce those approvals rather than trusting manifest text. Require a change
reference, one-hour approval expiry, a 5% canary for 30 minutes, and a rollback
artifact recorded before the window. A real cutover additionally requires the #507 migration classification,
backup/restore verification, explicit EC2 write-freeze or source-of-truth
decision, customer acceptance, and an approved rollback window.

## Execution boundary

After this gate passes, an external protected workflow may request the human
decision; it must not infer authorization from the manifest. Any later EKS,
database, state, or DNS action needs a separate approved runbook and
least-privilege identity. See `docs/EKS_STAGING_MIGRATION.md` for current
staging constraints and `docs/EKS_STAGING_CICD.md` for staging delivery.
