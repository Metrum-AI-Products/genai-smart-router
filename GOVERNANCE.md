# Project Governance

Metrum Router uses a maintainer-led governance model.

## Roles

- Contributors propose documentation, code, tests, designs, and issue reports.
- Reviewers provide technical feedback but do not gain merge or release
  authority solely by reviewing a change.
- Maintainers have repository write access and are accountable for review,
  merge, release, security, and community decisions.

The current approved owner for launch leadership, release management, security,
operations, and community management is Chetan Gadgil. This appointment records
decision ownership; it does not by itself record a release approval, operational
coverage, or a go/no-go decision. Repository access remains the source of truth
for who can review and merge changes.

## Decisions

Routine changes are decided through public issues and pull requests. A
maintainer merges a change after relevant checks pass and material review
feedback is addressed. Significant compatibility, security, data-handling,
deployment, or governance changes should start with an issue that records the
problem, alternatives, operational impact, and rollback approach.

When contributors disagree, they should first seek a technically supported
consensus in the issue or pull request. Maintainers make the final repository
decision when consensus is not reached and should record the reason publicly
unless security or privacy requires a private record.

## Contributions And Releases

Contributions follow [CONTRIBUTING.md](CONTRIBUTING.md), including the Developer
Certificate of Origin sign-off requirement. Maintainers may request focused
changes, tests, documentation, compatibility evidence, or security review.

Releases are cut by maintainers from the protected default branch after the
repository's release checks pass. Release notes describe caller and operator
impact, validation, and rollback. No contributor should infer release authority
from issue assignment, review participation, or package access.

This project is maintained as public open-source software. Release decisions
use public issues, pull requests, and release notes with current validation
evidence; an owner appointment is not a substitute for that evidence.

## Escalation

- Bugs, proposals, and governance questions use the public issue templates.
- Usage questions follow [SUPPORT.md](SUPPORT.md).
- Vulnerabilities follow [SECURITY.md](SECURITY.md) and must not be filed as
  public issues.
- Conduct concerns follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Repository ownership, maintainer appointments, and changes to this governance
model are maintainer decisions and should be recorded in a public issue or pull
request when privacy and security allow.
