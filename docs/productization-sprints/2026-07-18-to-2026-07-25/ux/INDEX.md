# UX catalog INDEX

**Commercial model:** base monthly Stripe subscription + model-mix included allowance + x402 overage ([DECISIONS.md](../../../../DECISIONS.md) D3).

**Entry points**

- [personas.md](personas.md)
- [e2e-subscription.md](e2e-subscription.md) — master cloud journey
- [previews/](previews/) — rewritten bento mocks
- [issues/](issues/) — per-issue narrative + HTML
- [shipped/](shipped/) — shipped router surfaces
- [video/](video/) — narrated video **approval packages** (no TTS until narration approved)

**Visual system:** [_ds/_ds_prompt.md](_ds/_ds_prompt.md) · [_ds/metrum-bento.css](_ds/metrum-bento.css)

---

## Persona × function matrix

| Persona | Functions | Narratives / mocks |
| --- | --- | --- |
| P1 Prospect | Pricing, signup | [e2e](e2e-subscription.md), [#533](issues/533-docs-legal-support.html), [#540](issues/540-plan-catalog.html) |
| P2 Billing admin | Subscribe, Portal, allowance | [checkout preview](previews/checkout-provisioning.html), [#524](issues/524-stripe-subscription.html), [#525](issues/525-prosumer-console.html) |
| P3 Developer | Keys, APIs, x402, CLIs | [#525](issues/525-prosumer-console.html), [#506](issues/506-x402-overage.html), [#522](issues/522-admission.html), [api-and-cli](shipped/api-and-cli.html) |
| P4 Tenant admin | BYOK, connections, studio | [#535](issues/535-byok-vault.html)–[#538](issues/538-private-upstream.html) |
| P5 Enterprise ops | Package, license, smoke | [enterprise-self-host](shipped/enterprise-self-host.html) |
| P6 Support | Suspend, safe IDs | [#521](issues/521-control-plane.html), [#529](issues/529-abuse-fraud.html) |
| P7 Finance | Catalog, ledger, Stripe | [#540](issues/540-plan-catalog.html), [#523](issues/523-ledger.html), [#524](issues/524-stripe-subscription.html) |
| P8 SRE | Provision, SLOs, release | [#526](issues/526-provisioning.html), [#531](issues/531-observability-oncall.html), [#527](issues/527-release-engineering.html) |
| P9 Security | Isolation, abuse, x402 | [#530](issues/530-security-compliance.html), [#529](issues/529-abuse-fraud.html) |
| P10 Model ops | Canary, auto-tune | [#528](issues/528-canary.html), [#541](issues/541-auto-tune.html) |
| P11 Deploy admin | Admin reports | [admin-reports-shell](shipped/admin-reports-shell.html) |

---

## Launch-blocking issues

| Issue | Narrative | HTML |
| ---: | --- | --- |
| 520 | [md](issues/520-epic-journey.md) | [html](issues/520-epic-journey.html) |
| 521 | [md](issues/521-control-plane.md) | [html](issues/521-control-plane.html) |
| 522 | [md](issues/522-admission.md) | [html](issues/522-admission.html) |
| 523 | [md](issues/523-ledger.md) | [html](issues/523-ledger.html) |
| 524 | [md](issues/524-stripe-subscription.md) | [html](issues/524-stripe-subscription.html) |
| 525 | [md](issues/525-prosumer-console.md) | [html](issues/525-prosumer-console.html) |
| 526 | [md](issues/526-provisioning.md) | [html](issues/526-provisioning.html) |
| 527 | [md](issues/527-release-engineering.md) | [html](issues/527-release-engineering.html) |
| 529 | [md](issues/529-abuse-fraud.md) | [html](issues/529-abuse-fraud.html) |
| 530 | [md](issues/530-security-compliance.md) | [html](issues/530-security-compliance.html) |
| 531 | [md](issues/531-observability-oncall.md) | [html](issues/531-observability-oncall.html) |
| 533 | [md](issues/533-docs-legal-support.md) | [html](issues/533-docs-legal-support.html) |
| 534 | [md](issues/534-integrated-beta.md) | [html](issues/534-integrated-beta.html) |
| 540 | [md](issues/540-plan-catalog.md) | [html](issues/540-plan-catalog.html) |
| 506 | [md](issues/506-x402-overage.md) | [html](issues/506-x402-overage.html) |

## Fast-follow issues

| Issue | Narrative | HTML |
| ---: | --- | --- |
| 528 | [md](issues/528-canary.md) | [html](issues/528-canary.html) |
| 532 | [md](issues/532-marketplace.md) | [html](issues/532-marketplace.html) |
| 535 | [md](issues/535-byok-vault.md) | [html](issues/535-byok-vault.html) |
| 536 | [md](issues/536-upstream-onboarding.md) | [html](issues/536-upstream-onboarding.html) |
| 537 | [md](issues/537-routing-studio.md) | [html](issues/537-routing-studio.html) |
| 538 | [md](issues/538-private-upstream.md) | [html](issues/538-private-upstream.html) |
| 539 | [md](issues/539-byok-attribution.md) | [html](issues/539-byok-attribution.html) |
| 541 | [md](issues/541-auto-tune.md) | [html](issues/541-auto-tune.html) |

## Shipped surfaces

| Surface | Narrative | HTML |
| --- | --- | --- |
| Admin reports (~50 tabs) | [md](shipped/admin-reports.md) | [html](shipped/admin-reports-shell.html) |
| Enterprise self-host | [md](shipped/enterprise-self-host.md) | [html](shipped/enterprise-self-host.html) |
| API & CLI | [md](shipped/api-and-cli.md) | [html](shipped/api-and-cli.html) |
| Routing & policy | [md](shipped/routing-and-policy.md) | [html](shipped/routing-and-policy.html) |
| Licensing ops | [md](shipped/licensing-ops.md) | [html](shipped/licensing-ops.html) |

## Videos (approval-gated)

See [video/README.md](video/README.md). **Do not synthesize audio or render until each package’s narration is explicitly approved.**
