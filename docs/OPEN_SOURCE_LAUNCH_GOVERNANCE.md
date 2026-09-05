# Open-Source Launch Governance Evidence

This record covers governance and community gates for the 2026-09-07
open-source launch window. It incorporates the final launch-authority facts
recorded on 2026-09-04 and the 2026-09-05 operational closeout. Repository
visibility remains **private** unless separately authorized.

## Current decision: objective GO (private Release)

The release notes and launch-governance record are approved for **v1.0.0**. CEO
Steen Graham gave the governance **GO** on 2026-09-04 for the planned
2026-09-07 launch, including the time-bounded residual-risk acceptance below.
On **2026-09-05 UTC**, Launch Lead Chetan Gadgil recorded **objective GO**
after:

- Production Fleet `llm-api` serving freeze
  `a77be222e2c35c2ec839e4be09387a9cbc57365f` (`/version` `v1.0.0`)
- Annotated SSH-signed tag `v1.0.0` verifying to that commit
- Private GitHub Release
  https://github.com/sysadmin-metrum-ai/genai-smart-router/releases/tag/v1.0.0
- Formal dated closes for remaining LGL/SEC/REL/OPS/DAY/LCH gates, including
  SEC-01 caller rotation evidence

> **CEO approval — 2026-09-04:** Steen Graham, CEO, approved the GenAI Smart
> Router v1.0.0 release notes and final launch-governance GO, including the
> documented docs-build dependency residual risk through its 2026-10-04 recheck
> deadline. Deployment, publication, and post-deploy verification were outside
> that approval record and are now evidenced separately (2026-09-05).

This record does **not** authorize changing repository visibility.

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
| [#985](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/985) | LCH-02 | CEO Steen Graham approved the v1.0.0 release notes and final launch-governance GO on 2026-09-04. Chetan Gadgil retains stop/rollback authority. | **Governance GO / closed.** |
| [#986](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/986) | LCH-03 | Freeze `a77be22…`, signed tag `v1.0.0`, Release URL, and artifact digests recorded 2026-09-05. | **COMPLETE / closed.** |
| [#987](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/987) | LCH-04 | Objective GO after prod==freeze, tag/Release, formal gate closes, SEC-01 rotation. Docs-build residual accepted through 2026-10-04. | **GO / closed.** |
| [#988](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/988) | LCH-05 | Chetan Gadgil owns support coverage for 2026-09-07 UTC and 2026-09-08 UTC. | **Confirmed / closed.** |
| [#1018](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1018) | COM-04 | Repo remains private; Apache-2.0 detected; default branch `main`. | **COMPLETE / closed** (private disposition). |
| [#1052](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1052) | Operational GO | Ownership transition and production profile resolved earlier; v1.0.0 freeze deployed 2026-09-05. | **COMPLETE / closed.** |

## Time-bounded residual risk (docs-build)

CEO Steen Graham's 2026-09-04 final GO accepts the following residual risk for
the docs build through **2026-10-04**:

- Unpatched build-time image metadata dependency risk in the docs build
- Not executable in the published browser bundle
- Mitigations: reviewed documentation inputs, bounded build jobs, static
  generated docs as the serving boundary
- Accountable follow-up owner: Chetan Gadgil
- Exit: remediates or obtains a new explicit disposition by 2026-10-04

## Decision log

- 2026-09-04: release notes and launch governance `GO` (Steen Graham); operational GO pending deploy/publish
- 2026-09-05: objective operational `GO` (Chetan Gadgil) after freeze deploy, signed private Release, and formal gate closes
- Approver (governance): Steen Graham, CEO
- Residual-risk follow-up owner: Chetan Gadgil
- Stop/rollback authority: Chetan Gadgil
- Support coverage owner: Chetan Gadgil for 2026-09-07 UTC and 2026-09-08 UTC
