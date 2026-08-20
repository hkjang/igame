import Phaser from 'phaser';
import { advanceFixedSimulation, calculateLocalResult, calculateStartingGold, simulationCooldownReady } from './content';
import { calculateTowerStats, effectiveDamage, mergedTraits, movementMultiplier } from './systems/CombatMath';
import { targetComparator } from './systems/TargetSystem';
import { canCompleteWave, expandWave } from './systems/WaveSystem';
import type {
  BattleHUD, BattleStats, EnemyArchetype, HeroDefinition, RealmCommand, RealmDifficulty,
  RealmGuardConfig, RealmResult, RealmSceneController, RealmStage, TargetingMode, TowerDefinition,
} from './types';

interface SceneCallbacks {
  onHUD: (hud: BattleHUD) => void;
  onTelemetry: (event: string, data?: Record<string, unknown>) => void | Promise<void>;
  onComplete: (result: RealmResult) => void;
  onCompleteError: (result: RealmResult, error: Error) => void;
}

interface MountOptions extends SceneCallbacks {
  config: RealmGuardConfig;
  stage: RealmStage;
  difficulty: RealmDifficulty;
  hero: HeroDefinition;
  accountHeroLevel: number;
}

interface EnemyUnit {
  id: number;
  definition: EnemyArchetype;
  object: Phaser.GameObjects.Container;
  body: Phaser.GameObjects.Graphics;
  health: Phaser.GameObjects.Graphics;
  hp: number;
  maxHp: number;
  speed: number;
  pathIndex: number;
  pathProgress: number;
  path: RealmStage['path'];
  slowUntil: number;
  slowFactor: number;
  healAt: number;
  hasteUntil: number;
  lastAttack: number;
  modifiers: Set<string>;
  phases: Set<string>;
  alive: boolean;
}

interface TowerUnit {
  spotId: string;
  definition: TowerDefinition;
  object: Phaser.GameObjects.Container;
  level: number;
  branch?: string;
  targeting: TargetingMode;
  lastShot: number;
  disabledUntil: number;
  soldiers?: Phaser.GameObjects.Container[];
  blockedEnemies: Set<number>;
}

interface HeroUnit {
  definition: HeroDefinition;
  object: Phaser.GameObjects.Container;
  target: Phaser.Math.Vector2;
  level: number;
  xp: number;
  lastShot: number;
  hp: number;
  maxHp: number;
  deadUntil: number;
  attackCount: number;
}

interface SpawnOrder { enemy: string; at: number; pathIndex: number; modifiers: string[] }

interface ReinforcementUnit {
  object: Phaser.GameObjects.Container;
  point: Phaser.Math.Vector2;
  nextStrikeAt: number;
  expiresAt: number;
}

const WIDTH = 1280;
const HEIGHT = 720;
const TARGETING: TargetingMode[] = ['first', 'last', 'strong', 'weak', 'closest'];
const themeColors: Record<RealmStage['theme'], { ground: number; accent: number; path: number; fog: number }> = {
  verdant: { ground: 0x173d38, accent: 0x4ca66d, path: 0x756c55, fog: 0x0a2829 },
  ember: { ground: 0x422d2c, accent: 0xd27352, path: 0x786153, fog: 0x241b27 },
  frost: { ground: 0x244353, accent: 0x7ccbe1, path: 0x71858a, fog: 0x142a3b },
  void: { ground: 0x282440, accent: 0x9c72d6, path: 0x625d72, fog: 0x111225 },
};

class RealmGuardBattleScene extends Phaser.Scene {
  private readonly options: MountOptions;
  private readonly enemies = new Map<number, EnemyUnit>();
  private readonly towers = new Map<string, TowerUnit>();
  private readonly spots = new Map<string, Phaser.GameObjects.Container>();
  private readonly skillReady = new Map<string, number>();
  private hero?: HeroUnit;
  private gold = 0;
  private lives = 20;
  private kills = 0;
  private waveIndex = 0;
  private waveActive = false;
  private spawnQueue: SpawnOrder[] = [];
  private waveStartedAt = 0;
  private selectedSpot?: string;
  private pointerMode?: 'meteor' | 'reinforcement' | 'move-hero';
  private gameStatus: BattleHUD['status'] = 'ready';
  private battleSpeed: 1 | 2 = 1;
  private enemySequence = 0;
  private lastHUD = 0;
  private completed = false;
  private simulationTime = 0;
  private accumulator = 0;
  private activeWallMs = 0;
  private nextWaveAt = 10_000;
  private nextGimmickAt = 12_000;
  private earnedGold = 0;
  private spentGold = 0;
  private soldGold = 0;
  private escaped = 0;
  private spawned = 0;
  private defeatedByEnemy: Record<string, number> = {};
  private escapedByEnemy: Record<string, number> = {};
  private spawnedByEnemy: Record<string, number> = {};
  private hitSequence = 0;
  private reinforcements: ReinforcementUnit[] = [];

  constructor(options: MountOptions) {
    super({ key: `RealmGuard-${Date.now()}` });
    this.options = options;
  }

  create() {
    this.gold = calculateStartingGold(this.options.stage, this.options.config.balance, this.options.difficulty);
    this.lives = this.options.stage.lives;
    this.drawWorld();
    this.createTowerSpots();
    this.createHero();
    this.input.on('pointerdown', (pointer: Phaser.Input.Pointer, objects: Phaser.GameObjects.GameObject[]) => {
      if (objects.length || !this.pointerMode || this.completed) return;
      const point = new Phaser.Math.Vector2(pointer.worldX, pointer.worldY);
      if (this.pointerMode === 'move-hero') this.moveHero(point);
      if (this.pointerMode === 'meteor') this.castMeteor(point);
      if (this.pointerMode === 'reinforcement') this.castReinforcement(point);
      this.pointerMode = undefined;
      this.emitHUD(true);
    });
    this.emitHUD(true);
    this.options.onTelemetry('realmguard.battle.ready', { stage_id: this.options.stage.id, difficulty: this.options.difficulty, hero_id: this.options.hero.id });
  }

  update(_time: number, delta: number) {
    if (this.gameStatus === 'paused' || this.completed) return;
    this.activeWallMs += Math.min(delta, 250);
    const advance = advanceFixedSimulation(this.accumulator, delta, this.battleSpeed);
    this.accumulator = advance.remainder;
    for (let step = 0; step < advance.steps; step += 1) {
      this.simulationTime += 50;
      this.simulationTick(this.simulationTime, 50);
    }
  }

