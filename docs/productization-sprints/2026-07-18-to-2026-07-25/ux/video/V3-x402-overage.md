# V3 — x402 overage path (approval package)

**Status:** AWAITING NARRATION APPROVAL  
**Target:** ~70s · 16:9 · Metrum brand · pause 0.75s  

## Complete narration

**Scene 1.** While your included allowance covers the quote, the router reserves locally and calls upstream. Stripe is never on this path.

**Scene 2.** When remaining allowance cannot cover the request, admission stops before upstream and returns HTTP 402 with an x402 payment challenge.

**Scene 3.** Your compatible client signs the payment and retries the identical request. The quote stays frozen—no re-pricing mid-retry.

**Scene 4.** The router verifies and settles once, then performs exactly one upstream call, and posts a settlement event to the ledger.

**Scene 5.** If the proof is expired or replayed, you get a fresh challenge. If the facilitator is down, you see a retryable payment-unavailable error—still with no upstream spend.

## Visual intent

| Scene | Mock |
| ---: | --- |
| 1 | 522 admission “within allowance” |
| 2–4 | 506 x402 mock |
| 5 | 522/506 error tabs |

## Source notes

Issues #522 #506; D11 error types; D3 overage rail.
