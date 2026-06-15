---
title: TypeScript Routing Policy
---

# TypeScript Routing Policy

Model groups can delegate selection to a TypeScript routing script. The script receives request metadata, safe caller metadata, and safe target metadata. Raw caller tokens, token hashes, and raw provider API keys are not passed to scripts.

<div class="contactBanner">
  <p>Need help designing routing policy? Contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Weighted Selection With Content Rules

```typescript
type RouteInput = {
  request: {
    model: string;
    text: string;
    metadata?: Record<string, unknown>;
    hasTools: boolean;
  };
  caller?: {
    id: string;
    user: string;
    project: string;
    environment: string;
    tokenId: string;
  };
  targets: Array<{
    provider: string;
    model: string;
    dialect: string;
    weight: number;
    keyId?: string;
    apiKeyEnv?: string;
    keyConfigured: boolean;
  }>;
};

export default function route(input: RouteInput) {
  let candidates = input.targets.filter((target) => target.keyConfigured && target.weight > 0);

  if (/refactor|debug|test failure|segfault/i.test(input.request.text)) {
    candidates = candidates.filter((target) =>
      /MiniMax-M3|kimi-k2\.7-code|deepseek-v4-flash/i.test(target.model)
    );
  }

  if (/^rtr_metrum_.*_prod_/i.test(input.caller?.tokenId || "")) {
    candidates = candidates.filter((target) => target.provider !== "experimental-provider");
  }

  const total = candidates.reduce((sum, target) => sum + target.weight, 0);
  let cursor = Math.random() * total;
  for (const target of candidates) {
    cursor -= target.weight;
    if (cursor <= 0) {
      return { targetIndex: input.targets.indexOf(target), classLabel: "weighted-content-policy" };
    }
  }

  return { targetIndex: input.targets.indexOf(candidates[0]), classLabel: "weighted-content-policy" };
}
```

## External Decision Service

Routing policy can call an internal service when classification needs model telemetry, business logic, or external context.

```typescript
export default async function route(input: RouteInput) {
  const response = await fetch("https://routing-policy.example.internal/route", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      request: input.request,
      caller: input.caller,
      targets: input.targets.map((target, index) => ({
        index,
        provider: target.provider,
        model: target.model,
        dialect: target.dialect,
        weight: target.weight,
        keyId: target.keyId,
        apiKeyEnv: target.apiKeyEnv,
        keyConfigured: target.keyConfigured,
      })),
    }),
  });

  if (!response.ok) {
    return { targetIndex: 0, classLabel: "external-policy-unavailable" };
  }

  const decision = await response.json();
  return {
    targetIndex: decision.targetIndex,
    classLabel: decision.reason || "external-policy",
  };
}
```

Returned targets are validated against the configured target list before use.