  private simulationTick(time: number, delta: number) {
    if (this.completed) return;
    if (!this.waveActive && this.gameStatus === 'ready' && time >= this.nextWaveAt) this.startWave(false);
    if (this.waveActive) this.processSpawnQueue(time);
    for (const enemy of [...this.enemies.values()]) {
      this.updateEnemy(enemy, time, delta);
      if (this.completed) return;
    }
    for (const tower of this.towers.values()) this.updateTower(tower, time);
    this.updateHero(time, delta);
    this.updateReinforcements(time);
    this.updateStageGimmick(time);
    if (canCompleteWave(this.completed, this.waveActive, this.spawnQueue.length, this.enemies.size)) this.completeWave();
    if (time - this.lastHUD > 180) this.emitHUD();
  }

  command(command: RealmCommand) {
    if (this.completed && command.type !== 'toggle-pause') return;
    switch (command.type) {
      case 'start-wave': this.startWave(true); break;
      case 'toggle-pause': this.togglePause(); break;
      case 'speed':
        this.battleSpeed = command.value;
        this.tweens.timeScale = command.value;
        this.time.timeScale = command.value;
        this.emitHUD(true);
        break;
      case 'build': this.buildTower(command.tower); break;
      case 'upgrade': this.upgradeTower(command.branch); break;
      case 'sell': this.sellTower(); break;
      case 'targeting': this.changeTargeting(command.mode); break;
      case 'skill': this.armSkill(command.skill); break;
      case 'move-hero': this.pointerMode = 'move-hero'; this.options.onTelemetry('realmguard.hero.move_armed'); break;
    }
  }

  private drawWorld() {
    const colors = themeColors[this.options.stage.theme];
    const graphics = this.add.graphics();
    graphics.fillStyle(colors.ground).fillRect(0, 0, WIDTH, HEIGHT);
    graphics.fillStyle(colors.fog, .35).fillCircle(80, 80, 180).fillCircle(1190, 650, 260).fillCircle(660, 40, 140);
    for (let index = 0; index < 42; index += 1) {
      const x = (index * 197 + this.options.stage.number * 61) % WIDTH;
      const y = (index * 113 + this.options.stage.number * 97) % HEIGHT;
      graphics.fillStyle(colors.accent, .18 + (index % 3) * .05).fillCircle(x, y, 4 + index % 8);
    }
    const lanes = this.options.stage.paths?.length ? this.options.stage.paths : [this.options.stage.path];
    for (const lane of lanes) {
      const path = lane.map((point) => new Phaser.Math.Vector2(point.x, point.y));
      graphics.lineStyle(68, 0x111827, .3).strokePoints(path, false, false);
      graphics.lineStyle(56, colors.path, 1).strokePoints(path, false, false);
      graphics.lineStyle(3, 0xe8dbb4, .2).strokePoints(path, false, false);
    }
    const gate = lanes[0].at(-1)!;
    graphics.fillStyle(0x63d5bd, .24).fillCircle(gate.x - 15, gate.y, 42);
    graphics.lineStyle(5, 0x8fffe8, .8).strokeCircle(gate.x - 15, gate.y, 31);
    this.add.text(20, 675, `${this.options.stage.name} · ${this.options.stage.version}`, { fontFamily: 'system-ui', fontSize: '17px', color: '#d8e5ed', backgroundColor: '#09131dbb', padding: { x: 10, y: 6 } }).setDepth(20);
  }

  private createTowerSpots() {
    for (const spot of this.options.stage.towerSpots) {
      const ring = this.add.graphics();
      ring.fillStyle(0x0a1623, .78).fillCircle(0, 0, 29);
      ring.lineStyle(3, 0x8bd8c5, .72).strokeCircle(0, 0, 27);
      ring.lineStyle(1, 0xffffff, .16).strokeCircle(0, 0, 20);
      const plus = this.add.text(0, -2, '+', { fontFamily: 'system-ui', fontSize: '30px', color: '#a9e7d6', fontStyle: 'bold' }).setOrigin(.5);
      const container = this.add.container(spot.x, spot.y, [ring, plus]).setSize(62, 62).setInteractive({ useHandCursor: true }).setDepth(4);
      container.on('pointerdown', (pointer: Phaser.Input.Pointer) => {
        pointer.event.stopPropagation();
        this.selectedSpot = spot.id;
        this.redrawSpotSelection();
        this.emitHUD(true);
      });
      this.spots.set(spot.id, container);
    }
  }

  private redrawSpotSelection() {
    for (const [id, spot] of this.spots) spot.setScale(id === this.selectedSpot ? 1.14 : 1);
  }

  private createHero() {
    const start = this.options.stage.path[Math.min(2, this.options.stage.path.length - 1)];
    const aura = this.add.graphics().fillStyle(this.options.hero.color, .18).fillCircle(0, 0, 30).lineStyle(2, this.options.hero.color, .75).strokeCircle(0, 0, 23);
    const body = this.add.graphics().fillStyle(this.options.hero.color).fillCircle(0, 0, 14).fillStyle(0xffffff, .85).fillTriangle(-7, -4, 7, -4, 0, -14);
    const label = this.add.text(0, 25, this.options.hero.name, { fontFamily: 'system-ui', fontSize: '16px', color: '#ffffff', backgroundColor: '#07101dcc', padding: { x: 5, y: 2 } }).setOrigin(.5, 0);
    const object = this.add.container(start.x, start.y - 58, [aura, body, label]).setDepth(9);
    const accountBonus = 1 + Math.max(0, this.options.accountHeroLevel - 1) * .025;
    const maxHp = this.options.hero.hp * accountBonus;
    this.hero = { definition: { ...this.options.hero, damage: this.options.hero.damage * accountBonus }, object, target: new Phaser.Math.Vector2(object.x, object.y), level: 1, xp: 0, lastShot: 0, hp: maxHp, maxHp, deadUntil: 0, attackCount: 0 };
  }

  private startWave(requestedEarly: boolean) {
    if (this.waveActive || this.completed) return;
    const stage = this.options.stage;
    if (stage.mode === 'campaign' && this.waveIndex >= stage.waves.length) return;
    const baseWave = stage.waves[this.waveIndex % stage.waves.length];
    const cycle = Math.floor(this.waveIndex / stage.waves.length);
    this.spawnQueue = expandWave(baseWave.entries, cycle);
    this.waveActive = true;
    this.gameStatus = 'playing';
    this.waveStartedAt = this.simulationTime;
    const secondsSaved = Math.max(0, Math.ceil((this.nextWaveAt - this.simulationTime) / 1000));
    const earlyCall = requestedEarly && secondsSaved > 0;
    const earlyBonus = earlyCall ? secondsSaved * 3 : 0;
    this.gold += earlyBonus;
    this.earnedGold += earlyBonus;
    this.options.onTelemetry('realmguard.wave.start', { stage_id: stage.id, wave: this.waveIndex + 1, early_call: earlyCall, early_bonus: earlyBonus });
    this.emitHUD(true);
  }

