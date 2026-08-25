/**
 * Kernel projection types.
 *
 * The kernel is the single authority for RealmGuard and Defense Series battle
 * rules. Everything it reads lives in this projection so the browser and the Go
 * replay verifier can be proven to start from byte-identical inputs.
 */

export interface KernelPoint {
  x: number;
  y: number;
}

export interface KernelSpot extends KernelPoint {
  id: string;
}

export interface KernelEnemy {
  id: string;
  hp: number;
  speed: number;
  armor: number;
  reward: number;
  lifeDamage: number;
  radius: number;
  traits: string[];
  threatType: string;
}

export interface KernelBranch {
  id: string;
  damageMultiplier: number;
  rangeMultiplier: number;
  rateMultiplier: number;
  splash: number;
  slow: number;
  pierce: number;
}

export interface KernelProfile {
  id: string;
  damageMultiplier: number;
}

export interface KernelTower {
  id: string;
  cost: number;
  damage: number;
  range: number;
  fireRate: number;
  damageType: string;
  blocking: boolean;
  effectiveAgainst: string[];
  effectiveMultiplier: number;
  branches: KernelBranch[];
  profiles: KernelProfile[];
}

export interface KernelWaveEntry {
  enemy: string;
  count: number;
  interval: number;
  delay: number;
  pathIndex: number;
  parallel: boolean;
  modifiers: string[];
}

export interface KernelWave {
  entries: KernelWaveEntry[];
  reward: number;
}

export interface KernelStage {
  id: string;
  mode: "campaign" | "endless";
  lives: number;
  startingGold: number;
  gimmick: string;
  lanes: KernelPoint[][];
  spots: KernelSpot[];
  waves: KernelWave[];
}

export interface KernelHero {
  id: string;
  hp: number;
  damage: number;
  range: number;
  speed: number;
  respawnSeconds: number;
}

export interface KernelSkill {
  id: string;
  cooldown: number;
}

export interface KernelBalance {
  enemyHp: number;
  enemySpeed: number;
  gold: number;
  towerUpgradeCost: number[];
  heroLevelXp: number[];
  endlessRamp: number;
  sellRefundRate: number;
}

export interface KernelConfig {
  difficulty: string;
  stage: KernelStage;
  enemies: KernelEnemy[];
  towers: KernelTower[];
  hero: KernelHero;
  skills: KernelSkill[];
  balance: KernelBalance;
}

/** Outcome the kernel produces; the server recomputes this from the ledger. */
export interface KernelOutcome {
  victory: boolean;
  ticks: number;
  duration_ms: number;
  lives: number;
  gold: number;
  earned_gold: number;
  spent_gold: number;
  sold_gold: number;
  kills: number;
  escaped: number;
  spawned: number;
  waves_completed: number;
  hero_level: number;
  defeated_by_enemy: Record<string, number>;
  escaped_by_enemy: Record<string, number>;
  spawned_by_enemy: Record<string, number>;
}
