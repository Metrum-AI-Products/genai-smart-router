# 08 — Abuse, Fraud, and Provider-Cost Protection (#529)

Plan version: **v1.0.0**  
Issue: [#529](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/529)  
Classification: **launch blocking / must precede public trial or pooled keys**

## Design approach

Build deterministic policy layers before considering opaque risk ML. Reuse verified
identity, Stripe payment signals, caller RPM/TPM/concurrency/quota/traffic shaping,
provider shaping/backoff, and signed balance grants. Do not inspect or retain
prompts, tool payloads, images, provider keys, token hashes, or card data as fraud
signals.

Risk decisions are versioned scalar events: policy version, bounded signal name,
safe bucket/value, decision (`allow`, `challenge`, `hold`, `deny`, `suspend`),
reason code, expiry, actor, tenant/user/device/payment safe identifiers, and audit
correlation. Repeated signals use child rows; no opaque JSON risk blob. Customer-
visible reasons are broad and actionable; detailed thresholds remain admin-only.

Controls operate at four layers:

1. **Signup:** verified email, terms, IP/ASN/region policy where legally approved,
   velocity by safe identity/device/payment fingerprint, disposable-domain policy,
   CAPTCHA/challenge, and one trial per approved identity/payment relationship.
2. **Funding:** Stripe paid state, top-up count/value velocity, dispute/refund state,
   card-country/region mismatch only through allowed Stripe signals, and cooling
   periods for high-risk funding.
3. **Spend:** prepaid authority, conservative quote/hold, per-tier hard caps,
   RPM/TPM/concurrency, output-cap limits, provider account cap, and no negative
   available balance.
4. **Operations:** anomaly alerts, tenant/token kill switches, grant revocation,
   review queue, support appeal, false-positive resume, and global pooled-provider
   emergency stop independent of the control plane.

## Human interaction and policy configuration

Finance/Legal/Security approve D10 trial value/eligibility, regions, data used for
risk, retention, challenge/appeal, refund/dispute consequence, top-up min/max,
velocity windows, hard spend caps, and who can suspend/resume. Product approves
customer messages. SRE defines provider-account exposure caps and global/tenant
kill-switch procedure. Every threshold has owner, rationale, version, dry-run
mode, effective date, and rollback value.

Support UI shows safe account/tenant/public token IDs, risk decision timeline,
policy version, bounded reasons, current caps/holds, linked Stripe event IDs (not
payment details), and reauth/two-person suspend/resume. It must not expose the
full rule engine to customers or permit a raw balance edit.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Required
integrations are OIDC verified claims, Stripe webhook-derived safe signals,
control-plane/ledger identity, router grant/quota config, CAPTCHA/risk provider if
approved, notification/support and observability. Secrets are any CAPTCHA/risk API
credential, internal service identity, DB DSN, and alert/support integration; the
risk service never receives provider keys, raw prompts/tokens, Stripe secret/card
fields, or signing private material. Non-secrets: policy/trial versions, allow/deny
regions, velocity windows, cap values, challenge thresholds, signal retention,
fail-safe mode, reason mapping, reviewer roles, and kill-switch identifiers.

## Customer and operator flows

* Low risk: verify -> one bounded trial -> provisioning -> hard capped use.
* Needs payment: no trial -> hosted Checkout -> paid webhook -> provision.
* Challenged: safe message -> CAPTCHA/add payment/support -> reevaluate idempotently.
* Spend anomaly: stop new grants or lower cap, allow safe in-flight reconciliation,
  notify customer/support, review, then resume with a new policy/grant version.
* Compromised API key: revoke public token ID immediately, release safe holds,
  rotate, preserve audit, and avoid suspending unrelated organization keys unless
  policy requires it.

## Test plan

| Scenario | Setup/action | Expected observable |
| --- | --- | --- |
| Trial farm | many accounts sharing approved safe signals | first bounded grant only; others challenge/deny; no provider call |
| Velocity | burst signup/top-up/key creation | deterministic cap and retry window; safe audit/metric |
| Concurrency | N maximum-output calls near balance | admitted reservation total stays within grant/cap |
| Compromise | one token spikes across models/clients | token/tenant kill switch stops new upstream work; others unaffected |
| Dispute/refund | ordered and unordered Stripe events | future grants held/suspended per versioned policy; ledger remains balanced |
| Dependency loss | risk/CAPTCHA/control plane unavailable | documented fail-safe; no free unbounded access |
| False positive | reviewer resumes with reason | new bounded grant/key works; immutable prior decision remains |
| Privacy | inspect risk DB/log/trace/evidence | no forbidden content/secrets; retention purge works |

Load-test a malicious tenant at cap alongside healthy tenants and provider account
limits; healthy p95/error SLO must remain within target. Tabletop day-one abuse,
credential theft, disputed top-up, and global provider-key leak.

## Monitoring, rollback, and definition of done

Alert on signup/challenge/deny rates, trial issuance/value, top-up velocity,
grant/cap denials, negative-balance prevention, kill-switch activity, provider
spend acceleration, false-positive resume and risk dependency health. A bad policy
rolls back to the last signed version without removing baseline prepaid/hard caps.
Done when Security/Finance/Legal approve policies and fault/load/tabletop evidence
shows provider exposure remains bounded on day one.
