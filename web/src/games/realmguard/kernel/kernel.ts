import { effectiveDamage, mergedTraits, movementMultiplier } from "../systems/CombatMath";
import { closestPointOnPaths, normalizedPathProgress } from "../systems/PathSystem";
import { targetComparator } from "../systems/TargetSystem";
import {
  canCompleteWave,
  canResolveCampaignVictory,
  expandWave,
} from "../systems/WaveSystem";
import type { EnemyArchetype, TargetingMode, TowerDefinition } from "../types";
import type { KernelCommand } from "./ledger";
import type {
  KernelConfig,
  KernelEnemy,
  KernelOutcome,
  KernelPoint,
  KernelTower,
} from "./types";

export const KERNEL_TICK_MS = 50;
export const KERNEL_WIDTH = 1280;
export const KERNEL_HEIGHT = 720;

const TARGETING: TargetingMode[] = ["first", "last", "strong", "weak", "closest"];
type DamageSource = TowerDefinition["damageType"] | "hero" | "skill";

export type KernelEvent =
  | { k: "spawn"; id: number }
  | { k: "despawn"; id: number; killed: boolean }
  | { k: "hp"; id: number }
  | { k: "shot"; spot: string; x: number; y: number; radius: number }
  | { k: "melee"; spot: string; x: number; y: number }
  | { k: "hero-shot"; id: number }
  | { k: "hero-respawn" }
  | { k: "hero-hit" }
  | { k: "hero-level" }
  | { k: "hero-wide" }
  | { k: "hero-ultimate" }
  | {
      k: "tower";
      spot: string;
      change: "build" | "upgrade" | "sell" | "disable";
    }
  | { k: "heal"; x: number; y: number }
  | { k: "meteor"; x: number; y: number }
  | { k: "reinforce"; x: number; y: number }
  | { k: "freeze" }
  | { k: "gimmick"; kind: string; x: number; y: number }
  | { k: "boss-phase"; id: number; phase: number }
  | { k: "telemetry"; event: string; data?: Record<string, unknown> }
  | { k: "complete"; victory: boolean };

export interface KernelEnemyView {
  id: number;
  def: number;
  hp: number;
  maxHp: number;
  x: number;
  y: number;
  alive: boolean;
  modifiers: Set<string>;
}

interface Enemy extends KernelEnemyView {
  speed: number;
  lane: number;
  pathIndex: number;
  pathProgress: number;
  slowUntil: number;
  slowFactor: number;
  healAt: number;
  hasteUntil: number;
  lastAttack: number;
  phases: Set<string>;
}

export interface KernelTowerView {
  spotId: string;
  def: number;
  x: number;
  y: number;
  level: number;
  branch: string;
  profile: string;
  targeting: TargetingMode;
  disabledUntil: number;
  soldiers: KernelPoint[];
}

interface Tower extends KernelTowerView {
  lastShot: number;
  blocked: Set<number>;
}

export interface KernelHeroView {
  x: number;
  y: number;
  level: number;
  hp: number;
  maxHp: number;
  deadUntil: number;
}

interface Hero extends KernelHeroView {
  targetX: number;
  targetY: number;
  xp: number;
  lastShot: number;
  attackCount: number;
}

interface Reinforcement {
  x: number;
  y: number;
  nextStrikeAt: number;
  expiresAt: number;
}

interface SpawnOrder {
  enemy: string;
  at: number;
  pathIndex: number;
  modifiers: string[];
}

