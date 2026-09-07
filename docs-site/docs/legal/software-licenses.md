---
title: Software License And Third-Party Notices
doc_type: reference
---

# Software License And Third-Party Notices

Use the legal files delivered at the root of each GenAI Smart Router release
artifact as a set. They have different scopes and should remain together when
an artifact is copied or redistributed.

| File | What it covers |
| --- | --- |
| `LICENSE` | Apache License 2.0 terms for GenAI Smart Router first-party content. |
| `NOTICE` | Notices distributed with the first-party work and any attribution that confirmed source terms require there. |
| `THIRD_PARTY_NOTICES.md` | The dependency and asset inventory, including recorded provenance, license information, and distribution surface. Third-party materials remain subject to their own applicable terms. |
| `MODEL_LICENSES.md` | The boundaries and recorded terms for model or dataset references and download surfaces. The project license does not grant rights to separately obtained model weights or datasets. |

The Apache-2.0 license applies to first-party content; it does not replace a
third party's license, notice requirements, model terms, dataset terms, or
trademark rules. An inventory entry is not a blanket grant of rights and does
not resolve an item whose disposition is recorded as unknown or pending.
Review the files from the exact release artifact before redistribution because
the included components and recorded dispositions can change between releases.

For source-browser readers, the current repository copy is
[`LICENSE`](https://github.com/sysadmin-metrum-ai/genai-smart-router/blob/main/LICENSE).
That link is not a substitute for the four legal files shipped with the exact
release artifact.

## Runtime `license.json` Is Operator Policy

A signed `license.json` is a deployment-specific runtime-policy input. The
router uses it to enforce configured feature and operational limits. It is not
the software copyright license, does not replace the artifact-root legal
files, and does not determine the terms of third-party components or models.

Keep `license.json` in the deployment's protected configuration path and out of
source control and redistribution packages. For configuration, renewal, and
rollback behavior, see [License](../configuration/license) and
[License-Protected Deployments](../operations/license-protected-deployments).

## Operator Review

Before deploying or redistributing a release:

1. Confirm the artifact includes all four root legal files and that they belong
   to that artifact version.
2. Review `THIRD_PARTY_NOTICES.md` for the shipped surface and any unresolved or
   specially classified component or asset.
3. Review `MODEL_LICENSES.md` before downloading, deploying, fine-tuning, or
   redistributing any separately obtained model or dataset.
4. Preserve the required license and notice material with redistributed
   copies, based on the applicable terms recorded for the exact contents.
5. Manage `license.json` separately as protected runtime configuration.

For questions about a delivered artifact or its notice set, use the
repository's public issue forms for non-sensitive licensing questions, or
email [contact@metrum.ai](mailto:contact@metrum.ai) for commercial inquiries.
