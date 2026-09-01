---
title: Evaluation and Pilot Deployments
doc_type: guide
---

# Evaluation and Pilot Deployments

Evaluation and pilot deployments use the same self-managed installation path as production. Generate a local signing keypair, issue a short-lived license with the deployment wrapper, and run the same readiness, authorization, model, and streaming smoke tests.

Use a distinct local keypair or a distinct license ID for isolated evaluations. The router does not depend on a hosted evaluation service or externally issued license.
