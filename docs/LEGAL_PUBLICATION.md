# Legal Document Publication

## Canonical documents

- `docs-site/docs/legal/terms-of-use.md` is the public Terms of Use served at
  `/docs/terms`.
- `docs/END_USER_LICENSE_AGREEMENT.md` is the canonical End User License
  Agreement (EULA). It is served at `/docs/eula` through the Docusaurus MDX
  import and is included in binary and Docker packages as
  `docs/END_USER_LICENSE_AGREEMENT.md`.

Do not create parallel EULA copies. Update the canonical package document so
that the website and distributable packages present identical EULA text.

## Release control

Before publishing a release that changes either document:

1. Obtain legal approval for the complete text, effective date, legal entity,
   governing law, liability limits, and acceptance mechanism.
2. Build Docusaurus and confirm `/docs/terms` and `/docs/eula` render.
3. Build binary and Docker packages and confirm the EULA is present in every
   package archive.
4. Record the approved document revision with the release evidence.

A separately executed agreement controls where it conflicts with the Terms of
Use or EULA. Do not represent informational product, licensing, DPA,
subprocessor, or transfer documents as an executed agreement.
