---
title: Documentation Privacy
doc_type: reference
---

# Documentation Privacy

This open-source documentation is delivered as static content embedded in the
router release. The documentation pages are separate from router request and
usage records.

Do not submit provider keys, caller tokens, token hashes, credentials, prompts,
raw images, customer data, private hostnames, or full production configuration
through public repository issues. Suspected vulnerabilities use the private
reporting path in the repository `SECURITY.md`.

Router usage retention is deployment-configured under `server.retention`.
Governed content capture is disabled by default and, if enabled, requires its
documented encryption, authorization, and retention controls. The deployment
operator is responsible for publishing any privacy notice required for its own
users, infrastructure, providers, and data-handling choices.

