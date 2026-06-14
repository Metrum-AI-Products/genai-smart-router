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

type WeightedTarget = {
  target: Target;
  index: number;
  weight: number;
};

type KeyRule = {
  name: string;
  tokenId?: RegExp;
  user?: RegExp;
  project?: RegExp;
  environment?: RegExp;
  provider?: RegExp;
  model?: RegExp;
  tier?: RegExp;
  keyId?: RegExp;
  apiKeyEnv?: RegExp;
};

type RouteContext = {
  group: string;
  text: string;
  targets: Target[];
  caller?: {
    id: string;
    user: string;
    project: string;
    environment: string;
    tokenId: string;
    allow: string[];
  };
  request: {
    model: string;
    max_tokens?: number;
    stream?: boolean;
  };
};

const keyRules: KeyRule[] = [
  {
    name: "metrum-prod-heavy",
    tokenId: /^rtr_metrum_chetan_metrum-insights_prod_/,
    project: /^metrum-insights$/,
    environment: /^prod$/,
    tier: /^heavy$/,
  },
  {
    name: "readme-dev-openrouter",
    tokenId: /^rtr_metrum_readme_metrum-insights_dev_/,
    provider: /^openrouter/,
    apiKeyEnv: /^OPENROUTER_API_KEY$/,
  },
  {
    name: "metrum-dev-openai",
    tokenId: /^rtr_metrum_.*_metrum-insights_dev_/,
    provider: /^openai$/,
    keyId: /^openai-default$/,
    apiKeyEnv: /^OPENAI_API_KEY$/,
  },
];

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

  for (const rule of keyRules) {
    if (!matchesRule(ctx, rule)) continue;
    const preferred = eligible.filter((entry) => matchesTargetRule(entry.target, rule));
    if (preferred.length > 0) {
      return weightedPick(preferred, eligible, `key-regex:${rule.name}`);
    }
  }

  return weightedPick(eligible, eligible, "weighted");
}

function matchesRule(ctx: RouteContext, rule: KeyRule) {
  const caller = ctx.caller;
  if (!caller) return false;
  return (
    (!rule.tokenId || rule.tokenId.test(caller.tokenId)) &&
    (!rule.user || rule.user.test(caller.user)) &&
    (!rule.project || rule.project.test(caller.project)) &&
    (!rule.environment || rule.environment.test(caller.environment))
  );
}

function matchesTargetRule(target: Target, rule: KeyRule) {
  return (
    (!rule.provider || rule.provider.test(target.provider)) &&
    (!rule.model || rule.model.test(target.model)) &&
    (!rule.tier || rule.tier.test(target.tier || "")) &&
    (!rule.keyId || rule.keyId.test(target.keyId || "")) &&
    (!rule.apiKeyEnv || rule.apiKeyEnv.test(target.apiKeyEnv || ""))
  );
}

function weightedPick(
  candidates: WeightedTarget[],
  fallbackPool: WeightedTarget[],
  labelPrefix: string,
) {
  const totalWeight = candidates.reduce((sum, entry) => sum + entry.weight, 0);
  let pick = Math.random() * totalWeight;
  let selected = candidates[0];
  for (const entry of candidates) {
    if (pick < entry.weight) {
      selected = entry;
      break;
    }
    pick -= entry.weight;
  }

  const fallbackIndexes = fallbackPool
    .filter((entry) => entry.index !== selected.index)
    .sort((a, b) => b.weight - a.weight)
    .map((entry) => entry.index);

  return {
    targetIndex: selected.index,
    fallbackIndexes,
    classLabel: `${labelPrefix}:${selected.target.provider}:${selected.target.model}`,
  };
}
