# GenAI Smart Router Commercialization Plan

This is an internal product and commercialization plan for GenAI Smart Router. It is not customer-facing pricing copy. Revalidate exact competitor pricing, procurement channels, marketplace availability, packaging, and commercial terms before publishing any public comparison or exact price claim.

Research snapshot: 2026-06-24. Strategy revised 2026-06-27 to enterprise-first license model (issues #159, #36, #39, #40, #42).

Current customer-facing docs live in `docs-site/docs/`, especially:

- `docs-site/docs/operations/license-protected-deployments.md` for shipped signed-license deployment behavior;
- `docs-site/docs/evaluation/commercial-evaluation.md` for the current commercial evaluation path;
- `docs-site/docs/operations/usage-reporting.md` for shipped usage reporting, rollup, and retention foundations.

Primary reference points reviewed:

- LiteLLM Enterprise: self-hosted/cloud enterprise license positioning, license-key unlock, deployment-size pricing, AWS/Azure Marketplace procurement.
  Source: https://docs.litellm.ai/docs/enterprise
- Helicone pricing: free/pro/team self-serve style, monthly tiers plus usage-based pricing.
  Source: https://www.helicone.ai/pricing
- Portkey positioning: AI gateway, observability, guardrails, governance, prompt and model-catalog platform.
  Source: https://portkey.ai/
- OpenRouter pricing: prepaid credits, manual/automatic top-up, enterprise by volume/prepayment/annual commit, and routing/fallback billing around successful model runs.
  Source: https://openrouter.ai/pricing

## Shipped Commercial Foundations

The router now includes several commercial foundations:

- Signed JSON license verification is shipped. Licensed deployments can enable `server.license` so the router verifies a Metrum-issued signed JSON license with embedded Ed25519 public verification keys, expiry, product checks, feature gates, deployment limits, grace behavior, clock-rollback state, safe status reporting, `/readyz` integration, and `license-*` caller errors. Issue #35 is closed.
- License-safe operations docs are shipped. Public docs cover license-protected deployments, configuration, renewal/replacement, admin status, metrics-admin visibility, troubleshooting, and secret-handling boundaries without publishing private signing keys, license payloads, private deployment paths, router tokens, or provider keys.
- Commercial evaluation docs are shipped. Public evaluation docs describe hosted, private-cloud, and enterprise/on-prem evaluation paths, deployment readiness, security assessment, workload validation, usage reporting evidence, and contact flow.
- Usage reporting and commercial retention foundations are shipped where documented. Request-time costs, upstream-reported billed costs, latency/throughput dimensions, safe license metadata, daily rollups, baseline savings fields, dry-run retention status, legal-hold rows, and guarded first-slice purge execution are available as documented. Archive/export automation, schedulers, broader data-class purge execution, and full browser/admin write workflows remain future slices.

Open commercial implementation issues (revised 2026-06-27):

- #159: Enterprise license shape: capabilities + time + volume + operational limits (P0, new).
- #36: Define enterprise SKUs and map them to capability/time/volume license entitlements (P0).
- #39: Enterprise license issuance, renewal, volume-top-up, and support workflow (P0).
- #40: Private managed deployments and marketplace procurement (P1).
- #42: Public pricing and package documentation (P2).

Closed as wontfix (managed cloud / billing ledger / customer portal deferred):

- #37: Managed cloud self-serve checkout and provisioning.
- #38: Billing-grade usage metering, credits, and invoice ledger.
- #41: Customer billing and admin portal for managed cloud.

## Strategic Direction (revised 2026-06-27)

Enterprise and private-managed buyers are sold via **signed `license.json` that encodes capabilities, operational limits, and usage budget** — not by forcing an annual prepaid contract and not by building an in-product billing ledger. Entitlements are enforced in-router (#35 shipped). Metrum bills via normal finance (invoice/PO/wire) per the **commercial shape the license encodes**, not via router metering.

Managed-cloud self-serve (#37), billing ledger (#38), and customer portal (#41) are **out of scope** for the current GTM motion and are closed `wontfix`.

## Goals

- Sell signed licenses that encode capability + time + volume independently or together.
- Preserve high-ACV enterprise paths for self-hosted, private-cloud, and Metrum-managed deployments.
- Monetize the router value separately from upstream provider cost (BYOK default).
- Keep packaging compatible with enterprise procurement: annual contracts, private offers, AWS/Azure Marketplace, signed self-hosted licenses, and short-eval or volume-only license templates.
- Make commercial enforcement practical without blocking legitimate on-prem and air-gapped customers.
- Keep public docs generic and deployment-defined; avoid exposing private hostnames, provider keys, real router tokens, private pricing commitments, or production-only model group details.

## Recommended Commercial Motions

### 1. Enterprise Self-Hosted License

Primary commercial motion.

Customer runs the Docker package or binary in their infrastructure with a signed license file. Metrum signs a `license.json` that encodes the SKU, capability gates, volume/time limits, and operational scope.

Best for:

- regulated environments;
- VPC/on-prem buyers;
- customers with strict data-residency requirements;
- customers with existing provider contracts and BYOK;
- annual contracts, volume prepurchase, and procurement.

Commercial shape:

- signed JSON license with SKU, expiry, feature entitlements, volume and window limits, and operational limits;
- support/SLA tier;
- limits by production instances, total/volume requests or tokens, admin seats, model groups, retention, or enabled features;
- procurement through direct order form, AWS/Azure Marketplace private offers, or volume prepurchase credit packs.

Implementation status: signed JSON license enforcement has shipped (#35). Runtime fields for capability + time + volume + operational limits are tracked in #159. Operational packaging around license issuance, renewal, volume top-up, support workflows, and entitlement policy is tracked in #39.

### 2. Private Managed Deployment

Metrum operates a dedicated router deployment for one customer in Metrum cloud, customer cloud, or a managed VPC pattern.

Best for:

- customers who need isolation but do not want to operate the router;
- pilots that may later become self-hosted;
- enterprise buyers who need custom upstream / network configuration.

Commercial shape:

- setup fee;
- monthly managed service fee;
- annual or volume license (`enterprise-annual` or `credit-pack-*` template);
- higher support/SLA tier;
- optional private networking, SSO, custom policy, and acceptance validation package.

### 3. Marketplace Procurement

Package Enterprise Self-Hosted (`enterprise-annual`) and Private Managed Deployment (`marketplace-seat`) for cloud marketplace procurement after the first direct enterprise sales.

Best for:

- enterprise procurement simplification;
- private offers;
- annual committed spend;
- reducing contract friction.

Do not make marketplace the first dependency for self-serve. Enterprise license direct sales should ship first.

### 4. Metrum-Managed Evaluation Endpoint

Time-bounded evaluation endpoint (`eval-72h` and `pilot-30d` license templates) for partners and prospective customers to validate workloads before signing an enterprise contract. Not a self-serve credit-card motion; contact required.

## License Templates (what Metrum actually sells)

Each commercial SKU maps to a license template. A SKU is the commercial name; the license template is the runtime envelope Metrum signs.

| Template | Time | Volume | Use case |
|---|---|---|---|
| `eval-72h` | `expires_at = +72h` | `max_total_tokens: 5_000_000` | Free / partner evals |
| `pilot-30d` | +30 days | `window: 1M tokens / 1h` | Time-boxed pilot |
| `enterprise-annual` | +12 months | unlimited (or contract ceiling) | Default annual contract |
| `credit-pack-5m` | +12 months | `max_total_tokens: 5_000_000` | Volume top-up |
| `credit-pack-25m` | +12 months | `max_total_tokens: 25_000_000` | Larger volume prepay |
| `marketplace-seat` | term of contract | per-seat volume | AWS/Azure private offer |

Time-only, volume-only, and time+volume are all legal. Whichever is reached first blocks further traffic. SKU-to-template mapping is defined in #36; runtime fields and enforcement are defined in #159.

## Billing Models

### Annual Enterprise License (primary)

Prepaid annual license for self-hosted or private-managed deployments.

Pros:

- procurement friendly;
- predictable revenue;
- works for on-prem and air-gapped environments;
- enforced by signed license file.

Cons:

- slower sales cycle;
- needs renewal process and license operations.

### Volume-Prepurchase License (primary for top-up)

Prepaid volume license (`credit-pack-*`) with optional time bound. Used both as initial purchase and as top-up when an `enterprise-annual` license's volume counter is exhausted.

Pros:

- familiar to API customers;
- matches OpenRouter / Helicone credit-pack expectations;
- enables short-term or trial volume-only purchases.

Cons:

- requires clean counter reset on replacement;
- customers may ask how credits map to provider cost.

### BYOK Platform Fee (default for enterprise)

Customer supplies provider keys; router enforces only its own entitlement envelope. Metrum bills only for the license.

Pros:

- enterprise friendly;
- lowers Metrum provider credit exposure;
- easier for private upstreams.

Cons:

- lower gross margin;
- value must be tied to routing, telemetry, governance, support, and validation.

### Deferred / Not Currently Offered

The following billing models are scoped under managed cloud and are **deferred indefinitely** (closed as wontfix via #37, #38, #41):

- Platform Subscription Plus Usage (managed cloud).
- Credit Wallet with auto-top-up (managed cloud).
- Provider Pass-Through Plus Markup (managed cloud).

If Metrum commits to a credit-card managed-cloud product in the future, these models may be reintroduced.

## Entitlements And Feature Gates

Commercial entitlements map to product capabilities rather than hardcoded model group names. The runtime fields and constants are defined in #159; the SKU-to-template commercial mapping is defined in #36.

Entitlement dimensions:

- deployment type: self-hosted, private-managed, managed-evaluation (managed cloud is deferred);
- enabled features: routing, dynamic_score, typescript_routing, external_policy, external_policy_http, model_group_contracts, usage_reporting, admin_reports, admin_security_reports, retention_rollups, content_capture, private_upstreams, audit_log_export, pii_filtering, usage_csv_export, usage_baseline_export;
- usage limits: total requests, total tokens, window requests, window tokens, concurrency;
- operational limits: projects, callers, admins, model groups, retention days, instances, allowed skins;
- support tier and SLA.

For self-hosted and private-managed deployments:

- entitlements live in the signed JSON license payload;
- the router validates expiry, product, issuer, features, limits, deployment fields, and optional instance fingerprint locally;
- the license can be replaced without binary rebuild.

For managed-evaluation endpoints:

- evaluation licenses are issued via the same license signing flow;
- payment is handled by Metrum commercial contact, not by the router.

Managed cloud (deferred) would have used a billing/control-plane DB; that path is not pursued at this time.

## Metering Requirements (customer-side only)

Metering inside the router exists to support **customer chargeback**, **optional contract true-up reviews**, and **internal usage reporting**. It is not a Metrum product billing ledger.

Meter at minimum:

- request count;
- successful request count;
- failed / fallback attempt count;
- input tokens;
- output tokens;
- total tokens;
- image count and image tokens where applicable;
- calculated input/image/output/total cost in USD;
- upstream-reported billed cost where available;
- selected provider/model/model group;
- caller org/project/environment/token ID;
- cache hit/miss/bypass;
- latency and throughput for service-tier reporting.

Rules:

- Use stored request-time cost values, not current provider pricing, for historical chargeback.
- Chargeback policy is set by the customer, not by the router.
- License-wide volume counters (`max_total_tokens`, `max_total_requests`, `window_*`) are tracked in `licensePersistentState` for license enforcement and reset on license replacement.
- Per-key counters (`Key.LifetimeTokens`) are tracked in `callerState` for caller policy and reset only on caller-key rotation.
- Keep billing data relational and queryable; no JSONB/array/serialized structured columns.

## Initial Launch Recommendation

Revised order after enterprise-first strategy:

1. #159: define license shape (capability + time + volume + operational limits) and runtime fields.
2. #36: SKU-to-template mapping and feature-gate matrix.
3. #39: license issuance, renewal, volume-top-up, and support workflow.
4. #40: private managed deployment runbook and marketplace procurement.
5. #42: public packaging documentation after legal review.

Rationale:

- Enterprise direct sales create predictable revenue and validate the license envelope.
- One signed license JSON encodes all entitlement axes; finance handles renewal, the router enforces.
- Signed licensing already protects self-hosted deployments; #39 makes issuance and renewal operations repeatable.
- Marketplace should follow proof of enterprise pull.

## Prioritized GitHub Workstreams

### P0 - Enterprise license shape (#159)

Define the runtime license envelope: capability gates, volume/window/concurrency limits, operational scope, and license-wide counter storage. Backward-compatible with `schema_version = 1`.

### P0 - Commercial packaging, SKU-to-template mapping (#36)

Define SKUs (`eval-72h`, `pilot-30d`, `enterprise-annual`, `credit-pack-*`, `marketplace-seat`), feature gates per SKU, and operational limits per SKU.

### P0 - Enterprise license operations (#39)

Build on shipped signed license enforcement (#35) and license shape (#159). Provide license issuance, renewal, volume top-up, support playbook, and audit log of issuances (outside repo).

### P1 - Private managed deployments and marketplace procurement (#40)

Define repeatable packaging, runbooks, support boundaries, private offers, and deployment acceptance.

### P2 - Public packaging documentation (#42)

Publish customer-facing packaging pages after legal review. No public pricing yet.

## Open Decisions

- Whether `max_total_tokens` is token-cost-weighted or raw token count (current proposal: raw tokens).
- Per-key counter reset on license renewal: keep contract/config, not license (current proposal: no reset).
- Volume top-up flow: `carry_over` flag for unused balance (current proposal: deferred; replacement resets counter).
- `server.license.fail_open_for_dev` semantics for local development images (#158).
- Marketplace SKU including Metrum-billed provider pass-through (current proposal: no).
- Support / SLA tier mapping to product tiers.

## Risks

- License disputes if volume counters are not reset cleanly on replacement.
- Customer confusion between per-key and license-wide counters.
- Enterprise friction if self-hosted licensing is too restrictive for air-gapped customers.
- Product confusion if hosted deployment group names are treated as product constants.

## Success Metrics

- Time from contact to first signed license issued.
- Eval-to-contract conversion rate (eval-72h / pilot-30d to enterprise-annual).
- License renewal rate.
- Volume top-up frequency per customer.
- Time-to-license-issued (sales to deployment).
- Support tickets per active customer.
- Router usage growth by project and model group.