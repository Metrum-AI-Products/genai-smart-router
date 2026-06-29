---
title: Model Groups
doc_type: reference
---

# Model Groups

Model groups are caller-facing routing contracts. A caller sends one deployment-defined model value, and the router chooses from that group's eligible targets according to the configured strategy and request shape.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml title="config.example.yaml"
models:
  default:
    strategy: weighted
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 51
      - provider: minimax
        model_ref: m3
        weight: 27
```

## Schema

Weights are local to each model group. A target with weight `60` in one group has no relationship to a target with weight `60` in another group. Targets reference `providers.<name>.models.<ref>` and can override safe metadata such as `dialect`, `tool_only`, `input_modalities`, per-attempt timeout, traffic shaping, and validation fields.

Groups can use `static`, `failover`, `weighted`, `dynamic_score`, `script`, or `external` strategies. Contracts, request-shape filters, modalities, tools, structured outputs, reasoning metadata, and max-token safety all filter the target list before strategy selection.

## Rollback

Roll back by restoring the prior target list, reducing or removing a bad target's weight, removing a failed capability override, or switching the caller back to a previous model group allow-list. Verify with `/v1/models`, a positive smoke for expected traffic, and a negative `no-eligible-target` smoke when capability filtering changed.

## Related

See [Routing Overview](../routing/overview), [Customer-Controlled Routing](../routing/customer-controlled-routing), [Model Group Contracts](./model-group-contracts), [Dynamic Score Routing](./dynamic-score-routing), [TypeScript Routing](./routing-typescript), [External Routing Policy](./external-routing-policy), and [Router Configuration](./router-config).
