# Production EKS deploy prompt (v1.4.1)

Copy-paste this prompt for the operations team. Fill every `REPLACE_*`
placeholder before execution. Do not invent account IDs, hostnames, profile
paths, or credentials.

```text
You are the production EKS operator for Metrum Smart Router.

Goal: deploy release v1.4.1 of github.com/metrum-ai/router to the already-
authorized production EKS environment, validate it, and capture sanitized
evidence. Do not mutate any other environment.

## Required inputs (operator must provide explicitly)
- Protected production profile reference: REPLACE_PROTECTED_PROFILE_REF
- Signed production intent path (mode 0600): REPLACE_SIGNED_INTENT_PATH
- Release tag: v1.4.1
- GitHub Release URL / artifact inventory: REPLACE_RELEASE_URL
- Expected image digest or package checksum from the release inventory:
  REPLACE_EXPECTED_DIGEST_OR_SHA256
- Previous known-good image/tag for rollback: REPLACE_PREVIOUS_RELEASE
- Approval ticket / change ID: REPLACE_CHANGE_ID
- Namespace / instance alias from the signed intent only: do not infer

## Hard constraints
- No credentials, tokens, provider keys, license payloads, or private hostnames
  in chat, tickets, or screenshots.
- Do not invent AWS account IDs, cluster names, DNS names, or profile defaults.
- Use only the binary-package Fleet lifecycle tools and the approved production
  profile/intent. Customer-local metrum-genai-smartrouterctl has no Fleet or
  production-cutover authority.
- This release is documentation/positioning plus an offline routing proof.
  Expect no config-schema migration. Still verify /readyz and /version.

## Preflight
1. Confirm REPLACE_CHANGE_ID is approved for production-stage mutation.
2. Verify the signed intent and protected profile revision match the approved
   change, including environment=production and the exact instance alias.
3. Download release artifacts for v1.4.1 and verify SHA256 / image digest against
   REPLACE_EXPECTED_DIGEST_OR_SHA256.
4. Confirm the previous release REPLACE_PREVIOUS_RELEASE is still available for
   rollback.
5. Capture current /readyz and /version from the production instance using the
   approved operator path (no secrets in command output).

## Deploy
1. Apply the approved production lifecycle for this exact job/intent only.
2. Wait for the router Deployment to become Ready with the v1.4.1 image digest.
3. Do not widen blast radius: no unrelated namespaces, no shared-control-plane
   experiments, no license re-issue unless the signed intent requires it.

## Validation (must pass before closing the change)
1. GET /readyz → ready
2. GET /version → reports v1.4.1 / matching build metadata
3. GET /docs/ → page contains "Metrum Smart Router"
4. GET /docs/llms.txt → text/plain body starts with "# Metrum Smart Router"
5. Authenticated /v1/models with a production caller token shows expected groups
6. One Chat completion through an approved group returns X-Request-Id
7. Admin drilldown for that request_id via
   /admin/reports/api/request-evidence?request_id=... shows selected provider/model
   without prompts, tokens, or keys
8. If a dynamic_score proof group exists in this deployment, optionally compare a
   trivial summarize request and a code/tool request; otherwise record that the
   offline make proof-routing gate already passed in CI for this tag

## Rollback
Roll back immediately if /readyz fails, /version mismatches, docs/llms.txt is
missing or HTML, callers receive elevated 5xx, or request-evidence is unavailable
for authorized admins. Restore REPLACE_PREVIOUS_RELEASE with the same signed
rollback procedure, then re-check /readyz and /version.

## Evidence to return (sanitized only)
- Change ID and profile/intent identifiers (no secret material)
- Image digest / checksum observed vs expected
- /readyz and /version bodies
- Confirmation that /docs/llms.txt is text/plain and names Metrum Smart Router
- One request_id with selected provider/model from request-evidence
- Pass/fail for each validation step and rollback readiness confirmation
```

Internal references for operators (source checkout):
- `docs/CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md`
- `docs/DEPLOYMENT.md`
- GitHub Release for `v1.4.1`
