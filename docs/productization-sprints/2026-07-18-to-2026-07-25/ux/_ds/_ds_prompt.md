# UX design system prompt (binding)

Use for every HTML mock under `ux/`.

## Brand

- Metrum AI only. Logo: `_ds/metrum-logo-white.png` — never recreate as text.
- Palette: black `#000`, surface `#08080a`, text white, muted `#b8bcc7`, accent gradient `#FF3132 → #FE005F → #EE0089 → #CC28AF → #9948CB → #465CDA`.
- Type: `MetrumSans` / `MetrumMono` from `_ds/*.woff2`.
- Radius: 0. Thin rules. No rounded consumer pills, no cyan Inter theme, no sparse heroes.

## Layout

- Strict **bento grid** (12 columns). Every tile earns its space.
- No large empty regions, no marketing hero voids, no stacked sparse cards.
- First viewport should be dense: status tiles + primary action + timeline or table.

## Commercial model (do not drift)

Base monthly Stripe subscription + model-mix included allowance + x402 overage.
No prepaid-credit-primary or Stripe Billing Meter overage copy.

## Content rules

- Fictional data only (`org_demo_…`, `price_…`, `https://api.example.invalid`).
- Never show real tokens, keys, Stripe secrets, card numbers, prompts.
