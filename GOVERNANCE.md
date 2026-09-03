# Project Governance

GenAI Smart Router uses a maintainer-led governance model. This document
describes roles and decisions without assigning identities that the project has
not publicly confirmed.

## Roles

- Contributors propose documentation, code, tests, designs, and issue reports.
- Reviewers provide technical feedback but do not gain merge or release
  authority solely by reviewing a change.
- Maintainers have repository write access and are accountable for review,
  merge, release, security, and community decisions.

The current maintainer roster is the set of people with maintainer access in
the public GitHub organization. A named roster will be added only after those
people consent to publication.

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

## Escalation

- Bugs, proposals, and governance questions use the public issue templates.
- Usage questions follow [SUPPORT.md](SUPPORT.md).
- Vulnerabilities follow [SECURITY.md](SECURITY.md) and must not be filed as
  public issues.
- Conduct concerns follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Repository ownership, maintainer appointments, and changes to this governance
model are maintainer decisions and should be recorded in a public issue or pull
request when privacy and security allow.

