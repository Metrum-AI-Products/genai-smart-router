# Security Policy

## Reporting A Vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private
vulnerability-reporting form for this repository:

https://github.com/sysadmin-metrum-ai/genai-smart-router/security/advisories/new

Include affected versions, impact, a minimal reproduction, and suggested
mitigations when available. Do not include live credentials, customer data,
private infrastructure details, or destructive proof-of-concept activity.

The project does not publish a response-time or remediation SLA. Maintainers
will coordinate validation, disclosure, and release timing through the private
advisory when the report is accepted.

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

