# Narrated mock-interaction videos — approval gate

Per [narrated-video-production](../../../../../.agents/skills/narrated-video-production/SKILL.md):

1. Each clip has an **approval package** with complete speakable narration.
2. **Do not** run TTS or assemble final MP4 until you explicitly approve that clip’s full narration.
3. After approval, choose TTS provider/voice; then render against measured audio.

## Clip set (~1 minute each)

| ID | Title | Target | Package | Primary mocks |
| --- | --- | --- | --- | --- |
| V1 | Subscribe and provision | ~60–75s | [V1-subscribe-provision.md](V1-subscribe-provision.md) | checkout-provisioning |
| V2 | Console: allowance and keys | ~60s | [V2-console-allowance.md](V2-console-allowance.md) | 525 console |
| V3 | x402 overage path | ~60–75s | [V3-x402-overage.md](V3-x402-overage.md) | 506, 522 |
| V4 | Expiry cancel and past-due | ~60s | [V4-expiry-cancel.md](V4-expiry-cancel.md) | e2e + 524/525 |
| V5 | Admin reports tour | ~60–75s | [V5-admin-reports.md](V5-admin-reports.md) | admin-reports-shell |
| V6 | Enterprise self-host smoke | ~60s | [V6-enterprise-selfhost.md](V6-enterprise-selfhost.md) | enterprise-self-host |
| V7 | Canary and auto-tune (fast-follow) | ~60s | [V7-canary-autotune.md](V7-canary-autotune.md) | 528, 541 |

**Aspect:** 16:9 · **Brand:** Metrum AI (logo, dark bento UI, red→blue gradient) · **Pause:** 0.75s between non-final scenes.

**Visual source:** screen-cap of the HTML mocks (preferred) + optional nabapro stills labeled generated.

## How to approve

Reply with which clip IDs you approve, e.g. `Approve narrations V1 V2 V3` (full narration as written). If you change any wording, the complete revised narration must be re-approved.
