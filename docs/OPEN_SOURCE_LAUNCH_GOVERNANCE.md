# Open-Source Launch Governance Evidence

This record covers governance and community gates for the planned 2026-09-07
open-source launch. It incorporates the final launch-authority facts recorded on
2026-09-04. It is not a release announcement, a signed tag, a GitHub Release,
permission to deploy, or permission to change repository visibility.

## Current decision: release notes approved; operational GO pending

The release notes and launch-governance record are approved for **v1.0.0**. CEO
Steen Graham gave the final governance **GO** on 2026-09-04 for the planned
2026-09-07 open-source launch, including the time-bounded residual-risk
acceptance below. The v1.0.0 release has **not been deployed or published**.
Issue [#1052](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1052)
records a stale live revision and no resolvable production Fleet profile for the
current release. Post-deploy verification is therefore the remaining operational
GO condition. This record does not create or approve a tag, publish a GitHub
Release, deploy the router, or authorize changing repository visibility.

> **CEO approval — 2026-09-04:** Steen Graham, CEO, approved the GenAI Smart
> Router v1.0.0 release notes and final launch-governance GO, including the
> documented docs-build dependency residual risk through its 2026-10-04 recheck
> deadline. Deployment, publication, and post-deploy verification are outside
> this approval record.

The semantic version is derived from repository release policy and history:
there are no prior release tags, and the initial-release package and clean-build
contracts consistently use `v1.0.0`. No tag or GitHub Release exists yet.

## Accountable owner

Chetan Gadgil is the approved Launch Lead, Release Manager, Security Owner,
Operations owner, and Community owner. In those roles, Chetan holds launch stop
and rollback authority. Chetan Gadgil (`chetan@metrum.ai`) also owns support
coverage for launch day, 2026-09-07, and the first business day after launch,
2026-09-08. Public support remains in GitHub issues, private inquiries use
`contact@metrum.ai`, and vulnerabilities use the private security-reporting
path.

## Gate evidence

| Issue | Gate | Current evidence | State and exit condition |
| --- | --- | --- | --- |
| [#985](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/985) | LCH-02 | CEO Steen Graham approved the v1.0.0 release notes and final launch-governance GO on 2026-09-04. Chetan Gadgil retains stop/rollback authority. | **Governance GO / closed.** Deployment, publication, and operational verification remain separate gates. |
| [#986](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/986) | LCH-03 | Release-management ownership and approved version v1.0.0 are recorded. A signed tag, complete artifact SHA-256 set, final release URL, and approved announcement copy are not recorded. | **Open.** Freeze and record all remaining acceptance fields together without publishing the release. |
| [#987](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/987) | LCH-04 | The final governance GO includes the time-bounded docs-build dependency exception recorded below. Chetan Gadgil owns follow-up and recheck by 2026-10-04. Issue #1052 remains an unresolved operational GO blocker and has not been excepted. | **Open.** Close only after #1052 post-deploy verification resolves the remaining blocked gate. |
| [#988](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/988) | LCH-05 | Chetan Gadgil owns support coverage for launch day, 2026-09-07 UTC, and the first business day after launch, 2026-09-08 UTC; privacy-safe response channels are documented. | **Confirmed / closed.** The named dates are the coverage windows in the launch record's UTC time zone. |
| [#1018](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1018) | COM-04 | GitHub and public-release access have been verified. The repository remains private by explicit instruction; `main` remains the default branch and GitHub detects Apache-2.0. | **Open.** Visibility must not be changed. Complete the remaining approved public-presentation metadata only when separately authorized. |
| [#1052](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1052) | Operational GO | Production is healthy but runs a stale revision; no production Fleet profile currently resolves the deployment path for v1.0.0. | **Open / remaining GO condition.** Deploy v1.0.0 through the documented production authority, then verify rollout identity, `/version`, readiness, authenticated model discovery, and the repository smoke before recording operational GO. |

## Repository metadata proposal

The following values are grounded in the repository's public README and may be
applied by an authenticated owner without changing repository visibility:

- Description: `Self-managed Go proxy for policy-based routing across OpenAI, Anthropic, and private OpenAI-compatible model servers.`
- Topics: `anthropic`, `generative-ai`, `go`, `kubernetes`, `llm`, `llm-router`, `openai`, `reverse-proxy`.
- Default branch: retain `main`.
- License: retain the root Apache-2.0 `LICENSE` and verify GitHub detection.

No homepage is proposed until a stable public hosted-docs URL is confirmed.
No social-preview asset is approved by this record. Discussions should remain
unchanged unless a community owner separately approves the moderation and
staffing commitment. Repository visibility must remain unchanged.

## Accepted residual dependency risk

CEO Steen Graham's 2026-09-04 final GO accepts the following residual risk for
v1.0.0 through 2026-10-04. This is the decision authority; no separate Security
Owner signature is asserted.

- Inventory: 18 High dependency-chain records in the locked docs build, all
  rooted in `image-size@2.0.2` through `@docusaurus/mdx-loader@3.10.2`.
- Advisories: GHSA-w3rx-r6r6-pgpr / CVE-2025-71330 and
  GHSA-5p2g-fcmc-qvqq / CVE-2025-71329. No patched version was available in the
  existing advisory evidence.
- Reachability and threshold: accepted only as a docs build-time denial-of-service
  risk from crafted ICNS, JXL, or HEIF inputs. It is not executable code in the
  published browser bundle and is not accepted for the router or admin runtime.
- Mitigating controls: build docs only from reviewed repository content; reject
  untrusted image contributions before build; keep CI jobs time-bounded; retain
  the generated static docs artifact as the serving boundary; and continue
  clean locked installs, audits, and docs builds.
- Accountable follow-up owner: Chetan Gadgil.
- Expiry/recheck: 2026-10-04. At or before that date, recheck the aligned stable
  Docusaurus dependency set and upstream replacement status, rerun the locked
  audit and build, and either remediate or obtain a new explicit disposition.

## Final review record

- Decision: release notes and launch governance `GO`; operational `GO` pending post-deploy verification
- Decision date: 2026-09-04
- Approver and review participant: Steen Graham, CEO
- Scope: GenAI Smart Router v1.0.0 release notes and launch governance for the planned 2026-09-07 launch
- Release state: release-notes approved; not tagged, not published, and not deployed
- Remaining GO condition: resolve #1052 by deploying the current approved release through the documented production authority and recording sanitized post-deploy identity, `/version`, readiness, authenticated `/v1/models`, and repository-smoke evidence
- Repository state: GitHub/public-release access verified; visibility remains private and must not be changed
- AMD validation: completed; the repository's AMD Instinct runbook and reference architecture remain the operational evidence locations
- Accepted residual risks: the docs-build dependency inventory above is accepted through 2026-10-04 under the stated threshold and controls
- Decision attribution: CEO Steen Graham; no separate Security Owner signature is asserted
- Residual-risk follow-up owner: Chetan Gadgil
- Stop/rollback authority: Chetan Gadgil
- Support coverage owner: Chetan Gadgil for launch day 2026-09-07 UTC and the first business day 2026-09-08 UTC
