import type {
  RealmDifficulty,
  RealmGuardConfig,
  RealmStage,
} from "../types";
import type {
  KernelBranch,
  KernelConfig,
  KernelEnemy,
  KernelProfile,
  KernelStage,
  KernelTower,
  KernelWave,
} from "./types";

/**
 * Blocking towers hold ground units instead of shooting past them. The rule has
 * always been "the barracks archetype plus whatever the pack puts last", so the
 * projection resolves it once rather than letting each call site guess.
 */
export function blockingTowerIds(config: RealmGuardConfig): string[] {
  const ids = new Set<string>();
  if (config.towers.some((tower) => tower.id === "windward")) ids.add("windward");
  const last = config.towers.at(-1)?.id;
  if (last) ids.add(last);
  return [...ids].sort();
}

function lanesOf(stage: RealmStage) {
  const lanes = stage.paths?.length ? stage.paths : [stage.path];
  return lanes.map((lane) => lane.map((point) => ({ x: point.x, y: point.y })));
}

function projectWaves(stage: RealmStage): KernelWave[] {
  return stage.waves.map((wave) => ({
    reward: wave.reward,
    entries: wave.entries.map((entry) => ({
      enemy: entry.enemy,
      count: entry.count,
      interval: entry.interval,
      delay: entry.delay ?? 0,
      pathIndex: Math.max(0, Math.floor(entry.pathIndex ?? 0)),
      parallel: Boolean(entry.parallel),
      modifiers: [...(entry.modifiers ?? [])],
    })),
  }));
}

function projectStage(stage: RealmStage): KernelStage {
  return {
    id: stage.id,
    mode: stage.mode,
    lives: stage.lives,
    startingGold: stage.startingGold,
    gimmick: stage.gimmick ?? "",
    lanes: lanesOf(stage),
    spots: stage.towerSpots.map((spot) => ({ id: spot.id, x: spot.x, y: spot.y })),
    waves: projectWaves(stage),
  };
}

function projectEnemies(config: RealmGuardConfig): KernelEnemy[] {
  return config.enemies.map((enemy) => ({
    id: enemy.id,
    hp: enemy.hp,
    speed: enemy.speed,
    armor: enemy.armor,
    reward: enemy.reward,
    lifeDamage: enemy.lifeDamage,
    radius: enemy.radius,
    traits: [...enemy.traits],
    threatType: enemy.threatType ?? "",
  }));
}

function projectTowers(config: RealmGuardConfig): KernelTower[] {
  const blocking = new Set(blockingTowerIds(config));
  return config.towers.map((tower) => ({
    id: tower.id,
    cost: tower.cost,
    damage: tower.damage,
    range: tower.range,
    fireRate: tower.fireRate,
    damageType: tower.damageType,
    blocking: blocking.has(tower.id),
    effectiveAgainst: [...(tower.effectiveAgainst ?? [])],
    effectiveMultiplier: tower.effectiveMultiplier ?? -1,
    branches: tower.branches.map(
      (branch): KernelBranch => ({
        id: branch.id,
        damageMultiplier: branch.damageMultiplier ?? 1,
        rangeMultiplier: branch.rangeMultiplier ?? 1,
        rateMultiplier: branch.rateMultiplier ?? 1,
        splash: branch.splash ?? -1,
        slow: branch.slow ?? -1,
        pierce: branch.pierce ?? 0,
      }),
    ),
    profiles: (tower.profiles ?? []).map(
      (profile): KernelProfile => ({
        id: profile.id,
        damageMultiplier: profile.damageMultiplier,
      }),
    ),
  }));
}

/**
 * The published content narrowed to one player's loadout.
 *
 * Skills unlock as a campaign progresses, so a battle usually runs on fewer
 * than the three skills the content declares. The narrowing has to be written
 * once: the browser projects the result, the server reprojects it from the
 * ledger's `skill_ids`, and the digest check refuses the battle if the two
 * disagree by so much as an ordering.
 */
export function withLoadout<T extends RealmGuardConfig>(config: T, skillIds: readonly string[]): T {
  return {
    ...config,
    skills: config.skills.filter((skill) => skillIds.includes(skill.id)).slice(0, KERNEL_LOADOUT_LIMIT),
  };
}

/** The loadout size the engine supports; the content declares exactly this many. */
export const KERNEL_LOADOUT_LIMIT = 3;

export function projectKernelConfig(
  config: RealmGuardConfig,
  stage: RealmStage,
  difficulty: RealmDifficulty,
  heroId: string,
): KernelConfig {
  const hero =
    config.heroes.find((item) => item.id === heroId) ?? config.heroes[0];
  const balance = config.balance.difficulties[difficulty];
  return {
    difficulty,
    stage: projectStage(stage),
    enemies: projectEnemies(config),
    towers: projectTowers(config),
    hero: {
      id: hero.id,
      hp: hero.hp,
      damage: hero.damage,
      range: hero.range,
      speed: hero.speed,
      respawnSeconds: hero.respawnSeconds,
    },
    skills: config.skills.map((skill) => ({
      id: skill.id,
      cooldown: skill.cooldown,
    })),
    balance: {
      enemyHp: balance.enemyHp,
      enemySpeed: balance.enemySpeed,
      gold: balance.gold,
      towerUpgradeCost: [...config.balance.towerUpgradeCost],
      heroLevelXp: [...config.balance.heroLevelXp],
      endlessRamp: config.balance.endlessRamp,
      sellRefundRate: config.balance.sellRefundRate,
    },
  };
}

/**
 * Fixed decimal rendering. `JSON.stringify` and Go's encoder disagree on the
 * shortest representation of some doubles, so the digest never depends on
 * either one.
 */
export function canonicalNumber(value: number): string {
  if (!Number.isFinite(value)) return "0";
  const normalized = value === 0 ? 0 : value;
  if (Number.isInteger(normalized) && Math.abs(normalized) < 1e15)
    return String(normalized);
  let text = normalized.toFixed(6);
  if (text.includes(".")) text = text.replace(/0+$/, "").replace(/\.$/, "");
  return text === "-0" ? "0" : text;
}

export function canonicalJSON(value: unknown): string {
  if (value === null || value === undefined) return "null";
  if (typeof value === "number") return canonicalNumber(value);
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "string") return JSON.stringify(value);
  if (Array.isArray(value))
    return `[${value.map((item) => canonicalJSON(item)).join(",")}]`;
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, item]) => item !== undefined)
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0));
  return `{${entries
    .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
    .join(",")}}`;
}

const FNV_OFFSET = 0xcbf29ce484222325n;
const FNV_PRIME = 0x100000001b3n;
const MASK = 0xffffffffffffffffn;

/** FNV-1a/64 over the canonical bytes; the server recomputes it independently. */
export function kernelDigest(config: KernelConfig): string {
  const bytes = new TextEncoder().encode(canonicalJSON(config));
  let hash = FNV_OFFSET;
  for (const byte of bytes) {
    hash = (hash ^ BigInt(byte)) & MASK;
    hash = (hash * FNV_PRIME) & MASK;
  }
  return hash.toString(16).padStart(16, "0");
}
