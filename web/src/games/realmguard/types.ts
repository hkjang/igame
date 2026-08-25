import type { KernelLedger } from "./kernel/ledger";

export type RealmDifficulty = "casual" | "normal" | "veteran";
export type RealmMode = "campaign" | "endless";
export type TargetingMode = "first" | "last" | "strong" | "weak" | "closest";
export type RealmSection =
  | "stages"
  | "waves"
  | "enemies"
  | "bosses"
  | "towers"
  | "heroes"
  | "skills"
  | "balance";

export interface Point {
  x: number;
  y: number;
}

export interface WaveEntry {
  enemy: string;
  count: number;
  interval: number;
  delay?: number;
  pathIndex?: number;
  parallel?: boolean;
  modifiers?: string[];
}

export interface RealmWave {
  id: string;
  label: string;
  entries: WaveEntry[];
  reward: number;
}

export interface TowerSpot extends Point {
  id: string;
}

export interface RealmStage {
  id: string;
  number: number;
  name: string;
  subtitle: string;
  mode: RealmMode;
  theme: "verdant" | "ember" | "frost" | "void";
  path: Point[];
  paths?: Point[][];
  towerSpots: TowerSpot[];
  waves: RealmWave[];
  startingGold: number;
  lives: number;
  version: string;
  /** Visual battlefield identity. Unknown values safely use the theme fallback. */
  mapStyle?: string;
  gimmick?: "ember_vents" | "winter_blessing" | "time_surge";
}

export interface EnemyArchetype {
  id: string;
  name: string;
  color: number;
  hp: number;
  speed: number;
  armor: number;
  reward: number;
  lifeDamage: number;
  radius: number;
  traits: Array<
    | "armored"
    | "swift"
    | "flying"
    | "regenerating"
    | "healer"
    | "splitting"
    | "phasing"
    | "siege"
    | "boss"
    | "magic_resist"
    | "stealth"
    | "berserk"
    | "immune_stun"
  >;
  threatType?: string;
  resourceEffect?: {
    compute?: number;
    token?: number;
    trust?: number;
    latency?: number;
  };
}

export interface TowerBranch {
  id: string;
  name: string;
  description: string;
  damageMultiplier?: number;
  rangeMultiplier?: number;
  rateMultiplier?: number;
  splash?: number;
  slow?: number;
  pierce?: number;
}

export interface TowerDefinition {
  id: string;
  name: string;
  role: string;
  color: number;
  cost: number;
  damage: number;
  range: number;
  fireRate: number;
  projectileSpeed: number;
  damageType: "physical" | "magic" | "true" | "arcane" | "siege" | "frost";
  branches: TowerBranch[];
  effectiveAgainst?: string[];
  effectiveMultiplier?: number;
  profiles?: Array<{ id: string; name: string; damageMultiplier: number }>;
}

export interface HeroDefinition {
  id: string;
  name: string;
  title: string;
  color: number;
  hp: number;
  damage: number;
  range: number;
  speed: number;
  respawnSeconds: number;
  skill1: string;
  skill2: string;
  ultimate: string;
  /** Campaign stage number at which this hero becomes selectable. */
  unlockStage?: number;
}

export interface SkillDefinition {
  id: string;
  name: string;
  description: string;
  cooldown: number;
  color: string;
}

export interface RealmBalance {
  difficulties: Record<
    RealmDifficulty,
    { enemyHp: number; enemySpeed: number; gold: number; score: number }
  >;
  towerUpgradeCost: number[];
  heroLevelXp: number[];
  endlessRamp: number;
  endlessWaveBonus: number;
  sellRefundRate: number;
  difficultyBonus: Record<RealmDifficulty, number>;
  clearTimeBonusPerSecond: number;
  parTimeSeconds: number;
}

export interface RealmGuardConfig {
  versionId: string;
  contentVersion: string;
  balanceVersion: string;
  assetVersion: string;
  stages: RealmStage[];
  enemies: EnemyArchetype[];
  towers: TowerDefinition[];
  heroes: HeroDefinition[];
  skills: SkillDefinition[];
  balance: RealmBalance;
}

export interface StageProgress {
  stage_id: string;
  stars: number;
  best_score: number;
  difficulties: RealmDifficulty[];
}

export interface RealmProgress {
  total_stars: number;
  unlocked_stage: number;
  hero_levels: Record<string, number>;
  heroes: Record<string, { unlocked: boolean; level: number; xp: number }>;
  skills: Record<string, { unlocked: boolean; level: number }>;
  loadout: {
    hero_id: string;
    skill_ids: string[];
    settings?: Record<string, unknown>;
  };
  campaign_completed: boolean;
  stages: StageProgress[];
}

export interface BattleStats {
  stage_id: string;
  mode: RealmMode;
  difficulty: RealmDifficulty;
  duration_ms: number;
  lives: number;
  gold: number;
  earned_gold: number;
  spent_gold: number;
  sold_gold: number;
  kills: number;
  waves: number;
  waves_completed: number;
  escaped: number;
  spawned: number;
  defeated_by_enemy: Record<string, number>;
  escaped_by_enemy: Record<string, number>;
  spawned_by_enemy: Record<string, number>;
  hero_id: string;
  hero_level: number;
  content_version: string;
  balance_version: string;
  stage_version: string;
  asset_version: string;
  /** Player input record the server replays; absent when it could not be kept. */
  ledger?: KernelLedger;
}

export interface RealmResult extends BattleStats {
  victory: boolean;
  score: number;
  stars: number;
  verified?: boolean;
}

export interface BattleHUD {
  status: "ready" | "playing" | "paused" | "victory" | "defeat";
  gold: number;
  lives: number;
  wave: number;
  totalWaves: number;
  kills: number;
  heroLevel: number;
  heroHp: number;
  heroMaxHp: number;
  heroAlive: boolean;
  heroRespawn: number;
  nextWaveIn: number;
  selectedSpot?: string;
  selectedTower?: {
    type: string;
    level: number;
    branch?: string;
    profile?: string;
    targeting: TargetingMode;
  };
  skillCooldowns: Record<string, number>;
  speed: 1 | 2;
}

export type RealmCommand =
  | { type: "start-wave" }
  | { type: "toggle-pause" }
  | { type: "speed"; value: 1 | 2 }
  | { type: "build"; tower: string; profile?: string }
  | { type: "upgrade"; branch?: string }
  | { type: "sell" }
  | { type: "targeting"; mode: TargetingMode }
  | { type: "skill"; skill: string }
  | { type: "move-hero" }
  | { type: "adjust-economy"; resourceDelta: number; healthDelta?: number }
  | { type: "force-defeat" };

export interface RealmSceneController {
  command(command: RealmCommand): void;
  destroy(): void;
}
