# Agent Orchestration Prompt — Prosumer Productization Sprint

Copy everything below the line into a new agent session (Cursor / Codex / Claude Code).  
Repo: `sysadmin-metrum-ai/genai-smart-router`  
Milestone: `productization-sprint-1`  
Epic: #520  
Canonical decisions: root `DECISIONS.md` (D3 is **decided**: subscription + included allowance + x402 overage)

---

## Mission

You are the **orchestrator agent** for the GenAI Smart Router Prosumer Launch. Your job is to implement the launch-blocking productization issues end-to-end, with parallel subagents in git worktrees, continuous testing, evidence attachments, PR review loops, and clean merges into `main`.

**Commercial product vision (must not drift):**

> Metrum-hosted / managed model-mix access = **base monthly subscription** (Stripe) + **model-mix-specific included usage allowance** (input + output tokens and any plan-defined billable units) + **x402 per-request payment and settlement** for usage beyond the included allowance.
>
> Stripe never runs on the router inference hot path. x402 verification/settlement is a bounded local admission step before upstream work.
>
> Prepaid credit packs and Stripe Billing Meters are **not** the primary overage rail (superseded 2026-07-19). Do not reintroduce them.

Read before coding:

- `AGENTS.md`
- `DECISIONS.md` (D1–D12)
- `docs/productization-sprints/2026-07-18-to-2026-07-25/README.md`
- **UX narratives (required for any UI/console work):** `docs/productization-sprints/2026-07-18-to-2026-07-25/ux/INDEX.md` and the matching `ux/issues/NNN-*.md` + HTML mock for the issue you implement. Do not invent screens that contradict those mocks.
- Each issue body you touch (source of truth for that work)

---

## Issue graph (implement in dependency order)

### Wave 0 — foundations (parallel)

| Issue | Title | Notes |
| ---: | --- | --- |
| #507 | data migration / schema versioning | if still open; required by almost everything |
| #516 | EKS delivery identity/bootstrap | if still open |
| #517 | signed immutable-image CI/CD to staging | if still open |
| #518 | evidence-gated EKS production promotion | if still open |
| #519 | Make workflow for EKS delivery | if still open |
| #527 | Release engineering for control plane + router | coordinates waves |

### Wave 1 — control plane + catalog + ledger (parallel after Wave 0)

| Issue | Title | Depends on |
| ---: | --- | --- |
| #521 | Control plane: accounts, tenant lifecycle, entitlement, lease | #507 |
| #540 | Plan catalog: SKUs, model-mix included allowance, x402 bands | #521 |
| #523 | Double-entry ledger: subscription + allowance + x402 | #507, #521 |

### Wave 2 — admission + payments (parallel after Wave 1 contracts freeze)

| Issue | Title | Depends on |
| ---: | --- | --- |
| #522 | Commercial admission: entitlement + allowance grants + x402 handoff | #521, #523, #540 |
| #506 | x402 per-request overage (launch-blocking) | #522, #540 |
| #524 | Stripe subscription checkout, webhooks, Portal | #521, #523, #540 |
| #526 | Hosted tenant provisioning on EKS | #521, #507, #516–#519 |

### Wave 3 — product surfaces (after Wave 2)

| Issue | Title | Depends on |
| ---: | --- | --- |
| #525 | Prosumer console | #521, #522, #523, #524, #506, #540 |
| #529 | Abuse / fraud / provider-cost controls | #521, #522, #523, #524, #506 |
| #531 | Observability, reconciliation SLOs, on-call | Wave 2 + #527 |
| #530 | Security / compliance / tenant isolation | #521, #524, #526, #527, #506 |

### Wave 4 — launch gate

| Issue | Title | Depends on |
| ---: | --- | --- |
| #533 | Docs, legal, pricing policy, support | all customer-visible blockers |
| #534 | Integrated beta: subscription + allowance + x402 E2E | #521–#527, #529–#531, #540, #506 |

### Fast-follow (do **not** block #534; schedule after launch gate or in idle capacity)

| Issue | Title |
| ---: | --- |
| #528 | Experimental model canary pipeline |
| #532 | AWS Marketplace strategy + hardened artifacts |
| #541 | Outcome-based model-mix auto-tune |
| #535 | Customer BYOK vault |
| #536 | Upstream connection onboarding + capability verification |
| #537 | Tenant routing studio |
| #538 | Governed private/custom upstream connectivity |
| #539 | BYOK commercial attribution |

