type Target = {
  provider: string;
  model: string;
  modelRef?: string;
  displayName?: string;
  dialect: string;
  baseUrl: string;
  weight: number;
  tier?: string;
  cost?: number;
  rpm?: number;
  keyId?: string;
  apiKeyEnv?: string;
  keyConfigured: boolean;
};

type RouteContext = {
  group: string;
  text: string;
  targets: Target[];
  request: {
    model: string;
    max_tokens?: number;
    stream?: boolean;
  };
};

export function route(ctx: RouteContext) {
  const eligible = ctx.targets
    .map((target, index) => ({
      target,
      index,
      weight: Math.max(0, Number(target.weight || 1)),
    }))
    .filter((entry) => entry.weight > 0 && entry.target.keyConfigured);

  if (eligible.length === 0) {
    return {
      targetIndex: 0,
      classLabel: "weighted:fallback-no-key",
    };
  }

  const totalWeight = eligible.reduce((sum, entry) => sum + entry.weight, 0);
  let pick = Math.random() * totalWeight;
  let selected = eligible[0];
  for (const entry of eligible) {
    if (pick < entry.weight) {
      selected = entry;
      break;
    }
    pick -= entry.weight;
  }

  const fallbackIndexes = eligible
    .filter((entry) => entry.index !== selected.index)
    .sort((a, b) => b.weight - a.weight)
    .map((entry) => entry.index);

  return {
    targetIndex: selected.index,
    fallbackIndexes,
    classLabel: `weighted:${selected.target.provider}:${selected.target.model}`,
  };
}