  private processSpawnQueue(time: number) {
    const elapsed = time - this.waveStartedAt;
    while (this.spawnQueue[0] && this.spawnQueue[0].at <= elapsed) this.spawnEnemy(this.spawnQueue.shift()!);
  }

  private spawnEnemy(order: SpawnOrder) {
    const definition = this.options.config.enemies.find((item) => item.id === order.enemy) ?? this.options.config.enemies[0];
    const difficulty = this.options.config.balance.difficulties[this.options.difficulty];
    const endlessScale = 1 + Math.floor(this.waveIndex / Math.max(1, this.options.stage.waves.length)) * this.options.config.balance.endlessRamp;
    const modifiers = new Set(order.modifiers);
    const maxHp = Math.round(definition.hp * difficulty.enemyHp * endlessScale * (modifiers.has('armored') ? 1.3 : 1));
    const body = this.add.graphics();
    const flying = definition.traits.includes('flying') || modifiers.has('flying');
    body.fillStyle(0x07111e, .5).fillCircle(3, 4, definition.radius + 3);
    body.fillStyle(definition.color).fillCircle(0, 0, definition.radius);
    body.lineStyle(definition.traits.includes('boss') ? 5 : 2, definition.traits.includes('boss') ? 0xffd166 : 0xffffff, .65).strokeCircle(0, 0, definition.radius);
    if (flying) body.lineStyle(3, 0xcaf3ff, .8).strokeEllipse(0, 0, definition.radius * 3, definition.radius);
    if (definition.traits.includes('armored')) body.fillStyle(0xd6d4c8, .7).fillTriangle(-8, -4, 0, -definition.radius - 4, 8, -4);
    const health = this.add.graphics();
    const label = definition.traits.includes('boss') ? this.add.text(0, definition.radius + 13, definition.name, { fontFamily: 'system-ui', fontSize: '16px', color: '#ffe29b', backgroundColor: '#111827cc', padding: { x: 4, y: 2 } }).setOrigin(.5, 0) : undefined;
    const lanes = this.options.stage.paths?.length ? this.options.stage.paths : [this.options.stage.path];
    const path = lanes[Math.min(order.pathIndex, lanes.length - 1)] ?? lanes[0];
    const start = path[0];
    const object = this.add.container(start.x, start.y - (flying ? 20 : 0), label ? [body, health, label] : [body, health]).setDepth(flying ? 8 : 6);
    const enemy: EnemyUnit = {
      id: ++this.enemySequence, definition, object, body, health, hp: maxHp, maxHp,
      speed: definition.speed * difficulty.enemySpeed * (modifiers.has('swift') ? 1.24 : 1), pathIndex: 1, pathProgress: 0, path,
      slowUntil: 0, slowFactor: 1, healAt: this.simulationTime + 2500, hasteUntil: 0, lastAttack: 0, modifiers, phases: new Set(), alive: true,
    };
    this.enemies.set(enemy.id, enemy);
    this.spawned += 1;
    this.spawnedByEnemy[definition.id] = (this.spawnedByEnemy[definition.id] ?? 0) + 1;
    this.drawEnemyHealth(enemy);
    return enemy;
  }

