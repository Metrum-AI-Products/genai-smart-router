# Open-Source Launch Governance Evidence

This record covers governance and community gates for the planned 2026-09-07
open-source launch. It records only evidence available on 2026-09-03. It is not
a release announcement, release approval, final hard-gate review, or permission
to make the repository public.

## Current decision: NO-GO

The current evidence supports **NO-GO**. The final LCH-02 review has not occurred,
the release set required by LCH-03 has not been frozen, and the LCH-04 disposition
of every blocked launch item has not been recorded. This is an interim evidence
review, not the final hard-gate review. It must not be used to close a gate that
requires a GO decision or completed release evidence.

## Accountable owner

Chetan Gadgil is the approved Launch Lead, Release Manager, Security owner,
Operations owner, and Community owner. In those roles, Chetan holds launch stop
and rollback authority. These appointments identify accountability but do not
attest that a gate passed, that launch-day coverage is confirmed, or that a
release may be published.

## Gate evidence

| Issue | Gate | Current evidence | State and exit condition |
| --- | --- | --- | --- |
| [#985](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/985) | LCH-02 | Owner and stop/rollback authority are named here. No completed meeting record, timestamp, attendee list, full hard-gate evidence set, or accepted-residual-risk record exists. | **NO-GO / open.** After every hard gate has an evidence link and owner attestation, conduct the final review and record its decision, timestamp, attendees, accepted residual risks, and stop/rollback authority. |
| [#986](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/986) | LCH-03 | Release-management ownership is named. Merged preparation work does not establish one approved launch commit, version, signed tag, complete artifact SHA-256 set, final release URL, or approved announcement copy. | **NO-GO / open.** Freeze and record all acceptance fields together without publishing the release. |
| [#987](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/987) | LCH-04 | Launch decision ownership is named. No complete register proves that every blocked gate is resolved or covered by an explicit, expiring exception with mitigating controls. | **NO-GO / open.** At the final review, link resolution evidence for every blocked item or record the owner's explicit exception, expiry, and mitigating controls. No unapproved blocked item may remain. |
| [#988](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/988) | LCH-05 | Community ownership and privacy-safe response channels are documented. The intended coverage dates are 2026-09-07 and the first business day after launch, 2026-09-08. Availability and time-zone coverage have not been confirmed. | **Open.** Record the owner's confirmed availability windows and time zone for both dates. Use GitHub issues for public support, `contact@metrum.ai` for private inquiries, and the private security-reporting path for vulnerabilities. Do not publish personal contact details. |
| [#1018](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/1018) | COM-04 | The 2026-09-03 GitHub review recorded a private repository, `main` as default, Apache-2.0 license detection, and empty description, homepage, and topics. Social-preview approval was not evidenced. | **Open.** Apply and verify only approved, public-doc-grounded metadata; separately approve/upload a social preview. Recheck for internal or customer information before any later visibility change. |

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

## Final review record (unexecuted template)

Complete this section only after all hard gates have evidence and attestations:

- Decision: `GO` or `NO-GO`
- Decision timestamp in UTC:
- Attendees:
- Launch commit, version, signed tag, artifact digests, and release URL:
- Hard-gate evidence links and owner attestations:
- Accepted residual risks, each with owner, expiry, and mitigating controls:
- Stop/rollback authority: Chetan Gadgil, unless a later approved record changes it
- Communications coverage windows and time zone:

Leaving these fields blank is intentional and accurately represents the current
NO-GO state.
