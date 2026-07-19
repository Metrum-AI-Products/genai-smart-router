# 15 — Plan Catalog: SKUs, Included Allowance, and x402 Price Bands (#540)

Plan version: **v1.0.0**  
Issue: [#540](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/540)  
Classification: **launch blocking / commercial contract**

## Role

Provides the versioned plan definitions that #522, #524, #506, #525, #533, and #534 consume. Without this catalog, no issue can settle a correct price or grant the correct included allowance.

## Schema

Relational `plan_skus`, `plan_versions`, `plan_model_groups`, `plan_included_units`, `plan_x402_bands`, `plan_feature_flags`, `plan_activations`. No JSON/JSONB. Stripe Price IDs are safe non-secret references.

## Product flow

Finance agrees allowed model groups, included token amounts per period, and x402 price bands per model group/request-shape bucket. A reviewed migration seeds the catalog. #521 activates a plan version per tenant. #522 quotes against active plan. #524 maps Stripe subscription Price ID. #506 quotes x402 bands. #525 displays plan details and allowance remaining.

## Seed data

Staging uses clearly non-production prices (e.g. 1 token = 1 microcredit). Never commit real customer pricing as the only catalog entry without Finance sign-off.

## Tests

Validation rejects zero/negative included units, overlapping bands, unknown Stripe references. Upgrade/downgrade does not change in-flight period grants. Catalog version stamped on every entitlement and allowed processing artifact.
