# Branch protection for Launch A (manual repo-admin gate)

Protect `main` so the normal contributor path cannot merge with failing required checks.

Required status checks (names must match workflow job names):

- `test-and-vet` (Go workflow)
- `docs-qa` (Docs QA workflow)
- `source-sbom-and-vulnerabilities` (Security evidence)
- `check-source-headers` (Source headers)
- DCO check job from `.github/workflows/dco.yml`

Also require:

- At least one approving review
- Dismiss stale reviews on new commits
- Restrict who can push / bypass (admins only, or none)
- Do not allow force pushes or deletions

Negative gate test (closure evidence for U02/A2):

1. Open a PR that fails `go test` or `make docs-qa`.
2. Confirm the merge button remains blocked.
3. Record the PR URL and settings screenshot/API dump with the release evidence pack.

GitHub CLI sketch (adjust team/org as needed):

```bash
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/metrum-ai/router/branches/main/protection" \
  -f required_status_checks='{"strict":true,"contexts":["test-and-vet","docs-qa","source-sbom-and-vulnerabilities"]}' \
  -F enforce_admins=true \
  -f required_pull_request_reviews='{"required_approving_review_count":1,"dismiss_stale_reviews":true}' \
  -F allow_force_pushes=false \
  -F allow_deletions=false
```

Settings are mutable and not captured by a git commit; keep evidence outside the tree.
