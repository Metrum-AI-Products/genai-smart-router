# Contributing to GenAI Smart Router

Thank you for contributing.

Please read the [Code of Conduct](CODE_OF_CONDUCT.md), project
[governance](GOVERNANCE.md), and [support boundaries](SUPPORT.md) before
participating. Vulnerabilities must use the private process in
[SECURITY.md](SECURITY.md), not a public issue.

## License

This project is licensed under the Apache License, Version 2.0. Unless a
contribution is explicitly identified and accepted under different terms in
writing, every contribution intentionally submitted for inclusion in this
project is provided under the same Apache-2.0 terms (inbound = outbound). By
submitting a contribution, you agree that the project may distribute it under
those terms.

Do not submit material that you do not have the right to contribute. Identify
third-party material and any applicable license, notice, attribution, or other
redistribution requirement in the contribution.

## Developer Certificate of Origin

Every commit must include a `Signed-off-by` trailer certifying the Developer
Certificate of Origin, version 1.1 (https://developercertificate.org/). Add the
trailer with:

```text
git commit --signoff
```

The trailer must use your real name and an email address you are authorized to
use:

```text
Signed-off-by: Your Name <you@example.com>
```

Signing off states that you created the contribution, or otherwise have the
right to submit it under the project's license, and that you understand the
contribution and sign-off are public records.

Keep each commit's sign-off intact when rebasing, squashing, or amending work.

## Development

```bash
python3 scripts/local_dev_bootstrap.py --out-dir tmp/local-dev
go test ./cmd/... ./internal/...
python3 scripts/local_dev_bootstrap_test.py
rtk make docs-qa
```

Learned Routing Policy tests (Python, uv): `make lrp-test`. Do not commit
`tmp/local-dev`, `env.json`, `license.json`, or `license.key`.

