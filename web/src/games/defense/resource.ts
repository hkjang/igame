import type { AIResources, DefenseContentPack } from "./types";

export type AIResourceChange = Partial<Record<keyof AIResources, number>>;

export function initialAIResources(pack: DefenseContentPack): AIResources {
  const rules = pack.resourceRules;
  return rules
    ? {
        compute: rules.compute_start,
        token: rules.token_start,
        trust: rules.trust_start,
        latency: rules.latency_max,
      }
    : { compute: 1000, token: 1000, trust: 100, latency: 100 };
}

export function buildAIResourceState(
  pack: DefenseContentPack,
  values: AIResources,
) {
  const starts = initialAIResources(pack);
  return Object.fromEntries(
    (Object.keys(starts) as Array<keyof AIResources>).map((key) => [
      key,
      {
        start: starts[key],
        spent: Math.max(0, starts[key] - values[key]),
        remaining: values[key],
      },
    ]),
  );
}

export function isAIResourceDepleted(values: AIResources) {
  return Object.values(values).some((value) => value <= 0);
}

export function aiDepletionDisposition(
  values: AIResources,
  educationPromptOpen: boolean,
) {
  if (!isAIResourceDepleted(values)) return "none" as const;
  return educationPromptOpen ? ("defer" as const) : ("defeat" as const);
}

export function defenseTelemetryUsesAIResourceState(event: string) {
  return [
    "defense.battle.ready",
    "defense.wave.start",
    "defense.wave.complete",
    "defense.tower.build",
    "defense.battle.complete",
  ].includes(event);
}

export function aiResourcePercent(
  pack: DefenseContentPack,
  key: keyof AIResources,
  remaining: number,
) {
  const start = initialAIResources(pack)[key];
  return Math.max(0, Math.min(100, start > 0 ? (remaining / start) * 100 : 0));
}

export function applyAIResourceCosts(
  pack: DefenseContentPack,
  values: AIResources,
  costs: AIResourceChange,
): AIResources {
  const limits = initialAIResources(pack);
  return Object.fromEntries(
    (Object.keys(limits) as Array<keyof AIResources>).map((key) => [
      key,
      Math.max(
        0,
        Math.min(
          limits[key],
          values[key] - Math.max(0, Number(costs[key] ?? 0)),
        ),
      ),
    ]),
  ) as unknown as AIResources;
}

export function applyAIResourceDeltas(
  pack: DefenseContentPack,
  values: AIResources,
  deltas: AIResourceChange,
): AIResources {
  const limits = initialAIResources(pack);
  return Object.fromEntries(
    (Object.keys(limits) as Array<keyof AIResources>).map((key) => [
      key,
      Math.max(
        0,
        Math.min(limits[key], values[key] + Number(deltas[key] ?? 0)),
      ),
    ]),
  ) as unknown as AIResources;
}

export function aiEscapedResourceCosts(
  pack: DefenseContentPack,
  cumulative: Record<string, number>,
  previous: Record<string, number>,
) {
  const costs: Record<keyof AIResources, number> = {
    compute: 0,
    token: 0,
    trust: 0,
    latency: 0,
  };
  let escapedTotal = 0;
  for (const [enemyId, countValue] of Object.entries(cumulative)) {
    const total = Math.max(0, Math.floor(Number(countValue)));
    const escaped = Math.max(0, total - (previous[enemyId] ?? 0));
    escapedTotal += escaped;
    const effect = pack.config.enemies.find(
      (enemy) => enemy.id === enemyId,
    )?.resourceEffect;
    if (!effect) continue;
    for (const key of Object.keys(costs) as Array<keyof AIResources>)
      costs[key] += Math.max(0, Number(effect[key] ?? 0)) * escaped;
  }
  costs.trust +=
    escapedTotal * Math.max(0, pack.resourceRules?.escaped_trust_cost ?? 0);
  costs.latency +=
    escapedTotal * Math.max(0, pack.resourceRules?.escaped_latency_cost ?? 0);
  return {
    costs,
    cumulative: Object.fromEntries(
      Object.entries(cumulative).map(([key, value]) => [
        key,
        Math.max(0, Math.floor(Number(value))),
      ]),
    ),
  };
}
