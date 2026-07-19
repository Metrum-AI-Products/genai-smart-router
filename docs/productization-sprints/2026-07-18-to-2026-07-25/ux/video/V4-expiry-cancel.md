# V4 — Expiry, cancel, and past-due (approval package)

**Status:** AWAITING NARRATION APPROVAL  
**Target:** ~60s · 16:9 · Metrum brand · pause 0.75s  

## Complete narration

**Scene 1.** On successful renewal, the subscription manager issues a new period allowance grant and retires the expired one. Your console allowance resets.

**Scene 2.** If payment fails, the entitlement moves past-due. After the approved grace window, the router denies new traffic with entitlement-past-due and a portal link.

**Scene 3.** When you cancel, future grants and keys are revoked first. In-flight work drains. Financial ledger history is retained.

**Scene 4.** Teardown removes only approved ephemeral resources after retention rules. Your inference path never called Stripe—and it still doesn’t during cancel.

## Visual intent

Console suspended/cancelled banners; 526 teardown notes; 524 portal.

## Source notes

e2e-subscription.md sections 11–12; issues #524 #526 #525.
