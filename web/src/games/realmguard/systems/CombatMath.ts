import type { EnemyArchetype, TowerDefinition } from "../types";

export function calculateTowerStats(
  definition: TowerDefinition,
  level: number,
  branchId?: string,
) {
  const levelDamage = [1, 1.45, 2.05][level - 1] ?? 1;
  const levelRange = [1, 1.08, 1.16][level - 1] ?? 1;
  const branch = definition.branches.find((item) => item.id === branchId);
  return {
    damage: definition.damage * levelDamage * (branch?.damageMultiplier ?? 1),
    range: definition.range * levelRange * (branch?.rangeMultiplier ?? 1),
    fireRate: definition.fireRate * (branch?.rateMultiplier ?? 1),
    splash: branch?.splash ?? (definition.damageType === "siege" ? 48 : 0),
    slow:
      branch?.slow ??
      (definition.id === "windward"
        ? 0.52
        : definition.damageType === "frost"
          ? 0.2
          : 0),
    pierce: branch?.pierce ?? 0,
  };
}

export function effectiveDamage(
  amount: number,
  type: TowerDefinition["damageType"] | "hero" | "skill",
  armor: number,
  traits: Set<string>,
) {
  const baseArmor = Math.min(0.75, armor + (traits.has("armored") ? 0.18 : 0));
  const magic = type === "arcane" || type === "magic";
  const physicalArmor =
    type === "true" || magic || type === "skill"
      ? 0
      : type === "siege"
        ? baseArmor * 0.45
        : baseArmor;
  const magicResistance = magic && traits.has("magic_resist") ? 0.48 : 0;
  return Math.max(1, amount * (1 - physicalArmor) * (1 - magicResistance));
}

export function towerEffectivenessMultiplier(
  tower: Pick<TowerDefinition, "effectiveAgainst" | "effectiveMultiplier">,
  enemy: Pick<EnemyArchetype, "threatType">,
) {
  return enemy.threatType && tower.effectiveAgainst?.includes(enemy.threatType)
    ? Math.max(1, tower.effectiveMultiplier ?? 1.5)
    : 1;
}

export function movementMultiplier(
  traits: Set<string>,
  hpRatio: number,
  slowed: boolean,
  slowFactor: number,
  hasted: boolean,
) {
  const slow = slowed && !traits.has("immune_stun") ? slowFactor : 1;
  const berserk = traits.has("berserk") && hpRatio <= 0.35 ? 1.5 : 1;
  return slow * (hasted ? 1.12 : 1) * berserk;
}

export function mergedTraits(
  definition: Pick<EnemyArchetype, "traits">,
  modifiers: Set<string>,
) {
  return new Set<string>([...definition.traits, ...modifiers]);
}