  private updateEnemy(enemy: EnemyUnit, time: number, delta: number) {
    if (!enemy.alive) return;
    if (this.hasTrait(enemy, 'regenerating') && time >= enemy.healAt) {
      enemy.hp = Math.min(enemy.maxHp, enemy.hp + enemy.maxHp * .025);
      enemy.healAt = time + 1600;
      this.drawEnemyHealth(enemy);
    }
    if (this.hasTrait(enemy, 'healer') && time >= enemy.healAt) {
      for (const ally of this.enemies.values()) if (ally.id !== enemy.id && Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, ally.object.x, ally.object.y) < 105) { ally.hp = Math.min(ally.maxHp, ally.hp + ally.maxHp * .06); this.drawEnemyHealth(ally); }
      enemy.healAt = time + 2300;
      this.pulse(enemy.object.x, enemy.object.y, 0x8ff0bd, 65);
    }
    const hero = this.hero;
    if (!this.hasTrait(enemy, 'flying') && !(this.hasTrait(enemy, 'phasing') && Math.floor(time / 500) % 3 === 0)) {
      const barracks = [...this.towers.values()].find((tower) => tower.definition.id === 'windward' && tower.disabledUntil <= time && tower.soldiers?.some((soldier) => Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, soldier.x, soldier.y) <= enemy.definition.radius + 22) && (tower.blockedEnemies.has(enemy.id) || tower.blockedEnemies.size < (tower.soldiers?.length ?? 0)));
      if (barracks) {
        const firstBlock = !barracks.blockedEnemies.has(enemy.id);
        barracks.blockedEnemies.add(enemy.id);
        if (firstBlock) this.options.onTelemetry('realmguard.barracks.block', { enemy: enemy.definition.id, tower: barracks.definition.id });
        if (time - enemy.lastAttack >= 850) {
          enemy.lastAttack = time;
          const soldier = barracks.soldiers?.[enemy.id % Math.max(1, barracks.soldiers.length)];
          if (soldier) this.pulse(soldier.x, soldier.y, barracks.definition.color, 24);
          if (this.hasTrait(enemy, 'siege')) this.siegeDisrupt(enemy);
        }
        return;
      }
    }
    if (hero && hero.deadUntil === 0 && !this.hasTrait(enemy, 'flying') && Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, hero.object.x, hero.object.y) <= enemy.definition.radius + 28) {
      if (time - enemy.lastAttack >= 850) {
        enemy.lastAttack = time;
        this.damageHero(Math.max(8, enemy.definition.lifeDamage * 11 + enemy.maxHp * .008));
        if (this.hasTrait(enemy, 'siege')) this.siegeDisrupt(enemy);
      }
      return;
    }
    const point = enemy.path[enemy.pathIndex];
    if (!point) { this.enemyEscaped(enemy); return; }
    const speed = enemy.speed * movementMultiplier(mergedTraits(enemy.definition, enemy.modifiers), enemy.hp / enemy.maxHp, time < enemy.slowUntil, enemy.slowFactor, time < enemy.hasteUntil);
    const distance = Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, point.x, point.y);
    const step = speed * (delta / 1000);
    if (distance <= step) {
      enemy.object.setPosition(point.x, point.y - (this.hasTrait(enemy, 'flying') ? 20 : 0));
      enemy.pathIndex += 1;
      enemy.pathProgress = enemy.pathIndex;
    } else {
      const angle = Phaser.Math.Angle.Between(enemy.object.x, enemy.object.y, point.x, point.y);
      enemy.object.x += Math.cos(angle) * step;
      enemy.object.y += Math.sin(angle) * step;
      enemy.pathProgress = enemy.pathIndex - 1 + (1 - distance / Math.max(1, Phaser.Math.Distance.Between(enemy.path[enemy.pathIndex - 1].x, enemy.path[enemy.pathIndex - 1].y, point.x, point.y)));
    }
  }

  private hasTrait(enemy: EnemyUnit, trait: string) {
    return enemy.definition.traits.includes(trait as EnemyArchetype['traits'][number]) || enemy.modifiers.has(trait);
  }

  private enemyEscaped(enemy: EnemyUnit) {
    this.lives = Math.max(0, this.lives - enemy.definition.lifeDamage);
    this.escaped += 1;
    this.escapedByEnemy[enemy.definition.id] = (this.escapedByEnemy[enemy.definition.id] ?? 0) + 1;
    this.removeEnemy(enemy);
    if (this.lives <= 0) void this.endBattle(false);
  }

  private updateTower(tower: TowerUnit, time: number) {
    if (tower.disabledUntil <= time && tower.object.alpha < 1) {
      tower.object.setAlpha(1);
      tower.soldiers?.forEach((soldier) => soldier.active && soldier.setAlpha(1));
    }
    const stats = calculateTowerStats(tower.definition, tower.level, tower.branch);
    if (tower.disabledUntil > time || !simulationCooldownReady(time, tower.lastShot, stats.fireRate * 1000)) return;
    const targets = [...this.enemies.values()].filter((enemy) => {
      const stealthRange = this.hasTrait(enemy, 'stealth') && enemy.pathProgress < enemy.path.length - 2 ? stats.range * .62 : stats.range;
      return enemy.alive && Phaser.Math.Distance.Between(tower.object.x, tower.object.y, enemy.object.x, enemy.object.y) <= stealthRange && (tower.definition.id !== 'windward' || tower.branch === 'skyrider_watch' || !this.hasTrait(enemy, 'flying'));
    });
    if (!targets.length) return;
    targets.sort((a, b) => targetComparator(tower.targeting, tower.object, { pathProgress: a.pathProgress, hp: a.hp, x: a.object.x, y: a.object.y }, { pathProgress: b.pathProgress, hp: b.hp, x: b.object.x, y: b.object.y }));
    tower.lastShot = time;
    this.fireProjectile(tower, targets[0], stats);
  }

  private fireProjectile(tower: TowerUnit, target: EnemyUnit, stats: ReturnType<typeof calculateTowerStats>) {
    const targetX = target.object.x;
    const targetY = target.object.y;
    this.damageEnemy(target, stats.damage, tower.definition.damageType, stats.slow);
    if (stats.splash) for (const enemy of [...this.enemies.values()]) if (enemy.id !== target.id && Phaser.Math.Distance.Between(targetX, targetY, enemy.object.x, enemy.object.y) <= stats.splash) this.damageEnemy(enemy, stats.damage * .48, tower.definition.damageType, stats.slow);
    if (stats.pierce > 0) {
      [...this.enemies.values()]
        .filter((enemy) => enemy.id !== target.id && Phaser.Math.Distance.Between(tower.object.x, tower.object.y, enemy.object.x, enemy.object.y) <= stats.range)
        .sort((a, b) => b.pathProgress - a.pathProgress)
        .slice(0, stats.pierce)
        .forEach((enemy) => this.damageEnemy(enemy, stats.damage * .62, tower.definition.damageType, stats.slow));
    }
    this.pulse(targetX, targetY, tower.definition.color, Math.max(20, stats.splash));
    if (tower.definition.id === 'windward') {
      const soldier = tower.soldiers?.[Math.floor(tower.lastShot / 50) % Math.max(1, tower.soldiers.length)];
      if (soldier) this.tweens.add({ targets: soldier, x: Phaser.Math.Linear(soldier.x, targetX, .42), y: Phaser.Math.Linear(soldier.y, targetY, .42), duration: 120, yoyo: true });
      return;
    }
    const projectile = this.add.circle(tower.object.x, tower.object.y, tower.definition.damageType === 'siege' ? 7 : 4, tower.definition.color).setDepth(12);
    this.tweens.add({
      targets: projectile, x: targetX, y: targetY, duration: Math.max(80, Phaser.Math.Distance.Between(projectile.x, projectile.y, targetX, targetY) / tower.definition.projectileSpeed * 1000),
      onComplete: () => projectile.destroy(),
    });
  }

  private damageEnemy(enemy: EnemyUnit, amount: number, type: TowerDefinition['damageType'] | 'hero' | 'skill', slow = 0) {
    if (!enemy.alive) return;
    if (this.hasTrait(enemy, 'phasing') && type !== 'skill' && ++this.hitSequence % 4 === 0) { this.pulse(enemy.object.x, enemy.object.y, 0xa78bfa, 34); return; }
    enemy.hp -= effectiveDamage(amount, type, enemy.definition.armor, mergedTraits(enemy.definition, enemy.modifiers));
    if (slow && !this.hasTrait(enemy, 'boss') && !this.hasTrait(enemy, 'immune_stun')) { enemy.slowUntil = this.simulationTime + 1900; enemy.slowFactor = Math.max(.35, 1 - slow); }
    this.drawEnemyHealth(enemy);
    if (this.hasTrait(enemy, 'boss')) this.handleBossPhases(enemy);
    if (enemy.hp <= 0) this.killEnemy(enemy);
  }

  private killEnemy(enemy: EnemyUnit) {
    if (!enemy.alive) return;
    enemy.alive = false;
    this.gold += enemy.definition.reward;
    this.earnedGold += enemy.definition.reward;
    this.kills += 1;
    this.defeatedByEnemy[enemy.definition.id] = (this.defeatedByEnemy[enemy.definition.id] ?? 0) + 1;
    this.heroGainXp(1);
    if (this.hasTrait(enemy, 'splitting')) for (let index = 0; index < 2; index += 1) this.spawnSplit(enemy);
    this.burst(enemy.object.x, enemy.object.y, enemy.definition.color);
    this.removeEnemy(enemy);
  }

  private spawnSplit(parent: EnemyUnit) {
    const definition = this.options.config.enemies.find((item) => item.id === 'mireling')!;
    const body = this.add.graphics().fillStyle(definition.color).fillCircle(0, 0, 8);
    const health = this.add.graphics();
    const object = this.add.container(parent.object.x + Phaser.Math.Between(-8, 8), parent.object.y + Phaser.Math.Between(-8, 8), [body, health]).setDepth(6);
    const enemy: EnemyUnit = { id: ++this.enemySequence, definition, object, body, health, hp: 22, maxHp: 22, speed: parent.speed * 1.2, pathIndex: parent.pathIndex, pathProgress: parent.pathProgress, path: parent.path, slowUntil: 0, slowFactor: 1, healAt: 0, hasteUntil: 0, lastAttack: 0, modifiers: new Set(), phases: new Set(), alive: true };
    this.enemies.set(enemy.id, enemy);
    this.spawned += 1;
    this.spawnedByEnemy[definition.id] = (this.spawnedByEnemy[definition.id] ?? 0) + 1;
    this.drawEnemyHealth(enemy);
  }

  private removeEnemy(enemy: EnemyUnit) {
    enemy.alive = false;
    this.enemies.delete(enemy.id);
    for (const tower of this.towers.values()) tower.blockedEnemies.delete(enemy.id);
    enemy.object.destroy(true);
  }

  private drawEnemyHealth(enemy: EnemyUnit) {
    enemy.health.clear();
    enemy.health.fillStyle(0x07101d, .85).fillRect(-17, -enemy.definition.radius - 11, 34, 5);
    enemy.health.fillStyle(enemy.hp / enemy.maxHp > .35 ? 0x65e392 : 0xff6f72).fillRect(-16, -enemy.definition.radius - 10, 32 * Math.max(0, enemy.hp / enemy.maxHp), 3);
  }

  private updateHero(time: number, delta: number) {
    const hero = this.hero;
    if (!hero) return;
    if (hero.deadUntil > 0) {
      if (time < hero.deadUntil) return;
      hero.deadUntil = 0;
      hero.hp = hero.maxHp;
      hero.object.setVisible(true).setAlpha(1);
      const start = this.options.stage.path[Math.min(2, this.options.stage.path.length - 1)];
      hero.object.setPosition(start.x, start.y - 58);
      hero.target.set(hero.object.x, hero.object.y);
      this.pulse(hero.object.x, hero.object.y, hero.definition.color, 70);
      this.options.onTelemetry('realmguard.hero.respawn', { hero_id: hero.definition.id, level: hero.level });
    }
    const distance = Phaser.Math.Distance.Between(hero.object.x, hero.object.y, hero.target.x, hero.target.y);
    if (distance > 4) {
      const step = Math.min(distance, hero.definition.speed * delta / 1000);
      const angle = Phaser.Math.Angle.Between(hero.object.x, hero.object.y, hero.target.x, hero.target.y);
      hero.object.x += Math.cos(angle) * step; hero.object.y += Math.sin(angle) * step;
    }
    if (!simulationCooldownReady(time, hero.lastShot, Math.max(420, 920 - hero.level * 35))) return;
    const target = [...this.enemies.values()].filter((enemy) => Phaser.Math.Distance.Between(hero.object.x, hero.object.y, enemy.object.x, enemy.object.y) <= hero.definition.range).sort((a, b) => b.pathProgress - a.pathProgress)[0];
    if (!target) return;
    hero.lastShot = time;
    hero.attackCount += 1;
    let damage = hero.definition.damage * (1 + (hero.level - 1) * .12);
    if (hero.attackCount % 5 === 0) {
      damage *= 1.75;
      this.options.onTelemetry('realmguard.hero.skill', { hero_id: hero.definition.id, skill: hero.definition.skill1 });
    }
    this.damageEnemy(target, damage, 'hero', hero.definition.id === 'nyra' ? .25 : 0);
    if (hero.attackCount % 11 === 0) {
      for (const enemy of [...this.enemies.values()]) if (enemy.id !== target.id && Phaser.Math.Distance.Between(target.object.x, target.object.y, enemy.object.x, enemy.object.y) < 82) this.damageEnemy(enemy, damage * .55, 'hero', hero.definition.id === 'brann' ? .7 : .2);
      this.options.onTelemetry('realmguard.hero.skill', { hero_id: hero.definition.id, skill: hero.definition.skill2 });
    }
    if (hero.attackCount % 25 === 0) {
      for (const enemy of [...this.enemies.values()]) this.damageEnemy(enemy, damage * .72, 'hero', hero.definition.id === 'nyra' ? .6 : .15);
      this.cameras.main.flash(180, 255, 224, 125, false);
      this.options.onTelemetry('realmguard.hero.ultimate', { hero_id: hero.definition.id, skill: hero.definition.ultimate });
    }
    this.pulse(target.object.x, target.object.y, hero.definition.color, 24);
  }

  private damageHero(amount: number) {
    const hero = this.hero;
    if (!hero || hero.deadUntil > 0) return;
    hero.hp = Math.max(0, hero.hp - amount);
    this.pulse(hero.object.x, hero.object.y, 0xff6f72, 30);
    if (hero.hp > 0) return;
    hero.deadUntil = this.simulationTime + hero.definition.respawnSeconds * 1000;
    hero.object.setVisible(false);
    this.options.onTelemetry('realmguard.hero.defeated', { hero_id: hero.definition.id, respawn_seconds: hero.definition.respawnSeconds });
    this.emitHUD(true);
  }

  private siegeDisrupt(enemy: EnemyUnit) {
    const tower = [...this.towers.values()]
      .filter((item) => item.disabledUntil <= this.simulationTime)
      .sort((a, b) => Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, a.object.x, a.object.y) - Phaser.Math.Distance.Between(enemy.object.x, enemy.object.y, b.object.x, b.object.y))[0];
    if (!tower) return;
    tower.disabledUntil = this.simulationTime + 2200;
    tower.object.setAlpha(.35);
    tower.soldiers?.forEach((soldier) => soldier.setAlpha(.35));
    this.options.onTelemetry('realmguard.enemy.siege_disrupt', { enemy: enemy.definition.id, tower: tower.definition.id });
  }

  private handleBossPhases(enemy: EnemyUnit) {
    const ratio = enemy.hp / enemy.maxHp;
    if (ratio <= .66 && !enemy.phases.has('phase-2')) {
      enemy.phases.add('phase-2');
      if (enemy.definition.id === 'hollow_king') {
        const tower = [...this.towers.values()][this.waveIndex % Math.max(1, this.towers.size)];
        if (tower) {
          tower.disabledUntil = this.simulationTime + 5000;
          tower.object.setAlpha(.3);
          tower.soldiers?.forEach((soldier) => soldier.setAlpha(.3));
        }
      } else this.spawnBossMinions(enemy, 3, 'glintfox');
      this.pulse(enemy.object.x, enemy.object.y, enemy.definition.color, 120);
      this.options.onTelemetry('realmguard.boss.phase', { boss: enemy.definition.id, phase: 2 });
    }
    if (ratio <= .33 && !enemy.phases.has('phase-3')) {
      enemy.phases.add('phase-3');
      if (enemy.definition.id === 'hollow_king') this.spawnBossMinions(enemy, 5, 'veilrunner');
      else { enemy.speed *= 1.65; enemy.modifiers.add('swift'); }
      this.cameras.main.shake(260, .006);
      this.options.onTelemetry('realmguard.boss.phase', { boss: enemy.definition.id, phase: 3 });
    }
  }

  private spawnBossMinions(boss: EnemyUnit, count: number, enemyId: string) {
    for (let index = 0; index < count; index += 1) {
      const lanes = this.options.stage.paths?.length ? this.options.stage.paths : [this.options.stage.path];
      const pathIndex = Math.max(0, lanes.indexOf(boss.path));
      const minion = this.spawnEnemy({ enemy: enemyId, at: 0, pathIndex, modifiers: ['summoned'] });
      minion.pathIndex = boss.pathIndex;
      minion.pathProgress = boss.pathProgress;
      minion.object.setPosition(boss.object.x + Phaser.Math.Between(-18, 18), boss.object.y + Phaser.Math.Between(-18, 18));
    }
  }

  private updateStageGimmick(time: number) {
    if (!this.options.stage.gimmick || time < this.nextGimmickAt) return;
    this.nextGimmickAt = time + 12_000;
    if (this.options.stage.gimmick === 'ember_vents') {
      const hotspots = [{ x: 360, y: 350 }, { x: 820, y: 280 }];
      const point = hotspots[(this.waveIndex + this.kills) % hotspots.length];
      for (const enemy of [...this.enemies.values()]) if (Phaser.Math.Distance.Between(point.x, point.y, enemy.object.x, enemy.object.y) < 115) this.damageEnemy(enemy, 58, 'skill');
      this.pulse(point.x, point.y, 0xff7c55, 120);
    } else if (this.options.stage.gimmick === 'winter_blessing') {
      for (const enemy of this.enemies.values()) { enemy.slowUntil = time + 3500; enemy.slowFactor = .62; }
      this.cameras.main.flash(140, 130, 220, 255, false);
    } else {
      for (const enemy of this.enemies.values()) if (!this.hasTrait(enemy, 'boss')) enemy.hasteUntil = Math.max(enemy.hasteUntil, time + 3500);
      this.cameras.main.shake(140, .003);
    }
    this.options.onTelemetry('realmguard.stage.gimmick', { stage_id: this.options.stage.id, gimmick: this.options.stage.gimmick });
  }

  private heroGainXp(amount: number) {
    if (!this.hero || this.hero.level >= 10) return;
    this.hero.xp += amount;
    const required = this.options.config.balance.heroLevelXp[this.hero.level] ?? Number.POSITIVE_INFINITY;
    if (this.hero.xp >= required) {
      this.hero.level += 1;
      this.pulse(this.hero.object.x, this.hero.object.y, 0xffdf72, 56);
      this.options.onTelemetry('realmguard.hero.level_up', { hero_id: this.hero.definition.id, level: this.hero.level });
    }
  }

  private moveHero(point: Phaser.Math.Vector2) {
    if (!this.hero) return;
    this.hero.target = new Phaser.Math.Vector2(Phaser.Math.Clamp(point.x, 35, WIDTH - 35), Phaser.Math.Clamp(point.y, 70, HEIGHT - 35));
    this.options.onTelemetry('realmguard.hero.move', { x: Math.round(point.x), y: Math.round(point.y) });
  }

  private buildTower(towerId: string) {
    if (!this.selectedSpot || this.towers.has(this.selectedSpot)) return;
    const definition = this.options.config.towers.find((tower) => tower.id === towerId);
    const spot = this.options.stage.towerSpots.find((item) => item.id === this.selectedSpot);
    if (!definition || !spot || this.gold < definition.cost) return;
    this.gold -= definition.cost;
    this.spentGold += definition.cost;
    const base = this.spots.get(spot.id)!;
    base.removeAll(true);
    const shape = this.drawTowerShape(definition, 1);
    const object = this.add.container(spot.x, spot.y, shape).setSize(68, 68).setInteractive({ useHandCursor: true }).setDepth(7);
    object.on('pointerdown', (pointer: Phaser.Input.Pointer) => { pointer.event.stopPropagation(); this.selectedSpot = spot.id; this.redrawSpotSelection(); this.emitHUD(true); });
    this.spots.set(spot.id, object);
    const soldiers = definition.id === 'windward' ? this.createBarracksSoldiers(spot.x, spot.y) : undefined;
    this.towers.set(spot.id, { spotId: spot.id, definition, object, level: 1, targeting: 'first', lastShot: 0, disabledUntil: 0, soldiers, blockedEnemies: new Set() });
    this.options.onTelemetry('realmguard.tower.build', { tower: towerId, spot: spot.id });
    this.emitHUD(true);
  }

  private drawTowerShape(definition: TowerDefinition, level: number) {
    const base = this.add.graphics();
    base.fillStyle(0x0a1522, .9).fillCircle(0, 0, 27);
    base.lineStyle(3 + level, definition.color, .95).strokeCircle(0, 0, 20 + level * 2);
    if (definition.id === 'sunspire') base.fillStyle(definition.color).fillTriangle(0, -22, -13, 13, 13, 13);
    if (definition.id === 'runebloom') for (let index = 0; index < 6; index += 1) base.fillStyle(definition.color, .8).fillCircle(Math.cos(index * Math.PI / 3) * 13, Math.sin(index * Math.PI / 3) * 13, 7);
    if (definition.id === 'stonepulse') base.fillStyle(definition.color).fillRoundedRect(-15, -15, 30, 30, 5).fillStyle(0x202938).fillRect(-5, -28, 10, 25);
    if (definition.id === 'windward') {
      base.fillStyle(0x334155).fillRoundedRect(-17, -15, 34, 27, 5);
      base.fillStyle(definition.color).fillTriangle(-15, -15, -7, -28, 0, -15).fillTriangle(0, -15, 8, -28, 15, -15);
      base.fillStyle(0xdffcff).fillCircle(-10, 13, 5).fillCircle(10, 13, 5);
    }
    const badge = this.add.text(0, 26, `L${level}`, { fontFamily: 'system-ui', fontSize: '16px', color: '#ffffff', backgroundColor: '#07101ddd', padding: { x: 4, y: 1 } }).setOrigin(.5);
    return [base, badge];
  }

  private createBarracksSoldiers(x: number, y: number) {
    const nearest = this.options.stage.path.reduce((best, point) => Phaser.Math.Distance.Between(x, y, point.x, point.y) < Phaser.Math.Distance.Between(x, y, best.x, best.y) ? point : best, this.options.stage.path[0]);
    return [-18, 18].map((offset) => {
      const shield = this.add.graphics().fillStyle(0x69dce4, .95).fillCircle(0, 0, 8).fillStyle(0xe7fbff).fillTriangle(-7, -3, 7, -3, 0, 10).lineStyle(2, 0x173b49).strokeCircle(0, 0, 9);
      return this.add.container(nearest.x + offset, nearest.y + (offset > 0 ? 16 : -16), [shield]).setDepth(8);
    });
  }

  private upgradeTower(branch?: string) {
    if (!this.selectedSpot) return;
    const tower = this.towers.get(this.selectedSpot);
    if (!tower || tower.level >= 3) return;
    const cost = this.options.config.balance.towerUpgradeCost[tower.level] ?? 100;
    if (this.gold < cost) return;
    this.gold -= cost; this.spentGold += cost; tower.level += 1;
    if (tower.level === 3 && branch && tower.definition.branches.some((item) => item.id === branch)) tower.branch = branch;
    tower.object.removeAll(true); tower.object.add(this.drawTowerShape(tower.definition, tower.level));
    this.pulse(tower.object.x, tower.object.y, tower.definition.color, 52);
    this.options.onTelemetry('realmguard.tower.upgrade', { tower: tower.definition.id, spot: tower.spotId, level: tower.level, branch: tower.branch });
    this.emitHUD(true);
  }

  private sellTower() {
    if (!this.selectedSpot) return;
    const tower = this.towers.get(this.selectedSpot);
    if (!tower) return;
    const invested = tower.definition.cost + this.options.config.balance.towerUpgradeCost.slice(1, tower.level).reduce((sum, value) => sum + value, 0);
    const refund = Math.round(invested * this.options.config.balance.sellRefundRate);
    this.gold += refund;
    this.soldGold += refund;
    tower.soldiers?.forEach((soldier) => soldier.destroy(true));
    tower.object.destroy(true); this.towers.delete(this.selectedSpot);
    const spot = this.options.stage.towerSpots.find((item) => item.id === this.selectedSpot)!;
    const ring = this.add.graphics().fillStyle(0x0a1623, .78).fillCircle(0, 0, 29).lineStyle(3, 0x8bd8c5, .72).strokeCircle(0, 0, 27);
    const plus = this.add.text(0, -2, '+', { fontFamily: 'system-ui', fontSize: '30px', color: '#a9e7d6', fontStyle: 'bold' }).setOrigin(.5);
    const container = this.add.container(spot.x, spot.y, [ring, plus]).setSize(62, 62).setInteractive({ useHandCursor: true }).setDepth(4);
    container.on('pointerdown', (pointer: Phaser.Input.Pointer) => { pointer.event.stopPropagation(); this.selectedSpot = spot.id; this.redrawSpotSelection(); this.emitHUD(true); });
    this.spots.set(spot.id, container);
    this.options.onTelemetry('realmguard.tower.sell', { spot: spot.id });
    this.emitHUD(true);
  }

  private changeTargeting(mode: TargetingMode) {
    if (!TARGETING.includes(mode) || !this.selectedSpot) return;
    const tower = this.towers.get(this.selectedSpot); if (!tower) return;
    tower.targeting = mode; this.options.onTelemetry('realmguard.tower.targeting', { tower: tower.definition.id, mode }); this.emitHUD(true);
  }

  private armSkill(skillId: string) {
    const skill = this.options.config.skills.find((item) => item.id === skillId);
    if (!skill || (this.skillReady.get(skillId) ?? 0) > this.simulationTime) return;
    this.skillReady.set(skillId, this.simulationTime + skill.cooldown * 1000);
    if (skillId === 'freeze') {
      for (const enemy of this.enemies.values()) if (!this.hasTrait(enemy, 'immune_stun')) { enemy.slowUntil = this.simulationTime + 5000; enemy.slowFactor = .18; }
      this.cameras.main.flash(180, 120, 220, 255, false);
      this.options.onTelemetry('realmguard.skill.cast', { skill: skillId });
    } else if (skillId === 'meteor' || skillId === 'reinforcement') this.pointerMode = skillId;
    else return;
    this.emitHUD(true);
  }

  private castMeteor(point: Phaser.Math.Vector2) {
    for (const enemy of this.enemies.values()) if (Phaser.Math.Distance.Between(point.x, point.y, enemy.object.x, enemy.object.y) < 125) this.damageEnemy(enemy, 245, 'skill');
    const impact = this.add.circle(point.x, point.y, 8, 0xff774e, .95).setDepth(14);
    this.tweens.add({ targets: impact, radius: 125, alpha: 0, duration: 460, onComplete: () => impact.destroy() });
    this.options.onTelemetry('realmguard.skill.cast', { skill: 'meteor', x: Math.round(point.x), y: Math.round(point.y) });
  }

  private castReinforcement(point: Phaser.Math.Vector2) {
    const ward = this.add.graphics().fillStyle(0xffd36b, .18).fillCircle(0, 0, 56).lineStyle(3, 0xffd36b, .8).strokeCircle(0, 0, 48);
    const blades = this.add.graphics().fillStyle(0xffe7a1).fillTriangle(-18, 12, -4, -20, 2, 16).fillTriangle(8, 15, 17, -18, 24, 13);
    const object = this.add.container(point.x, point.y, [ward, blades]).setDepth(8);
    this.reinforcements.push({ object, point: point.clone(), nextStrikeAt: this.simulationTime + 650, expiresAt: this.simulationTime + 8200 });
    this.options.onTelemetry('realmguard.skill.cast', { skill: 'reinforcement', x: Math.round(point.x), y: Math.round(point.y) });
  }

  private updateReinforcements(time: number) {
    for (const unit of this.reinforcements) {
      if (time >= unit.expiresAt) { unit.object.destroy(true); continue; }
      while (time >= unit.nextStrikeAt) {
        unit.nextStrikeAt += 650;
        const enemy = [...this.enemies.values()]
          .filter((item) => !this.hasTrait(item, 'flying') && Phaser.Math.Distance.Between(unit.point.x, unit.point.y, item.object.x, item.object.y) < 70)
          .sort((a, b) => b.pathProgress - a.pathProgress)[0];
        if (enemy) this.damageEnemy(enemy, 28, 'hero', .35);
      }
    }
    this.reinforcements = this.reinforcements.filter((unit) => unit.object.active && time < unit.expiresAt);
  }

  private completeWave() {
    const wave = this.options.stage.waves[this.waveIndex % this.options.stage.waves.length];
    this.gold += wave.reward;
    this.earnedGold += wave.reward;
    this.options.onTelemetry('realmguard.wave.complete', {
      stage_id: this.options.stage.id, wave: this.waveIndex + 1, lives: this.lives, kills: this.kills,
      escaped: this.escaped, spawned: this.spawned, gold: this.gold, earned_gold: this.earnedGold,
      spent_gold: this.spentGold, sold_gold: this.soldGold, hero_level: this.hero?.level ?? 1,
      defeated_by_enemy: { ...this.defeatedByEnemy }, escaped_by_enemy: { ...this.escapedByEnemy }, spawned_by_enemy: { ...this.spawnedByEnemy },
    });
    this.waveIndex += 1; this.waveActive = false;
    if (this.options.stage.mode === 'campaign' && this.waveIndex >= this.options.stage.waves.length) void this.endBattle(true);
    else { this.gameStatus = 'ready'; this.nextWaveAt = this.simulationTime + 10_000; this.emitHUD(true); }
  }

  private togglePause() {
    if (this.gameStatus === 'paused') {
      this.gameStatus = this.waveActive ? 'playing' : 'ready';
      this.time.paused = false;
      this.tweens.resumeAll();
      this.options.onTelemetry('game.resume');
    } else {
      this.gameStatus = 'paused';
      this.time.paused = true;
      this.tweens.pauseAll();
      this.options.onTelemetry('game.pause');
    }
    this.emitHUD(true);
  }

  private async endBattle(victory: boolean) {
    if (this.completed) return;
    this.completed = true; this.gameStatus = victory ? 'victory' : 'defeat';
    const duration = Math.round(this.activeWallMs);
    const local = calculateLocalResult({ victory, lives: this.lives, kills: this.kills, waves: this.waveIndex, gold: this.gold, duration_ms: duration, difficulty: this.options.difficulty, mode: this.options.stage.mode }, this.options.config.balance);
    const stats: BattleStats = {
      stage_id: this.options.stage.id, mode: this.options.stage.mode, difficulty: this.options.difficulty,
      duration_ms: duration, lives: this.lives, gold: this.gold, earned_gold: this.earnedGold, spent_gold: this.spentGold, sold_gold: this.soldGold,
      kills: this.kills, waves: this.waveIndex, waves_completed: this.waveIndex, escaped: this.escaped, spawned: this.spawned,
      defeated_by_enemy: { ...this.defeatedByEnemy }, escaped_by_enemy: { ...this.escapedByEnemy }, spawned_by_enemy: { ...this.spawnedByEnemy },
      hero_id: this.options.hero.id, hero_level: this.hero?.level ?? 1,
      content_version: this.options.config.contentVersion, balance_version: this.options.config.balanceVersion,
      stage_version: this.options.stage.version, asset_version: this.options.config.assetVersion,
    };
    this.emitHUD(true);
    const result = { ...stats, victory, ...local };
    try {
      await this.options.onTelemetry('realmguard.battle.complete', { ...stats, victory, local_score: local.score, local_stars: local.stars });
      this.options.onComplete(result);
    } catch (cause) {
      this.options.onCompleteError(result, cause instanceof Error ? cause : new Error('전투 검증 로그를 전송하지 못했습니다.'));
    }
  }

  private emitHUD(force = false) {
    if (!force && this.simulationTime - this.lastHUD < 180) return;
    this.lastHUD = this.simulationTime;
    const selected = this.selectedSpot ? this.towers.get(this.selectedSpot) : undefined;
    const cooldowns: Record<string, number> = {};
    for (const skill of this.options.config.skills) cooldowns[skill.id] = Math.max(0, Math.ceil(((this.skillReady.get(skill.id) ?? 0) - this.simulationTime) / 1000));
    this.options.onHUD({
      status: this.gameStatus, gold: this.gold, lives: this.lives, wave: this.waveIndex + (this.waveActive ? 1 : 0),
      totalWaves: this.options.stage.mode === 'endless' ? 0 : this.options.stage.waves.length, kills: this.kills,
      heroLevel: this.hero?.level ?? 1, heroAlive: !this.hero?.deadUntil, heroRespawn: this.hero?.deadUntil ? Math.max(0, Math.ceil((this.hero.deadUntil - this.simulationTime) / 1000)) : 0,
      nextWaveIn: this.waveActive ? 0 : Math.max(0, Math.ceil((this.nextWaveAt - this.simulationTime) / 1000)), selectedSpot: this.selectedSpot,
      selectedTower: selected ? { type: selected.definition.id, level: selected.level, branch: selected.branch, targeting: selected.targeting } : undefined,
      skillCooldowns: cooldowns, speed: this.battleSpeed,
    });
  }

  private pulse(x: number, y: number, color: number, radius: number) {
    const ring = this.add.circle(x, y, 8, color, .18).setStrokeStyle(3, color, .8).setDepth(13);
    this.tweens.add({ targets: ring, radius, alpha: 0, duration: 360, onComplete: () => ring.destroy() });
  }

  private burst(x: number, y: number, color: number) {
    for (let index = 0; index < 7; index += 1) {
      const shard = this.add.circle(x, y, 3 + index % 3, color, .85).setDepth(13);
      const angle = index / 7 * Math.PI * 2;
      this.tweens.add({ targets: shard, x: x + Math.cos(angle) * (28 + index * 3), y: y + Math.sin(angle) * (28 + index * 3), alpha: 0, duration: 330, onComplete: () => shard.destroy() });
    }
  }
}

export function mountRealmGuard(parent: HTMLElement, options: MountOptions): RealmSceneController {
  const scene = new RealmGuardBattleScene(options);
  const game = new Phaser.Game({
    type: Phaser.AUTO,
    parent,
    width: WIDTH,
    height: HEIGHT,
    backgroundColor: '#101a27',
    scene,
    transparent: false,
    input: { mouse: { preventDefaultWheel: true }, touch: { capture: true } },
    render: { antialias: true, roundPixels: true },
    scale: { mode: Phaser.Scale.FIT, autoCenter: Phaser.Scale.CENTER_BOTH, width: WIDTH, height: HEIGHT },
  });
  return {
    command: (command) => scene.command(command),
    destroy: () => game.destroy(true),
  };
}