/** sqrt of the squared sum, not `Math.hypot`, so Go replays bit-identically. */
function distance(ax: number, ay: number, bx: number, by: number) {
  const dx = bx - ax;
  const dy = by - ay;
  return Math.sqrt(dx * dx + dy * dy);
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

/** Replaces the renderer's RNG so split and summon placement replays exactly. */
function jitter(sequence: number, span: number) {
  const hash = Math.imul(sequence, 2654435761) >>> 0;
  return (hash % (2 * span + 1)) - span;
}

export interface KernelStatus {
  status: "ready" | "playing" | "victory" | "defeat";
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
  skillCooldowns: Record<string, number>;
}

/**
 * The authoritative RealmGuard and Defense Series battle simulation.
 *
 * It owns every rule that decides a score and touches no renderer: a fixed 50ms
 * step, no wall clock, no RNG, and only add/sub/mul/div/sqrt so the Go verifier
 * reproduces the same doubles. The scene draws whatever this reports.
 */
export class BattleKernel {
  readonly config: KernelConfig;

  private readonly events: KernelEvent[] = [];
  private readonly enemies: Enemy[] = [];
  private readonly towers: Tower[] = [];
  private readonly skillReady = new Map<string, number>();
  private readonly lanes: KernelPoint[][];
  private readonly hero: Hero;
  private readonly heroDamage: number;
  private reinforcements: Reinforcement[] = [];
  private armed = "";
  private enemySequence = 0;
  private hitSequence = 0;
  private spawnQueue: SpawnOrder[] = [];

  private gold = 0;
  private lives = 0;
  private kills = 0;
  private escaped = 0;
  private spawned = 0;
  private earnedGold = 0;
  private spentGold = 0;
  private soldGold = 0;
  private waveIndex = 0;
  private waveActive = false;
  private waveStartedAt = 0;
  private nextWaveAt = 10_000;
  private nextGimmickAt = 12_000;
  private simulationTime = 0;
  private tickCount = 0;
  private completed = false;
  private victory = false;
  private readonly defeatedByEnemy: Record<string, number> = {};
  private readonly escapedByEnemy: Record<string, number> = {};
  private readonly spawnedByEnemy: Record<string, number> = {};

  constructor(config: KernelConfig, accountHeroLevel = 1) {
    this.config = config;
    this.lanes = config.stage.lanes;
    this.gold = Math.round(config.stage.startingGold * config.balance.gold);
    this.lives = config.stage.lives;
    const first = this.lanes[0];
    const start = first[Math.min(2, first.length - 1)];
    const accountBonus = 1 + Math.max(0, accountHeroLevel - 1) * 0.025;
    const maxHp = config.hero.hp * accountBonus;
    this.heroDamage = config.hero.damage * accountBonus;
    this.hero = {
      x: start.x,
      y: start.y - 58,
      targetX: start.x,
      targetY: start.y - 58,
      level: 1,
      xp: 0,
      lastShot: 0,
      hp: maxHp,
      maxHp,
      deadUntil: 0,
      attackCount: 0,
    };
  }

  get ticks() {
    return this.tickCount;
  }

  get finished() {
    return this.completed;
  }

  get won() {
    return this.victory;
  }

  get time() {
    return this.simulationTime;
  }

  enemyView(): readonly KernelEnemyView[] {
    return this.enemies;
  }

  towerView(): readonly KernelTowerView[] {
    return this.towers;
  }

  heroView(): KernelHeroView {
    return this.hero;
  }

  reinforcementView(): readonly Reinforcement[] {
    return this.reinforcements;
  }

  drainEvents(): KernelEvent[] {
    return this.events.splice(0, this.events.length);
  }

  towerAt(spotId: string): KernelTowerView | undefined {
    return this.towers.find((tower) => tower.spotId === spotId);
  }

  status(): KernelStatus {
    const cooldowns: Record<string, number> = {};
    for (const skill of this.config.skills)
      cooldowns[skill.id] = Math.max(
        0,
        Math.ceil(((this.skillReady.get(skill.id) ?? 0) - this.simulationTime) / 1000),
      );
    return {
      status: this.completed
        ? this.victory
          ? "victory"
          : "defeat"
        : this.waveActive
          ? "playing"
          : "ready",
      gold: Math.max(0, this.gold),
      lives: this.lives,
      wave: this.waveIndex + (this.waveActive ? 1 : 0),
      totalWaves:
        this.config.stage.mode === "endless" ? 0 : this.config.stage.waves.length,
      kills: this.kills,
      heroLevel: this.hero.level,
      heroHp: Math.max(0, Math.round(this.hero.hp)),
      heroMaxHp: Math.max(0, Math.round(this.hero.maxHp)),
      heroAlive: this.hero.deadUntil === 0,
      heroRespawn: this.hero.deadUntil
        ? Math.max(0, Math.ceil((this.hero.deadUntil - this.simulationTime) / 1000))
        : 0,
      nextWaveIn: this.waveActive
        ? 0
        : Math.max(0, Math.ceil((this.nextWaveAt - this.simulationTime) / 1000)),
      skillCooldowns: cooldowns,
    };
  }

  outcome(): KernelOutcome {
    return {
      victory: this.victory,
      ticks: this.tickCount,
      duration_ms: this.tickCount * KERNEL_TICK_MS,
      lives: this.lives,
      gold: Math.max(0, this.gold),
      earned_gold: this.earnedGold,
      spent_gold: this.spentGold,
      sold_gold: this.soldGold,
      kills: this.kills,
      escaped: this.escaped,
      spawned: this.spawned,
      waves_completed: this.waveIndex,
      hero_level: this.hero.level,
      defeated_by_enemy: { ...this.defeatedByEnemy },
      escaped_by_enemy: { ...this.escapedByEnemy },
      spawned_by_enemy: { ...this.spawnedByEnemy },
    };
  }

  // ---------------------------------------------------------------- commands

  apply(command: KernelCommand) {
    if (this.completed) return;
    switch (command.op) {
      case "wave":
        this.startWave(true);
        break;
      case "build":
        this.buildTower(command.spot, command.tower, command.profile ?? "");
        break;
      case "upgrade":
        this.upgradeTower(command.spot, command.branch ?? "");
        break;
      case "sell":
        this.sellTower(command.spot);
        break;
      case "target":
        this.changeTargeting(command.spot, command.mode);
        break;
      case "skill":
        this.armSkill(command.skill);
        break;
      case "meteor":
        this.castMeteor(command.x, command.y);
        break;
      case "reinforce":
        this.castReinforcement(command.x, command.y);
        break;
      case "hero":
        this.moveHero(command.x, command.y);
        break;
      case "economy":
        this.adjustEconomy(command.gold, command.lives);
        break;
      case "defeat":
        this.endBattle(false);
        break;
    }
  }

  private buildTower(spotId: string, towerId: string, profile: string) {
    if (this.towers.some((tower) => tower.spotId === spotId)) return;
    const index = this.config.towers.findIndex((tower) => tower.id === towerId);
    const spot = this.config.stage.spots.find((item) => item.id === spotId);
    if (index < 0 || !spot) return;
    const definition = this.config.towers[index];
    if (this.gold < definition.cost) return;
    if (profile && !definition.profiles.some((item) => item.id === profile)) return;
    this.gold -= definition.cost;
    this.spentGold += definition.cost;
    const nearest = closestPointOnPaths(this.lanes, { x: spot.x, y: spot.y }).point;
    this.towers.push({
      spotId,
      def: index,
      x: spot.x,
      y: spot.y,
      level: 1,
      branch: "",
      profile,
      targeting: "first",
      lastShot: 0,
      disabledUntil: 0,
      soldiers: definition.blocking
        ? [
            { x: nearest.x - 18, y: nearest.y - 16 },
            { x: nearest.x + 18, y: nearest.y + 16 },
          ]
        : [],
      blocked: new Set(),
    });
    this.events.push({ k: "tower", spot: spotId, change: "build" });
    this.events.push({
      k: "telemetry",
      event: "realmguard.tower.build",
      data: { tower: towerId, spot: spotId, profile_id: profile || undefined },
    });
  }

  private upgradeTower(spotId: string, branch: string) {
    const tower = this.towers.find((item) => item.spotId === spotId);
    if (!tower || tower.level >= 3) return;
    const cost = this.config.balance.towerUpgradeCost[tower.level] ?? 100;
    if (this.gold < cost) return;
    this.gold -= cost;
    this.spentGold += cost;
    tower.level += 1;
    const definition = this.config.towers[tower.def];
    if (
      tower.level === 3 &&
      branch &&
      definition.branches.some((item) => item.id === branch)
    )
      tower.branch = branch;
    this.events.push({ k: "tower", spot: spotId, change: "upgrade" });
    this.events.push({
      k: "telemetry",
      event: "realmguard.tower.upgrade",
      data: {
        tower: definition.id,
        spot: spotId,
        level: tower.level,
        branch: tower.branch || undefined,
      },
    });
  }

  private sellTower(spotId: string) {
    const index = this.towers.findIndex((item) => item.spotId === spotId);
    if (index < 0) return;
    const tower = this.towers[index];
    const definition = this.config.towers[tower.def];
    const invested =
      definition.cost +
      this.config.balance.towerUpgradeCost
        .slice(1, tower.level)
        .reduce((sum, value) => sum + value, 0);
    const refund = Math.round(invested * this.config.balance.sellRefundRate);
    this.gold += refund;
    this.soldGold += refund;
    this.towers.splice(index, 1);
    this.events.push({ k: "tower", spot: spotId, change: "sell" });
    this.events.push({
      k: "telemetry",
      event: "realmguard.tower.sell",
      data: { spot: spotId },
    });
  }

  private changeTargeting(spotId: string, mode: TargetingMode) {
    if (!TARGETING.includes(mode)) return;
    const tower = this.towers.find((item) => item.spotId === spotId);
    if (!tower) return;
    tower.targeting = mode;
    this.events.push({
      k: "telemetry",
      event: "realmguard.tower.targeting",
      data: { tower: this.config.towers[tower.def].id, mode },
    });
  }

  private armSkill(skillId: string) {
    const skill = this.config.skills.find((item) => item.id === skillId);
    if (!skill || (this.skillReady.get(skillId) ?? 0) > this.simulationTime) return;
    this.skillReady.set(skillId, this.simulationTime + skill.cooldown * 1000);
    if (skillId === "freeze") {
      for (const enemy of this.enemies)
        if (!this.hasTrait(enemy, "immune_stun")) {
          enemy.slowUntil = this.simulationTime + 5000;
          enemy.slowFactor = 0.18;
        }
      this.events.push({ k: "freeze" });
      this.events.push({
        k: "telemetry",
        event: "realmguard.skill.cast",
        data: { skill: skillId },
      });
      return;
    }
    if (skillId === "meteor" || skillId === "reinforcement") this.armed = skillId;
  }

  private castMeteor(x: number, y: number) {
    if (this.armed !== "meteor") return;
    this.armed = "";
    const px = clamp(x, 0, KERNEL_WIDTH);
    const py = clamp(y, 0, KERNEL_HEIGHT);
    for (const enemy of this.enemies.slice())
      if (distance(px, py, enemy.x, enemy.y) < 125)
        this.damageEnemy(enemy, 245, "skill", 0);
    this.events.push({ k: "meteor", x: px, y: py });
    this.events.push({
      k: "telemetry",
      event: "realmguard.skill.cast",
      data: { skill: "meteor", x: Math.round(px), y: Math.round(py) },
    });
  }

  private castReinforcement(x: number, y: number) {
    if (this.armed !== "reinforcement") return;
    this.armed = "";
    const px = clamp(x, 0, KERNEL_WIDTH);
    const py = clamp(y, 0, KERNEL_HEIGHT);
    this.reinforcements.push({
      x: px,
      y: py,
      nextStrikeAt: this.simulationTime + 650,
      expiresAt: this.simulationTime + 8200,
    });
    this.events.push({ k: "reinforce", x: px, y: py });
    this.events.push({
      k: "telemetry",
      event: "realmguard.skill.cast",
      data: { skill: "reinforcement", x: Math.round(px), y: Math.round(py) },
    });
  }

  private moveHero(x: number, y: number) {
    this.hero.targetX = clamp(x, 35, KERNEL_WIDTH - 35);
    this.hero.targetY = clamp(y, 70, KERNEL_HEIGHT - 35);
    this.events.push({
      k: "telemetry",
      event: "realmguard.hero.move",
      data: { x: Math.round(x), y: Math.round(y) },
    });
  }

  private adjustEconomy(goldDelta: number, livesDelta: number) {
    const delta = Math.round(goldDelta);
    this.gold += delta;
    if (delta >= 0) this.earnedGold += delta;
    else this.spentGold += Math.abs(delta);
    this.lives = Math.max(0, this.lives + Math.round(livesDelta));
    if (this.lives <= 0) this.endBattle(false);
  }

  // -------------------------------------------------------------- simulation

  tick() {
    if (this.completed) return;
    this.tickCount += 1;
    this.simulationTime += KERNEL_TICK_MS;
    const time = this.simulationTime;
    if (!this.waveActive && time >= this.nextWaveAt) this.startWave(false);
    if (this.waveActive) this.processSpawnQueue(time);
    for (const enemy of this.enemies.slice()) {
      this.updateEnemy(enemy, time);
      if (this.completed) return;
    }
    for (const tower of this.towers.slice()) this.updateTower(tower, time);
    this.updateHero(time);
    this.updateReinforcements(time);
    this.updateStageGimmick(time);
    if (
      canCompleteWave(
        this.completed,
        this.waveActive,
        this.spawnQueue.length,
        this.enemies.length,
      )
    )
      this.completeWave();
  }

  private startWave(requestedEarly: boolean) {
    if (this.waveActive || this.completed) return;
    const stage = this.config.stage;
    if (stage.mode === "campaign" && this.waveIndex >= stage.waves.length) return;
    const baseWave = stage.waves[this.waveIndex % stage.waves.length];
    const cycle = Math.floor(this.waveIndex / stage.waves.length);
    this.spawnQueue = expandWave(baseWave.entries, cycle);
    this.waveActive = true;
    this.waveStartedAt = this.simulationTime;
    const secondsSaved = Math.max(
      0,
      Math.ceil((this.nextWaveAt - this.simulationTime) / 1000),
    );
    const earlyCall = requestedEarly && secondsSaved > 0;
    const earlyBonus = earlyCall ? secondsSaved * 3 : 0;
    this.gold += earlyBonus;
    this.earnedGold += earlyBonus;
    this.events.push({
      k: "telemetry",
      event: "realmguard.wave.start",
      data: {
        stage_id: stage.id,
        wave: this.waveIndex + 1,
        early_call: earlyCall,
        early_bonus: earlyBonus,
      },
    });
  }

  private processSpawnQueue(time: number) {
    const elapsed = time - this.waveStartedAt;
    while (this.spawnQueue[0] && this.spawnQueue[0].at <= elapsed)
      this.spawnEnemy(this.spawnQueue.shift()!);
  }

  private spawnEnemy(order: SpawnOrder): Enemy {
    let index = this.config.enemies.findIndex((item) => item.id === order.enemy);
    if (index < 0) index = 0;
    const definition = this.config.enemies[index];
    const endlessScale =
      1 +
      Math.floor(this.waveIndex / Math.max(1, this.config.stage.waves.length)) *
        this.config.balance.endlessRamp;
    const modifiers = new Set(order.modifiers);
    const maxHp = Math.round(
      definition.hp *
        this.config.balance.enemyHp *
        endlessScale *
        (modifiers.has("armored") ? 1.3 : 1),
    );
    const flying = definition.traits.includes("flying") || modifiers.has("flying");
    const lane = Math.max(0, Math.min(order.pathIndex, this.lanes.length - 1));
    const start = this.lanes[lane][0];
    this.enemySequence += 1;
    const enemy: Enemy = {
      id: this.enemySequence,
      def: index,
      hp: maxHp,
      maxHp,
      speed:
        definition.speed *
        this.config.balance.enemySpeed *
        (modifiers.has("swift") ? 1.24 : 1),
      x: start.x,
      y: start.y - (flying ? 20 : 0),
      lane,
      pathIndex: 1,
      pathProgress: 0,
      slowUntil: 0,
      slowFactor: 1,
      healAt: this.simulationTime + 2500,
      hasteUntil: 0,
      lastAttack: 0,
      modifiers,
      phases: new Set(),
      alive: true,
    };
    this.enemies.push(enemy);
    this.spawned += 1;
    this.spawnedByEnemy[definition.id] = (this.spawnedByEnemy[definition.id] ?? 0) + 1;
    this.events.push({ k: "spawn", id: enemy.id });
    return enemy;
  }

  private definitionOf(enemy: Enemy): KernelEnemy {
    return this.config.enemies[enemy.def];
  }

  private hasTrait(enemy: Enemy, trait: string) {
    return (
      this.definitionOf(enemy).traits.includes(trait) || enemy.modifiers.has(trait)
    );
  }

  private traitSet(enemy: Enemy) {
    return mergedTraits(
      { traits: this.definitionOf(enemy).traits as EnemyArchetype["traits"] },
      enemy.modifiers,
    );
  }

  private updateEnemy(enemy: Enemy, time: number) {
    if (!enemy.alive) return;
    const definition = this.definitionOf(enemy);
    if (this.hasTrait(enemy, "regenerating") && time >= enemy.healAt) {
      enemy.hp = Math.min(enemy.maxHp, enemy.hp + enemy.maxHp * 0.025);
      enemy.healAt = time + 1600;
      this.events.push({ k: "hp", id: enemy.id });
    }
    if (this.hasTrait(enemy, "healer") && time >= enemy.healAt) {
      for (const ally of this.enemies)
        if (ally.id !== enemy.id && distance(enemy.x, enemy.y, ally.x, ally.y) < 105) {
          ally.hp = Math.min(ally.maxHp, ally.hp + ally.maxHp * 0.06);
          this.events.push({ k: "hp", id: ally.id });
        }
      enemy.healAt = time + 2300;
      this.events.push({ k: "heal", x: enemy.x, y: enemy.y });
    }
    if (
      !this.hasTrait(enemy, "flying") &&
      !(this.hasTrait(enemy, "phasing") && Math.floor(time / 500) % 3 === 0)
    ) {
      const barracks = this.towers.find(
        (tower) =>
          this.config.towers[tower.def].blocking &&
          tower.disabledUntil <= time &&
          tower.soldiers.some(
            (soldier) =>
              distance(enemy.x, enemy.y, soldier.x, soldier.y) <=
              definition.radius + 22,
          ) &&
          (tower.blocked.has(enemy.id) || tower.blocked.size < tower.soldiers.length),
      );
      if (barracks) {
        const firstBlock = !barracks.blocked.has(enemy.id);
        barracks.blocked.add(enemy.id);
        if (firstBlock)
          this.events.push({
            k: "telemetry",
            event: "realmguard.barracks.block",
            data: {
              enemy: definition.id,
              tower: this.config.towers[barracks.def].id,
            },
          });
        if (time - enemy.lastAttack >= 850) {
          enemy.lastAttack = time;
          const soldier =
            barracks.soldiers[enemy.id % Math.max(1, barracks.soldiers.length)];
          if (soldier)
            this.events.push({
              k: "melee",
              spot: barracks.spotId,
              x: soldier.x,
              y: soldier.y,
            });
          if (this.hasTrait(enemy, "siege")) this.siegeDisrupt(enemy);
        }
        return;
      }
    }
    if (
      this.hero.deadUntil === 0 &&
      !this.hasTrait(enemy, "flying") &&
      distance(enemy.x, enemy.y, this.hero.x, this.hero.y) <= definition.radius + 28
    ) {
      if (time - enemy.lastAttack >= 850) {
        enemy.lastAttack = time;
        this.damageHero(Math.max(8, definition.lifeDamage * 11 + enemy.maxHp * 0.008));
        if (this.hasTrait(enemy, "siege")) this.siegeDisrupt(enemy);
      }
      return;
    }
    const path = this.lanes[enemy.lane];
    const point = path[enemy.pathIndex];
    if (!point) {
      this.enemyEscaped(enemy);
      return;
    }
    const speed =
      enemy.speed *
      movementMultiplier(
        this.traitSet(enemy),
        enemy.hp / enemy.maxHp,
        time < enemy.slowUntil,
        enemy.slowFactor,
        time < enemy.hasteUntil,
      );
    const displayYOffset = this.hasTrait(enemy, "flying") ? 20 : 0;
    const targetY = point.y - displayYOffset;
    const travel = distance(enemy.x, enemy.y, point.x, targetY);
    const step = speed * (KERNEL_TICK_MS / 1000);
    if (travel <= step) {
      enemy.x = point.x;
      enemy.y = targetY;
      enemy.pathIndex += 1;
    } else {
      enemy.x += ((point.x - enemy.x) / travel) * step;
      enemy.y += ((targetY - enemy.y) / travel) * step;
    }
    enemy.pathProgress = normalizedPathProgress(
      path,
      enemy.pathIndex,
      { x: enemy.x, y: enemy.y },
      displayYOffset,
    );
  }

  private enemyEscaped(enemy: Enemy) {
    const definition = this.definitionOf(enemy);
    this.lives = Math.max(0, this.lives - definition.lifeDamage);
    this.escaped += 1;
    this.escapedByEnemy[definition.id] = (this.escapedByEnemy[definition.id] ?? 0) + 1;
    this.removeEnemy(enemy, false);
    if (this.lives <= 0) this.endBattle(false);
  }

  private towerStats(tower: Tower) {
    const definition = this.config.towers[tower.def];
    const levelDamage = [1, 1.45, 2.05][tower.level - 1] ?? 1;
    const levelRange = [1, 1.08, 1.16][tower.level - 1] ?? 1;
    const branch = definition.branches.find((item) => item.id === tower.branch);
    return {
      damage: definition.damage * levelDamage * (branch ? branch.damageMultiplier : 1),
      range: definition.range * levelRange * (branch ? branch.rangeMultiplier : 1),
      fireRate: definition.fireRate * (branch ? branch.rateMultiplier : 1),
      splash:
        branch && branch.splash >= 0
          ? branch.splash
          : definition.damageType === "siege"
            ? 48
            : 0,
      slow:
        branch && branch.slow >= 0
          ? branch.slow
          : definition.id === "windward"
            ? 0.52
            : definition.damageType === "frost"
              ? 0.2
              : 0,
      pierce: branch ? branch.pierce : 0,
    };
  }

  private effectiveness(tower: KernelTower, enemy: KernelEnemy) {
    if (!enemy.threatType || !tower.effectiveAgainst.includes(enemy.threatType))
      return 1;
    return Math.max(1, tower.effectiveMultiplier < 0 ? 1.5 : tower.effectiveMultiplier);
  }

  private updateTower(tower: Tower, time: number) {
    const definition = this.config.towers[tower.def];
    const stats = this.towerStats(tower);
    if (tower.disabledUntil > time || time - tower.lastShot < stats.fireRate * 1000)
      return;
    const skyBranch = definition.branches[1]?.id;
    const targets = this.enemies.filter((enemy) => {
      const stealthRange =
        this.hasTrait(enemy, "stealth") && enemy.pathProgress < 0.72
          ? stats.range * 0.62
          : stats.range;
      return (
        enemy.alive &&
        distance(tower.x, tower.y, enemy.x, enemy.y) <= stealthRange &&
        (!definition.blocking ||
          (skyBranch !== undefined && tower.branch === skyBranch) ||
          !this.hasTrait(enemy, "flying"))
      );
    });
    if (!targets.length) return;
    targets.sort((a, b) =>
      targetComparator(
        tower.targeting,
        { x: tower.x, y: tower.y },
        { pathProgress: a.pathProgress, hp: a.hp, x: a.x, y: a.y },
        { pathProgress: b.pathProgress, hp: b.hp, x: b.x, y: b.y },
      ),
    );
    tower.lastShot = time;
    this.fireTower(tower, targets[0], stats);
  }

  private fireTower(
    tower: Tower,
    target: Enemy,
    stats: ReturnType<BattleKernel["towerStats"]>,
  ) {
    const definition = this.config.towers[tower.def];
    const targetX = target.x;
    const targetY = target.y;
    const profileMultiplier =
      definition.profiles.find((profile) => profile.id === tower.profile)
        ?.damageMultiplier ?? 1;
    this.damageEnemy(
      target,
      stats.damage *
        profileMultiplier *
        this.effectiveness(definition, this.definitionOf(target)),
      definition.damageType as DamageSource,
      stats.slow,
    );
    if (stats.splash)
      for (const enemy of this.enemies.slice())
        if (enemy.id !== target.id && distance(targetX, targetY, enemy.x, enemy.y) <= stats.splash)
          this.damageEnemy(
            enemy,
            stats.damage *
              profileMultiplier *
              0.48 *
              this.effectiveness(definition, this.definitionOf(enemy)),
            definition.damageType as DamageSource,
            stats.slow,
          );
    if (stats.pierce > 0) {
      const pierced = this.enemies
        .filter(
          (enemy) =>
            enemy.id !== target.id &&
            distance(tower.x, tower.y, enemy.x, enemy.y) <= stats.range,
        )
        .sort((a, b) => b.pathProgress - a.pathProgress)
        .slice(0, stats.pierce);
      for (const enemy of pierced)
        this.damageEnemy(
          enemy,
          stats.damage *
            profileMultiplier *
            0.62 *
            this.effectiveness(definition, this.definitionOf(enemy)),
          definition.damageType as DamageSource,
          stats.slow,
        );
    }
    if (definition.blocking)
      this.events.push({ k: "melee", spot: tower.spotId, x: targetX, y: targetY });
    else
      this.events.push({
        k: "shot",
        spot: tower.spotId,
        x: targetX,
        y: targetY,
        radius: Math.max(20, stats.splash),
      });
  }

  private damageEnemy(enemy: Enemy, amount: number, type: DamageSource, slow: number) {
    if (!enemy.alive) return;
    if (this.hasTrait(enemy, "phasing") && type !== "skill") {
      this.hitSequence += 1;
      if (this.hitSequence % 4 === 0) return;
    }
    const definition = this.definitionOf(enemy);
    enemy.hp -= effectiveDamage(amount, type, definition.armor, this.traitSet(enemy));
    if (slow && !this.hasTrait(enemy, "boss") && !this.hasTrait(enemy, "immune_stun")) {
      enemy.slowUntil = this.simulationTime + 1900;
      enemy.slowFactor = Math.max(0.35, 1 - slow);
    }
    this.events.push({ k: "hp", id: enemy.id });
    if (this.hasTrait(enemy, "boss")) this.handleBossPhases(enemy);
    if (enemy.hp <= 0) this.killEnemy(enemy);
  }

  private killEnemy(enemy: Enemy) {
    if (!enemy.alive) return;
    enemy.alive = false;
    const definition = this.definitionOf(enemy);
    this.gold += definition.reward;
    this.earnedGold += definition.reward;
    this.kills += 1;
    this.defeatedByEnemy[definition.id] = (this.defeatedByEnemy[definition.id] ?? 0) + 1;
    this.heroGainXp(1);
    if (this.hasTrait(enemy, "splitting"))
      for (let index = 0; index < 2; index += 1) this.spawnSplit(enemy);
    this.removeEnemy(enemy, true);
  }

  private spawnSplit(parent: Enemy) {
    let index = this.config.enemies.findIndex((item) => item.id === "mireling");
    if (index < 0) index = 0;
    const definition = this.config.enemies[index];
    this.enemySequence += 1;
    const id = this.enemySequence;
    this.enemies.push({
      id,
      def: index,
      hp: 22,
      maxHp: 22,
      speed: parent.speed * 1.2,
      x: parent.x + jitter(id * 2, 8),
      y: parent.y + jitter(id * 2 + 1, 8),
      lane: parent.lane,
      pathIndex: parent.pathIndex,
      pathProgress: parent.pathProgress,
      slowUntil: 0,
      slowFactor: 1,
      healAt: 0,
      hasteUntil: 0,
      lastAttack: 0,
      modifiers: new Set(),
      phases: new Set(),
      alive: true,
    });
    this.spawned += 1;
    this.spawnedByEnemy[definition.id] = (this.spawnedByEnemy[definition.id] ?? 0) + 1;
    this.events.push({ k: "spawn", id });
  }

  private removeEnemy(enemy: Enemy, killed: boolean) {
    enemy.alive = false;
    const index = this.enemies.indexOf(enemy);
    if (index >= 0) this.enemies.splice(index, 1);
    for (const tower of this.towers) tower.blocked.delete(enemy.id);
    this.events.push({ k: "despawn", id: enemy.id, killed });
  }

  private updateHero(time: number) {
    const hero = this.hero;
    if (hero.deadUntil > 0) {
      if (time < hero.deadUntil) return;
      hero.deadUntil = 0;
      hero.hp = hero.maxHp;
      const first = this.lanes[0];
      const start = first[Math.min(2, first.length - 1)];
      hero.x = start.x;
      hero.y = start.y - 58;
      hero.targetX = hero.x;
      hero.targetY = hero.y;
      this.events.push({ k: "hero-respawn" });
      this.events.push({
        k: "telemetry",
        event: "realmguard.hero.respawn",
        data: { hero_id: this.config.hero.id, level: hero.level },
      });
    }
    const travel = distance(hero.x, hero.y, hero.targetX, hero.targetY);
    if (travel > 4) {
      const step = Math.min(travel, (this.config.hero.speed * KERNEL_TICK_MS) / 1000);
      hero.x += ((hero.targetX - hero.x) / travel) * step;
      hero.y += ((hero.targetY - hero.y) / travel) * step;
    }
    if (time - hero.lastShot < Math.max(420, 920 - hero.level * 35)) return;
    const target = this.enemies
      .filter((enemy) => distance(hero.x, hero.y, enemy.x, enemy.y) <= this.config.hero.range)
      .sort((a, b) => b.pathProgress - a.pathProgress)[0];
    if (!target) return;
    hero.lastShot = time;
    hero.attackCount += 1;
    this.events.push({ k: "hero-shot", id: target.id });
    let damage = this.heroDamage * (1 + (hero.level - 1) * 0.12);
    const heroId = this.config.hero.id;
    if (hero.attackCount % 5 === 0) {
      damage *= 1.75;
      this.events.push({
        k: "telemetry",
        event: "realmguard.hero.skill",
        data: { hero_id: heroId, slot: 1 },
      });
    }
    this.damageEnemy(target, damage, "hero", heroId === "nyra" ? 0.25 : 0);
    if (hero.attackCount % 11 === 0) {
      for (const enemy of this.enemies.slice())
        if (enemy.id !== target.id && distance(target.x, target.y, enemy.x, enemy.y) < 82)
          this.damageEnemy(enemy, damage * 0.55, "hero", heroId === "brann" ? 0.7 : 0.2);
      this.events.push({ k: "hero-wide" });
      this.events.push({
        k: "telemetry",
        event: "realmguard.hero.skill",
        data: { hero_id: heroId, slot: 2 },
      });
    }
    if (hero.attackCount % 25 === 0) {
      for (const enemy of this.enemies.slice())
        this.damageEnemy(enemy, damage * 0.72, "hero", heroId === "nyra" ? 0.6 : 0.15);
      this.events.push({ k: "hero-ultimate" });
      this.events.push({
        k: "telemetry",
        event: "realmguard.hero.ultimate",
        data: { hero_id: heroId },
      });
    }
  }

  private damageHero(amount: number) {
    const hero = this.hero;
    if (hero.deadUntil > 0) return;
    hero.hp = Math.max(0, hero.hp - amount);
    this.events.push({ k: "hero-hit" });
    if (hero.hp > 0) return;
    hero.deadUntil = this.simulationTime + this.config.hero.respawnSeconds * 1000;
    this.events.push({
      k: "telemetry",
      event: "realmguard.hero.defeated",
      data: {
        hero_id: this.config.hero.id,
        respawn_seconds: this.config.hero.respawnSeconds,
      },
    });
  }

  private heroGainXp(amount: number) {
    const hero = this.hero;
    if (hero.level >= 10) return;
    hero.xp += amount;
    const required =
      this.config.balance.heroLevelXp[hero.level] ?? Number.POSITIVE_INFINITY;
    if (hero.xp >= required) {
      hero.level += 1;
      this.events.push({ k: "hero-level" });
      this.events.push({
        k: "telemetry",
        event: "realmguard.hero.level_up",
        data: { hero_id: this.config.hero.id, level: hero.level },
      });
    }
  }

  private siegeDisrupt(enemy: Enemy) {
    const tower = this.towers
      .filter((item) => item.disabledUntil <= this.simulationTime)
      .sort(
        (a, b) => distance(enemy.x, enemy.y, a.x, a.y) - distance(enemy.x, enemy.y, b.x, b.y),
      )[0];
    if (!tower) return;
    tower.disabledUntil = this.simulationTime + 2200;
    this.events.push({ k: "tower", spot: tower.spotId, change: "disable" });
    this.events.push({
      k: "telemetry",
      event: "realmguard.enemy.siege_disrupt",
      data: {
        enemy: this.definitionOf(enemy).id,
        tower: this.config.towers[tower.def].id,
      },
    });
  }

  private handleBossPhases(enemy: Enemy) {
    const definition = this.definitionOf(enemy);
    const ratio = enemy.hp / enemy.maxHp;
    if (ratio <= 0.66 && !enemy.phases.has("phase-2")) {
      enemy.phases.add("phase-2");
      if (definition.id === "hollow_king") {
        const tower = this.towers[this.waveIndex % Math.max(1, this.towers.length)];
        if (tower) {
          tower.disabledUntil = this.simulationTime + 5000;
          this.events.push({ k: "tower", spot: tower.spotId, change: "disable" });
        }
      } else this.spawnBossMinions(enemy, 3, "glintfox");
      this.events.push({ k: "boss-phase", id: enemy.id, phase: 2 });
      this.events.push({
        k: "telemetry",
        event: "realmguard.boss.phase",
        data: { boss: definition.id, phase: 2 },
      });
    }
    if (ratio <= 0.33 && !enemy.phases.has("phase-3")) {
      enemy.phases.add("phase-3");
      if (definition.id === "hollow_king") this.spawnBossMinions(enemy, 5, "veilrunner");
      else {
        enemy.speed *= 1.65;
        enemy.modifiers.add("swift");
      }
      this.events.push({ k: "boss-phase", id: enemy.id, phase: 3 });
      this.events.push({
        k: "telemetry",
        event: "realmguard.boss.phase",
        data: { boss: definition.id, phase: 3 },
      });
    }
  }

  private spawnBossMinions(boss: Enemy, count: number, enemyId: string) {
    for (let index = 0; index < count; index += 1) {
      const minion = this.spawnEnemy({
        enemy: enemyId,
        at: 0,
        pathIndex: boss.lane,
        modifiers: ["summoned"],
      });
      minion.pathIndex = boss.pathIndex;
      minion.pathProgress = boss.pathProgress;
      minion.x = boss.x + jitter(minion.id * 2, 18);
      minion.y = boss.y + jitter(minion.id * 2 + 1, 18);
    }
  }

  private updateStageGimmick(time: number) {
    const gimmick = this.config.stage.gimmick;
    if (!gimmick || time < this.nextGimmickAt) return;
    this.nextGimmickAt = time + 12_000;
    if (gimmick === "ember_vents") {
      const hotspots = [
        { x: 360, y: 350 },
        { x: 820, y: 280 },
      ];
      const point = hotspots[(this.waveIndex + this.kills) % hotspots.length];
      for (const enemy of this.enemies.slice())
        if (distance(point.x, point.y, enemy.x, enemy.y) < 115)
          this.damageEnemy(enemy, 58, "skill", 0);
      this.events.push({ k: "gimmick", kind: gimmick, x: point.x, y: point.y });
    } else if (gimmick === "winter_blessing") {
      for (const enemy of this.enemies) {
        enemy.slowUntil = time + 3500;
        enemy.slowFactor = 0.62;
      }
      this.events.push({ k: "gimmick", kind: gimmick, x: 0, y: 0 });
    } else {
      for (const enemy of this.enemies)
        if (!this.hasTrait(enemy, "boss"))
          enemy.hasteUntil = Math.max(enemy.hasteUntil, time + 3500);
      this.events.push({ k: "gimmick", kind: gimmick, x: 0, y: 0 });
    }
    this.events.push({
      k: "telemetry",
      event: "realmguard.stage.gimmick",
      data: { stage_id: this.config.stage.id, gimmick },
    });
  }

  private updateReinforcements(time: number) {
    for (const unit of this.reinforcements) {
      if (time >= unit.expiresAt) continue;
      while (time >= unit.nextStrikeAt) {
        unit.nextStrikeAt += 650;
        const enemy = this.enemies
          .filter(
            (item) =>
              !this.hasTrait(item, "flying") &&
              distance(unit.x, unit.y, item.x, item.y) < 70,
          )
          .sort((a, b) => b.pathProgress - a.pathProgress)[0];
        if (enemy) this.damageEnemy(enemy, 28, "hero", 0.35);
      }
    }
    this.reinforcements = this.reinforcements.filter((unit) => time < unit.expiresAt);
  }

  private completeWave() {
    const stage = this.config.stage;
    const wave = stage.waves[this.waveIndex % stage.waves.length];
    const completedWave = this.waveIndex + 1;
    this.gold += wave.reward;
    this.earnedGold += wave.reward;
    this.waveIndex += 1;
    this.waveActive = false;
    this.events.push({
      k: "telemetry",
      event: "realmguard.wave.complete",
      data: {
        stage_id: stage.id,
        wave: completedWave,
        lives: this.lives,
        kills: this.kills,
        escaped: this.escaped,
        spawned: this.spawned,
        gold: Math.max(0, this.gold),
        earned_gold: this.earnedGold,
        spent_gold: this.spentGold,
        sold_gold: this.soldGold,
        hero_level: this.hero.level,
        defeated_by_enemy: { ...this.defeatedByEnemy },
        escaped_by_enemy: { ...this.escapedByEnemy },
        spawned_by_enemy: { ...this.spawnedByEnemy },
      },
    });
    if (this.completed) return;
    if (
      canResolveCampaignVictory(
        this.completed,
        stage.mode,
        this.waveIndex,
        stage.waves.length,
      )
    )
      this.endBattle(true);
    else this.nextWaveAt = this.simulationTime + 10_000;
  }

  private endBattle(victory: boolean) {
    if (this.completed) return;
    this.completed = true;
    this.victory = victory;
    this.events.push({ k: "complete", victory });
  }
}

/**
 * Replays a ledger head-on. The browser records what the player did; the Go
 * verifier runs this same routine to decide what actually happened.
 */
export function replayBattle(
  config: KernelConfig,
  commands: KernelCommand[],
  ticks: number,
  accountHeroLevel = 1,
): KernelOutcome {
  const kernel = new BattleKernel(config, accountHeroLevel);
  let cursor = 0;
  for (let tick = 0; tick < ticks; tick += 1) {
    while (cursor < commands.length && commands[cursor].tick <= tick) {
      kernel.apply(commands[cursor]);
      cursor += 1;
    }
    kernel.drainEvents();
    if (kernel.finished) break;
    kernel.tick();
  }
  while (cursor < commands.length) {
    kernel.apply(commands[cursor]);
    cursor += 1;
  }
  kernel.drainEvents();
  return kernel.outcome();
}
