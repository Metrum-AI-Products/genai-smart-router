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

  const keyRules: Array<{
    tokenId?: RegExp;
    user?: RegExp;
    project?: RegExp;
    environment?: RegExp;
    provider?: RegExp;
    model?: RegExp;
    tier?: RegExp;
  }> = [
    // Example: route one project's production keys to heavy coding targets.
    // { tokenId: /^rtr_metrum_chetan_metrum-insights_prod_/, tier: /^heavy$/ },
  ];

  for (const rule of keyRules) {
    if (!matchesRule(ctx, rule)) continue;
    const preferred = eligible.filter((entry) => {
      const target = entry.target;
      return (
        (!rule.provider || rule.provider.test(target.provider)) &&
        (!rule.model || rule.model.test(target.model)) &&
        (!rule.tier || rule.tier.test(target.tier || ""))
      );
    });
    if (preferred.length > 0) {
      return weightedPick(preferred, eligible, `key-regex:${ctx.caller?.tokenId || ctx.caller?.id || "anonymous"}`);
    }
  }

  return weightedPick(eligible, eligible, "weighted");
}

function matchesRule(ctx: RouteContext, rule: { tokenId?: RegExp; user?: RegExp; project?: RegExp; environment?: RegExp }) {
  const caller = ctx.caller;
  if (!caller) return false;
  return (
    (!rule.tokenId || rule.tokenId.test(caller.tokenId)) &&
    (!rule.user || rule.user.test(caller.user)) &&
    (!rule.project || rule.project.test(caller.project)) &&
    (!rule.environment || rule.environment.test(caller.environment))
  );
}

function weightedPick(
  candidates: Array<{ target: Target; index: number; weight: number }>,
  fallbackPool: Array<{ target: Target; index: number; weight: number }>,
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