Also keep enterprise self-hosted path intact (do not break offline `license.json`).

---

## Worktree + subagent rules

1. **Main checkout stays clean.** Orchestrator works from the primary clone only for planning, merging, and evidence index updates. Do not implement features in the dirty main worktree.
2. **One issue → one worktree → one branch → one PR.**
   - Worktrees live as **siblings** of the primary clone, under `../`:
     ```bash
     # from primary: /path/to/smart-llmrouter
     git fetch origin
     git worktree add -b agent/issue-NNN-short-slug ../smart-llmrouter-issue-NNN origin/main
     ```
   - Branch naming: `agent/issue-<num>-<short-slug>`
   - Worktree path: `../smart-llmrouter-issue-<num>`
3. **Launch parallel subagents** for independent issues within the same wave. Never start a dependent issue until its upstream PR is **merged to main** (or the dependency contract is explicitly stubbed behind a feature flag and the parent issue documents the contract freeze).
4. Each subagent receives:
   - exact issue number + title + body URL
   - `AGENTS.md` + `DECISIONS.md` D3 reminder
   - worktree path and branch name
   - “Definition of Done” from the issue
   - evidence requirements (below)
   - instruction to use `rtk` before shell commands in this repo
5. **No secrets in commits, issues, PR bodies, logs, screenshots, or evidence.** Never print provider keys, router tokens, token hashes, Stripe secrets, card data, license private keys, or full production config.

---

## Per-issue execution loop (every subagent)

```text
1. git fetch + rebase onto latest origin/main in the worktree
2. Implement only that issue's scope (no drive-by refactors)
3. Format: rtk gofmt -w <changed go files>
4. Test (minimum):
     rtk go test ./cmd/... ./internal/...
   Plus issue-specific tests (unit/integration/E2E) from the issue body
5. Update docs if behavior/config/API changed (internal + docs-site when customer-facing)
6. Commit with message:  "issue NNN: <imperative summary>"
   - Prefer small, frequent commits over one giant commit
7. Push branch and open PR:
     gh pr create --base main --title "issue NNN: ..." --body "$(cat <<'EOF'
     ## Summary
     - ...

     ## Issue
     Closes #NNN

     ## Commercial model check
     - [ ] Subscription + included allowance + x402 overage (no prepaid-primary drift)

     ## Test plan
     - [ ] unit/integration commands + results
     - [ ] staging/smoke if applicable

     ## Evidence
     - attached screenshots / console snapshots / redacted terminal replays
     (paths or uploaded PR assets)

     ## Risk / rollback
     - ...
     EOF
     )"
8. Attach evidence to the PR (screenshots, console snapshots, redacted terminal
   recordings). Prefer files under docs/productization-sprints/.../evidence/issue-NNN/
   committed in the PR when safe; otherwise upload as PR assets.
9. Enter the PR review loop (next section).
10. After merge: remove worktree + delete local/remote branch; update epic #520
    with a short progress comment linking the merged PR.
```

---

## PR review loop (mandatory)

For **every** open PR you own:

1. Poll review state:
   ```bash
   gh pr view <num> --json state,mergeable,reviewDecision,statusCheckRollup,reviews,comments,url
   gh api repos/{owner}/{repo}/issues/<pr-num>/comments
   gh api repos/{owner}/{repo}/pulls/<pr-num>/comments
   gh api repos/{owner}/{repo}/pulls/<pr-num>/reviews
   # reactions on the PR issue (thumbs-up means "no comments to address"):
   gh api repos/{owner}/{repo}/issues/<pr-num>/reactions --jq '.[] | {user:.user.login,content:.content}'
   ```
2. **Thumbs-up rule:** if the PR has a `+1` / `THUMBS_UP` reaction from a human reviewer **and** there are no unresolved review comments / requested changes, treat review as approved-for-addressing-none. Still require green CI and `mergeable` before merge.
3. If there **are** review comments or `CHANGES_REQUESTED`:
   - address every actionable thread in the worktree
   - push fixes
   - **reply on each thread** explaining how it was addressed (commit SHA + test coverage)
   - do **not** resolve threads unless explicitly asked
   - re-run tests and update evidence if behavior changed
4. When CI is green, review is approved (or thumbs-up with no open comments), and the issue DoD is met:
   ```bash
   gh pr merge <num> --squash --delete-branch
   ```
   Prefer squash unless the issue explicitly needs a merge commit. Never force-push `main`.
