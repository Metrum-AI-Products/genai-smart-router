# Legal Document Publication

## Canonical documents

- `docs-site/docs/legal/terms-of-use.md` is the public Terms of Use served at
  `/docs/terms`.
- `docs/LICENSE.md` is the package copy of the Apache License 2.0 notice for all
  Metrum Smart Router first-party content. It is included in
  binary and Docker packages as `docs/LICENSE.md`.

The product has no EULA. Terms of Use govern only the public website and do not
limit rights granted for Apache-covered materials. A runtime `license.json` is
operator policy, not a copyright or commercial-use condition.

## Release control

Before publishing a release that changes either document:

1. Obtain legal approval for Terms changes, including the complete text,
   effective date, legal entity, governing law, and liability limits.
2. Build Docusaurus and confirm `/docs/terms` renders with no EULA route.
3. Build binary and Docker packages and confirm `docs/LICENSE.md` is present in
   every package archive.
4. Record the approved document revision with the release evidence.

A separately executed agreement controls where it conflicts with the Terms of
Use for its subject matter, but does not alter Apache-2.0 rights except through
an agreement made by the relevant rights holder. Do not represent informational
product, licensing, DPA, subprocessor, or transfer documents as an executed
agreement.
