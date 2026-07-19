# V1 — Subscribe and provision (approval package)

**Status:** AWAITING NARRATION APPROVAL — do not synthesize audio yet  
**Audience:** prospects / billing admins  
**Objective:** Show buy → webhook → provision → key once  
**Aspect:** 16:9 · **Target duration:** ~70s including 0.75s pauses  
**Brand:** Metrum AI · bento mocks · official logo only  

## Complete narration (speakable)

**Scene 1.** Welcome to GenAI Smart Router hosted access. You pick a monthly plan that includes a model-mix token allowance, with x402 for overage.

**Scene 2.** After email verification, choose your region—it stays fixed—and select a plan from the versioned catalog.

**Scene 3.** Checkout runs on Stripe-hosted pages. Card details never enter the Metrum console. The success redirect alone does not activate your account.

**Scene 4.** A signed Stripe webhook credits the ledger, activates your subscription entitlement, and issues this period’s included allowance grant.

**Scene 5.** Provisioning configures your logical tenant on the regional fleet, installs verifier settings for x402, and runs readiness and API smokes.

**Scene 6.** Only after smokes pass do you see your API endpoint and a key shown once. Store it offline—then open the console and call /v1/models.

## Visual intent

| Scene | Visual | On-screen text |
| ---: | --- | --- |
| 1 | Pricing tiles (checkout mock, Plan tab) | Sub + allowance + x402 |
| 2 | Region + plan picker | Region immutable |
| 3 | Checkout + Confirming payment | Webhook required |
| 4 | Ledger/entitlement tiles (523/524 mocks) | Grant issued |
| 5 | Provisioning saga timeline | Shared fleet |
| 6 | Key reveal panel | Shown once |

## Source notes

Claims follow DECISIONS.md D3 and ux/e2e-subscription.md. Prices fictional.

## Open questions

- TTS provider/voice after approval.
- Prefer live browser capture of HTML vs static screenshots.
