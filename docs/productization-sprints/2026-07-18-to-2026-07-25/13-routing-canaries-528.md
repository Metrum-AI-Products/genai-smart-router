# 13 — Experimental-Model Canary Delivery and Auto-Rollback (#528)

Plan version: **v1.0.0**  
Issue: [#528](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/528)  
Classification: **fast follow unless launch requires a new target**

## Design approach and operator UI

Extend #505, #7, model-group contracts, weighted/dynamic-score routing and existing
safe diagnostic tables. A canary is a versioned config proposal with exact target,
provider/model/API skin, eligible request shapes, validation evidence, exposure,
quality/cost/SLO gates, soak, owner, expiry and rollback config. Never treat a model
pass as a provider/API-skin/request-shape pass.

The operator preview is committed at
[canary-operations.html](previews/canary-operations.html). It shows proposal diff,
direct/router smoke evidence, shape gates, shadow metrics, canary weight/stage,
quality/latency/error/cost thresholds, current exposure, automatic rollback state,
audit timeline, and approve/promote/abort controls. Production changes require
reauth, reason and human approval; normal Prosumer users do not see experimental
targets unless explicitly enrolled in a deployment-defined program.

## Promotion flow

1. Catalog metadata and source-dated pricing/capabilities are current (#392).
2. Direct upstream text, realistic image if claimed, tool/forced-tool if claimed,
   streaming and exact output-cap tests pass for each intended API skin.
3. Router staging repeats OpenAI Chat, Responses, Anthropic Messages and bridge
   directions with representative large tools/schemas/images/byte buckets.
4. Create isolated smoke/staging group and objective evaluator/golden dataset.
5. Shadow records what would be selected and verifier result without affecting the
   response. No ungoverned content is persisted.
6. Canary at small bounded tenant/request percentage/weight with exact shape
   eligibility. Hold through minimum requests/time and compare control.
7. Automatic rollback restores previous config fingerprint when any terminal gate
   breaches. Promotion increases one deliberate stage at a time.
8. Full promotion occurs only after quality contract, cost/latency and client
   acceptance; keep prior config as rollback artifact.

## Human interaction and configuration

Model owner defines workloads/verifier and direct evidence; Product defines group
contract; SRE defines SLO/traffic/soak/rollback; Finance approves price/margin;
Security approves provider/data handling; API owners approve each skin/bridge.
Promotion/rollback roles are separated from proposal author where practical.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Provider accounts
and exact entitled model IDs are environment-owned; provider API keys remain in
data-plane Secrets Manager paths and never enter canary DB/UI/evidence. The canary
worker/operator uses #7 config activation identity, metrics/report read identity,
evaluator dataset/result store and notification integration. Non-secrets: provider/
model/dialect/skin refs, price metadata/source date, modalities/tools/structured/
reasoning/output-cap and request-shape support, direct/router validation timestamps,
group/target weights, tenant/shape exposure, evaluator/version/golden dataset safe
ID, minimum sample/time, quality/error/latency/cost thresholds, config fingerprints,
approver and rollback version.

## Test plan

* Unit-test state machine, authorization, metric windows/minimum samples, comparison,
  config generation, exact shape filtering, trigger precedence and idempotent rollback.
* Direct and router smoke matrix for every asserted capability/skin with realistic
  caller payloads and production-derived synthetic incident fixtures.
* Shadow test proves response/selected target are unchanged while bounded evidence
  is recorded.
* Canary selection test proves only enrolled tenant/shape and configured percentage
  reach target; normal groups/users cannot request the target directly.
* Fault inject upstream 400/429/5xx/timeout, empty content, quality regression,
  p95/TTFB, cost/margin, settlement mismatch, monitoring loss and config activation
  failure. Each required trigger automatically restores prior fingerprint.
* Streaming/tool/image/client tests cover Codex/Claude Code caller surfaces where
  group contract includes them; tiny cap is enforcement, not vision acceptance.
* Load/soak compares control/canary with statistical minimums and no tenant label/
  prompt leakage. Capture terminal traffic, dashboard, config diff, audit and
  rollback replay.

## Monitoring and rollback

Use request/attempt/error/shape/candidate/filter/policy/fallback/cost/latency/TTFB/
throughput fields plus objective evaluator outcomes. Alert on monitoring
incompleteness and pause canary rather than assume health. Rollback is a #7 active
config change, not a blanket provider skin switch; confirm new requests stop
selecting target and in-flight/settlement completes safely.

## Definition of done

One deliberately failing canary rolls back automatically and one passing canary
promotes through staging with complete safe evidence. No target enters broad
routing without exact request-shape and API-skin proof.