5. Cleanup:
   ```bash
   git worktree remove ../smart-llmrouter-issue-NNN
   git branch -D agent/issue-NNN-short-slug 2>/dev/null || true
   git fetch --prune
   ```
6. Poll interval: check open PRs at least every 15–30 minutes while working; always check before starting a new wave.

---

## Evidence standard (attach per issue / PR)

Minimum evidence pack under  
`docs/productization-sprints/2026-07-18-to-2026-07-25/evidence/issue-NNN/`  
(or PR uploads if binary-heavy):

| Kind | Examples |
| --- | --- |
| Terminal replay | redacted command + output for tests, smokes, migrations |
| Console / UI | screenshots of onboarding, allowance, x402 challenge UX, admin reports |
| API | curl of `/readyz`, `/v1/models`, Chat/Responses/Messages; 402 challenge sample (no secrets) |
| DB | redacted SQL invariant queries (ledger balanced, unique settlement) |
| Security | secret scan clean; metrics-admin 403 for ordinary caller |
| Rollback | how to revert the change |

**Redact always:** prompts, tool payloads, images, provider keys, router tokens/hashes, Stripe secrets, card data, signing keys, full config, connection strings, private hostnames when policy requires.

---

## Testing expectations by area

- **Go:** `rtk go test ./cmd/... ./internal/...` after every substantial change; broader `./...` when safe (ignore generated Harbor artifacts if they fail).
- **Migrations:** forward + backward compatibility with #507 rules; Postgres-primary (not SQLite-only shapes).
- **Stripe (#524):** test-mode Checkout + webhooks via Stripe CLI; browser success redirect alone must not activate entitlement.
- **x402 (#506):** fake facilitator + test-network smoke: challenge → pay → settle → exactly one upstream; allowance-sufficient path must skip x402.
- **Admission (#522):** concurrent reserve race never exceeds grant; control-plane outage still respects local grant/entitlement expiry.
- **Ledger (#523):** double-entry invariants; idempotent settlement replay.
- **Console (#525):** Playwright (or equivalent) for key states including allowance-exhausted + payment-required.
- **Docs (#533):** `rtk make docs-build` / `rtk make docs-qa` when `docs-site/` changes.
- **Release (#527/#534):** staging E2E from signup → subscribe → first request → allowance exhaust → x402 → cancel.

---

## Orchestrator daily checklist

- [ ] `git fetch origin` and note `main` HEAD
- [ ] List open PRs; run review loop; merge ready PRs
- [ ] Clean merged worktrees/branches
- [ ] Start next ready issues as parallel worktrees (respect dependency graph)
- [ ] Post progress comment on #520 with merged PRs + blockers
- [ ] Update evidence index file if you maintain one
- [ ] Never leave secrets in the tree; run a quick secret scan before PR

---

## Hard stop / escalate to human

Stop and comment on the issue / ask the user when:

- A `NEEDS-HUMAN` decision (D1–D2, D4 numbers, D5–D12 except D3 shape) blocks correct implementation
- Finance numbers for #540 are required for production activation (staging fixtures OK)
- D12 treasury/compliance blocks **live** x402 production enablement (staging facilitator OK)
- CI is red for reasons outside the PR
- Merge conflicts involve commercial invariants you cannot safely resolve
- Any request would put secrets in git or weaken admission safety (unbounded pooled spend)

---

## Success criteria for the orchestrator

1. All **launch-blocking** issues in milestone `productization-sprint-1` are closed with merged PRs and evidence.
2. #534 acceptance criteria pass in staging (subscription + included allowance + x402 E2E).
3. No prepaid-primary or Stripe-meter-primary drift remains in issue text, docs, or code paths for hosted overage.
4. All worktrees for merged issues are removed; no stale `agent/issue-*` branches on origin.
5. Epic #520 has a final go/no-go comment linking the evidence index.

---

## First actions (do these immediately)

1. Confirm repo root and `origin/main`.
2. Read `DECISIONS.md` and epic #520.
3. `gh issue list --milestone productization-sprint-1 --state open`
4. Inventory open PRs.
5. Start Wave 0 / Wave 1 worktrees for the highest-priority ready issues (`#521`, `#540`, `#523`, `#527` foundations as applicable).
6. Do not begin #534 until its dependencies are merged.

Begin.
