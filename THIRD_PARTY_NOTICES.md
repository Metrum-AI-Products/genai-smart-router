# Third-Party Notices

GenAI Smart Router includes, builds with, or refers to third-party software.
The Apache-2.0 license in `LICENSE` applies to first-party content only; it does
not replace third-party terms.

## Locally evidenced inventory

The following inventory is based only on files checked into this repository.
It is not a complete license determination.

| Surface | Local evidence | What is established locally |
| --- | --- | --- |
| Go modules | `go.mod` and `go.sum` | Names and pinned module versions for direct and indirect dependencies. These files do not state every dependency's license or required notices. |
| Documentation web application | `docs-site/package.json` and `docs-site/package-lock.json` | Direct dependencies, resolved transitive packages, versions, integrity data, and package-manager-reported license identifiers where present. |
| Embedded admin web application | `internal/router/admindist/web/package.json` and `internal/router/admindist/web/package-lock.json` | Source dependencies and package-manager metadata. Compiled assets under `internal/router/admindist/static/` contain bundled third-party code. |
| Work dashboard | `work-dashboard/package.json` and `work-dashboard/package-lock.json` | Source dependencies and package-manager metadata. |
| Python evaluation tooling | `tests/api_compat/pyproject.toml` and `evaluators/livecodebench/requirements.txt` | Direct pinned requirements (`pytest==8.3.5` and `datasets==3.5.0`); complete transitive license metadata is not recorded there. The Harbor helper in `examples/harbor-algotune-pca/pyproject.toml` declares no Python dependencies. |
| Vendored ElevenLabs widget | `docs-site/static/vendor/elevenlabs/README.md` and `convai-widget-embed-0.16.3.js` | The repository records package `@elevenlabs/convai-widget-embed`, version `0.16.3`, its source URL, and vendoring date. No license text or copyright notice for this artifact is present locally. |
| Vendored libsamplerate worklet | `docs-site/static/vendor/elevenlabs/README.md` and `libsamplerate.worklet-2.1.2.js` | The repository records package `@alexanderolsen/libsamplerate-js`, version `2.1.2`, and its source URL. No license text or copyright notice for this artifact is present locally. |
| Fonts and images | `docs-site/static/fonts/`, `docs-site/static/img/`, and `internal/router/admindist/static/` | Binary font and image assets are checked in, but local files do not consistently identify their provenance or governing terms. |

## Required-attribution status

No third-party attribution is copied into the root `NOTICE` because the local
evidence reviewed for this inventory does not establish a specific notice that
must be reproduced there. This is not a conclusion that no attribution is
required.

## Unresolved risk

This inventory is intentionally conservative and is not legal advice. Package
names, lockfile license fields, source URLs, and dependency checksums do not by
themselves prove the governing terms for the exact source or binary being
distributed, nor do they identify all copyright notices, attribution clauses,
source-offer obligations, exceptions, generated-code provenance, or license
compatibility constraints.

Before any public or customer distribution, a qualified reviewer should obtain
and preserve the authoritative license and notice material for every shipped
dependency and asset, including transitive code included in compiled Go
binaries, JavaScript bundles, vendored scripts, fonts, and images. The two
vendored JavaScript artifacts and the unattributed font/image assets require
particular attention because their governing terms are not established by the
checked-in evidence. Generated distribution packages should carry all notices
required by that completed review.
