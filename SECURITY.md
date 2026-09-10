# Security Policy

## Reporting A Vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private
vulnerability-reporting form for this repository:

https://github.com/metrum-ai/router/security/advisories/new

Include affected versions, impact, a minimal reproduction, and suggested
mitigations when available. Do not include live credentials, customer data,
private infrastructure details, or destructive proof-of-concept activity.
Arrange a protected transfer with the security team if sensitive evidence is
necessary.

Maintainers aim to acknowledge a report within three business days, provide an
initial assessment within seven business days, and send status updates at least
every fourteen days until resolution. Timing may vary with severity and
complexity.

## Coordinated Disclosure

Maintainers coordinate validation, disclosure, release timing, credit
preferences, and CVE handling through the private advisory. Reporters are asked
to keep a vulnerability confidential while a fix and user guidance are
prepared. The project will not request indefinite nondisclosure. Active
exploitation or material user harm may require an accelerated advisory and
mitigation guidance.

## Supported Versions

Security fixes are made on the current development line and released through
the normal release process. Older releases are not guaranteed to receive
backports. Operators should review release notes, validate upgrades in their
environment, and retain a tested rollback artifact.

## Deployment Boundary

The router cannot secure provider accounts, networks, hosts, clusters, secret
stores, databases, ingress, or clients that an operator configures around it.
See the public security and deployment-readiness documentation for those
self-managed responsibilities.
