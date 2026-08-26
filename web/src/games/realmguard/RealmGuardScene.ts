import Phaser from "phaser";
import { advanceFixedSimulation, calculateLocalResult } from "./content";
import { BattleKernel, KERNEL_TICK_MS } from "./kernel/kernel";
import type { KernelEvent, KernelTowerView } from "./kernel/kernel";
import { drawEnemyBody } from "./enemyArt";
import { resolveEnemyPresentation } from "./enemyPresentation";
import { drawTowerBody } from "./towerArt";
import { resolveTowerPresentation } from "./towerPresentation";
import { kernelDigest, projectKernelConfig } from "./kernel/config";
import { LedgerRecorder } from "./kernel/ledger";
import type { KernelAction, KernelCommand } from "./kernel/ledger";
import {
  resolveHeroPresentation,
  type HeroPortraitMotif,
  type HeroPresentationGame,
} from "./heroPresentation";
import type {
  BattleHUD,
  BattleStats,
  EnemyArchetype,
  HeroDefinition,
  RealmCommand,
  RealmDifficulty,
  RealmGuardConfig,
  RealmResult,
  RealmSceneController,
  RealmStage,
  TargetingMode,
  TowerDefinition,
} from "./types";

interface SceneCallbacks {
  onHUD: (hud: BattleHUD) => void;
  onTelemetry: (
    event: string,
    data?: Record<string, unknown>,
  ) => void | Promise<void>;
  onComplete: (result: RealmResult) => void;
  onCompleteError: (result: RealmResult, error: Error) => void;
}

interface MountOptions extends SceneCallbacks {
  config: RealmGuardConfig;
  stage: RealmStage;
  difficulty: RealmDifficulty;
  hero: HeroDefinition;
  accountHeroLevel: number;
  presentationGame?: HeroPresentationGame;
}

interface EnemySprite {
  object: Phaser.GameObjects.Container;
  body: Phaser.GameObjects.Graphics;
  health: Phaser.GameObjects.Graphics;
  definition: EnemyArchetype;
}

interface TowerSprite {
  object: Phaser.GameObjects.Container;
  soldiers: Phaser.GameObjects.Container[];
  level: number;
  definition: TowerDefinition;
}

interface HeroSprite {
  object: Phaser.GameObjects.Container;
  health: Phaser.GameObjects.Graphics;
  levelLabel: Phaser.GameObjects.Text;
  motif: HeroPortraitMotif;
  level: number;
}

const WIDTH = 1280;
const HEIGHT = 720;
const TARGETING: TargetingMode[] = [
  "first",
  "last",
  "strong",
  "weak",
  "closest",
];
const themeColors: Record<
  RealmStage["theme"],
  { ground: number; accent: number; path: number; fog: number }
> = {
  verdant: {
    ground: 0x173d38,
    accent: 0x4ca66d,
    path: 0x756c55,
    fog: 0x0a2829,
  },
  ember: { ground: 0x422d2c, accent: 0xd27352, path: 0x786153, fog: 0x241b27 },
  frost: { ground: 0x244353, accent: 0x7ccbe1, path: 0x71858a, fog: 0x142a3b },
  void: { ground: 0x282440, accent: 0x9c72d6, path: 0x625d72, fog: 0x111225 },
};
type ThemePalette = (typeof themeColors)[RealmStage["theme"]];

/**
 * Phaser presentation for a battle the kernel decides.
 *
 * The scene owns pixels, tweens and pointer intent only: every rule that can
 * change a score lives in `BattleKernel`, and every player action is appended to
 * the ledger the server replays. Nothing drawn here can move the outcome.
 */
class RealmGuardBattleScene extends Phaser.Scene {
  private readonly options: MountOptions;
  private readonly kernel: BattleKernel;
  private readonly recorder = new LedgerRecorder();
  private readonly configDigest: string;
  private readonly enemySprites = new Map<number, EnemySprite>();
  private readonly towerSprites = new Map<string, TowerSprite>();
  private readonly spots = new Map<string, Phaser.GameObjects.Container>();
  private hero?: HeroSprite;
  private selectedSpot?: string;
  private pointerMode?: "meteor" | "reinforcement" | "move-hero";
  private paused = false;
  private battleSpeed: 1 | 2 = 1;
  private accumulator = 0;
  private lastHUD = -1000;
  private completed = false;

  private isBlockingTower(definition: TowerDefinition) {
    return (
      definition.id === "windward" ||
      definition.id === this.options.config.towers.at(-1)?.id
    );
  }

  constructor(options: MountOptions) {
    super({ key: `RealmGuard-${Date.now()}` });
    this.options = options;
    const projection = projectKernelConfig(
      options.config,
      options.stage,
      options.difficulty,
      options.hero.id,
    );
    this.configDigest = kernelDigest(projection);
    this.kernel = new BattleKernel(projection, options.accountHeroLevel);
  }

  create() {
    this.drawWorld();
    this.createTowerSpots();
    this.createHero();
    this.input.on(
      "pointerdown",
      (
        pointer: Phaser.Input.Pointer,
        objects: Phaser.GameObjects.GameObject[],
      ) => {
        if (objects.length || !this.pointerMode || this.completed) return;
        const x = pointer.worldX;
        const y = pointer.worldY;
        if (this.pointerMode === "move-hero") this.send({ op: "hero", x, y });
        if (this.pointerMode === "meteor") this.send({ op: "meteor", x, y });
        if (this.pointerMode === "reinforcement")
          this.send({ op: "reinforce", x, y });
        this.pointerMode = undefined;
        this.drainKernel();
        this.emitHUD(true);
      },
    );
    this.emitHUD(true);
    this.options.onTelemetry("realmguard.battle.ready", {
      stage_id: this.options.stage.id,
      difficulty: this.options.difficulty,
      hero_id: this.options.hero.id,
    });
  }

  update(_time: number, delta: number) {
    if (this.paused || this.completed) return;
    const advance = advanceFixedSimulation(
      this.accumulator,
      delta,
      this.battleSpeed,
      KERNEL_TICK_MS,
    );
    this.accumulator = advance.remainder;
    for (let step = 0; step < advance.steps; step += 1) {
      this.kernel.tick();
      this.drainKernel();
      if (this.completed) break;
    }
    this.syncSprites();
    this.emitHUD();
  }

  // ------------------------------------------------------------- kernel edge

  /** Applies a player action to the kernel and appends it to the replay ledger. */
  private send(action: KernelAction) {
    if (this.completed) return;
    const entry = { ...action, tick: this.kernel.ticks } as KernelCommand;
    this.kernel.apply(entry);
    this.recorder.record(entry);
  }

  command(command: RealmCommand) {
    if (this.completed && command.type !== "toggle-pause") return;
    switch (command.type) {
      case "start-wave":
        this.send({ op: "wave" });
        break;
      case "toggle-pause":
        this.togglePause();
        return;
      case "speed":
        this.battleSpeed = command.value;
        this.tweens.timeScale = command.value;
        this.time.timeScale = command.value;
        break;
      case "build":
        if (this.selectedSpot)
          this.send({
            op: "build",
            spot: this.selectedSpot,
            tower: command.tower,
            profile: command.profile,
          });
        break;
      case "upgrade":
        if (this.selectedSpot)
          this.send({
            op: "upgrade",
            spot: this.selectedSpot,
            branch: command.branch,
          });
        break;
      case "sell":
        if (this.selectedSpot) this.send({ op: "sell", spot: this.selectedSpot });
        break;
      case "targeting":
        if (this.selectedSpot && TARGETING.includes(command.mode))
          this.send({
            op: "target",
            spot: this.selectedSpot,
            mode: command.mode,
          });
        break;
      case "skill":
        this.armSkill(command.skill);
        break;
      case "move-hero":
        this.pointerMode = "move-hero";
        this.options.onTelemetry("realmguard.hero.move_armed");
        break;
      case "adjust-economy":
        this.send({
          op: "economy",
          gold: command.resourceDelta,
          lives: command.healthDelta ?? 0,
        });
        break;
      case "force-defeat":
        this.send({ op: "defeat" });
        break;
    }
    this.drainKernel();
    this.syncSprites();
    this.emitHUD(true);
  }

  private armSkill(skillId: string) {
    const skill = this.options.config.skills.find((item) => item.id === skillId);
    if (!skill) return;
    const ready = (this.kernel.status().skillCooldowns[skillId] ?? 0) === 0;
    if (!ready) return;
    this.send({ op: "skill", skill: skillId });
    if (skillId === "meteor" || skillId === "reinforcement")
      this.pointerMode = skillId;
  }

  private togglePause() {
    this.paused = !this.paused;
    if (this.paused) {
      this.time.paused = true;
      this.tweens.pauseAll();
      this.options.onTelemetry("game.pause");
    } else {
      this.time.paused = false;
      this.tweens.resumeAll();
      this.options.onTelemetry("game.resume");
    }
    this.emitHUD(true);
  }

  private drainKernel() {
    for (const event of this.kernel.drainEvents()) this.renderEvent(event);
  }

  private renderEvent(event: KernelEvent) {
    switch (event.k) {
      case "spawn":
        this.createEnemySprite(event.id);
        break;
      case "despawn": {
        const sprite = this.enemySprites.get(event.id);
        if (sprite) {
          if (event.killed)
            this.burst(
              sprite.object.x,
              sprite.object.y,
              sprite.definition.color,
            );
          sprite.object.destroy(true);
          this.enemySprites.delete(event.id);
        }
        break;
      }
      case "shot":
        this.drawProjectile(event.spot, event.x, event.y, event.radius);
        break;
      case "melee":
        this.drawMelee(event.spot, event.x, event.y);
        break;
      case "hero-shot":
        this.drawHeroAttack(event.id);
        break;
      case "hero-respawn":
        if (this.hero) {
          this.hero.object.setVisible(true).setAlpha(1);
          this.pulse(
            this.hero.object.x,
            this.hero.object.y,
            this.options.hero.color,
            70,
          );
        }
        break;
      case "hero-hit":
        if (this.hero) {
          this.pulse(this.hero.object.x, this.hero.object.y, 0xff6f72, 30);
          if (this.kernel.heroView().deadUntil > 0)
            this.hero.object.setVisible(false);
        }
        break;
      case "hero-level":
        if (this.hero)
          this.pulse(this.hero.object.x, this.hero.object.y, 0xffdf72, 56);
        break;
      case "hero-ultimate":
        this.cameras.main.flash(180, 255, 224, 125, false);
        break;
      case "tower":
        this.applyTowerChange(event.spot, event.change);
        break;
      case "heal":
        this.pulse(event.x, event.y, 0x8ff0bd, 65);
        break;
      case "meteor":
        this.drawMeteor(event.x, event.y);
        break;
      case "reinforce":
        this.drawReinforcement(event.x, event.y);
        break;
      case "freeze":
        this.cameras.main.flash(180, 120, 220, 255, false);
        break;
      case "gimmick":
        if (event.kind === "ember_vents") this.pulse(event.x, event.y, 0xff7c55, 120);
        else if (event.kind === "winter_blessing")
          this.cameras.main.flash(140, 130, 220, 255, false);
        else this.cameras.main.shake(140, 0.003);
        break;
      case "boss-phase": {
        const sprite = this.enemySprites.get(event.id);
        if (event.phase === 2 && sprite)
          this.pulse(
            sprite.object.x,
            sprite.object.y,
            sprite.definition.color,
            120,
          );
        if (event.phase === 3) this.cameras.main.shake(260, 0.006);
        break;
      }
      case "telemetry":
        void this.options.onTelemetry(event.event, event.data);
        break;
      case "complete":
        void this.finish(event.victory);
        break;
      default:
        break;
    }
  }

  // ----------------------------------------------------------------- sprites

  private syncSprites() {
    for (const enemy of this.kernel.enemyView()) {
      const sprite = this.enemySprites.get(enemy.id);
      if (!sprite) continue;
      sprite.object.setPosition(enemy.x, enemy.y);
      this.drawEnemyHealth(sprite, enemy.hp / enemy.maxHp);
    }
    const heroState = this.kernel.heroView();
    if (this.hero) {
      this.hero.object.setPosition(heroState.x, heroState.y);
      this.hero.object.setVisible(heroState.deadUntil === 0);
      this.drawHeroHealth(this.hero, heroState.hp / heroState.maxHp);
      if (this.hero.level !== heroState.level) {
        this.hero.level = heroState.level;
        this.hero.levelLabel.setText(
          `${this.options.hero.name} · Lv.${heroState.level}`,
        );
      }
    }
    const now = this.kernel.time;
    for (const tower of this.kernel.towerView()) {
      const sprite = this.towerSprites.get(tower.spotId);
      if (!sprite) continue;
      const alpha = tower.disabledUntil > now ? 0.35 : 1;
      sprite.object.setAlpha(alpha);
      for (const soldier of sprite.soldiers)
        if (soldier.active) soldier.setAlpha(alpha);
    }
  }

  private createEnemySprite(id: number) {
    const state = this.kernel.enemyView().find((enemy) => enemy.id === id);
    if (!state) return;
    const definition = this.options.config.enemies[state.def];
    if (!definition) return;
    // Traits come from the pinned content and from the wave's own modifiers, so
    // what a player reads on a body is exactly what the battle rules act on.
    const traits = [...definition.traits, ...state.modifiers];
    const flying = traits.includes("flying");
    const presentation = resolveEnemyPresentation(
      definition.id,
      traits,
      this.options.presentationGame ?? "realmguard",
    );
    const body = this.add.graphics();
    drawEnemyBody(
      body,
      // Content owns the colour; art direction only supplies one when the
      // roster does not, so an operator's palette still wins on the field.
      definition.color ? { ...presentation, primary: `#${definition.color.toString(16).padStart(6, "0")}` } : presentation,
      definition.radius,
    );
    const health = this.add.graphics();
    const label = definition.traits.includes("boss")
      ? this.add
          .text(0, definition.radius + 13, definition.name, {
            fontFamily: "system-ui",
            fontSize: "16px",
            color: "#ffe29b",
            backgroundColor: "#111827cc",
            padding: { x: 4, y: 2 },
          })
          .setOrigin(0.5, 0)
      : undefined;
    const object = this.add
      .container(state.x, state.y, label ? [body, health, label] : [body, health])
      .setDepth(flying ? 8 : 6);
    const sprite: EnemySprite = { object, body, health, definition };
    this.enemySprites.set(id, sprite);
    this.drawEnemyHealth(sprite, 1);
  }

  private drawEnemyHealth(sprite: EnemySprite, ratio: number) {
    const clamped = Math.max(0, Math.min(1, ratio));
    const radius = sprite.definition.radius;
    // Scaled off the radius rather than fixed: silhouettes reach about 1.5
    // radii above their centre, and a boss carries a ring at 1.22, so a
    // constant offset left the bar sitting inside the biggest bodies.
    const width = Math.max(26, radius * 2.2);
    const top = -radius * 1.55 - 6;
    sprite.health.clear();
    sprite.health
      .fillStyle(0x07101d, 0.85)
      .fillRect(-width / 2, top, width, 5);
    sprite.health
      .fillStyle(clamped > 0.35 ? 0x65e392 : 0xff6f72)
      .fillRect(-width / 2 + 1, top + 1, (width - 2) * clamped, 3);
  }

  private applyTowerChange(
    spotId: string,
    change: "build" | "upgrade" | "sell" | "disable",
  ) {
    const state = this.kernel.towerAt(spotId);
    if (change === "sell") {
      const sprite = this.towerSprites.get(spotId);
      sprite?.soldiers.forEach((soldier) => soldier.destroy(true));
      sprite?.object.destroy(true);
      this.towerSprites.delete(spotId);
      this.restoreSpot(spotId);
      return;
    }
    if (!state) return;
    if (change === "build") this.createTowerSprite(state);
    if (change === "upgrade") {
      const sprite = this.towerSprites.get(spotId);
      if (!sprite) return;
      sprite.object.removeAll(true);
      sprite.object.add(this.drawTowerShape(sprite.definition, state.level, state.branch));
      sprite.level = state.level;
      this.pulse(sprite.object.x, sprite.object.y, sprite.definition.color, 52);
    }
  }

  private createTowerSprite(state: KernelTowerView) {
    const definition = this.options.config.towers[state.def];
    const spot = this.options.stage.towerSpots.find(
      (item) => item.id === state.spotId,
    );
    if (!definition || !spot) return;
    const base = this.spots.get(spot.id);
    base?.removeAll(true);
    const object = this.add
      .container(spot.x, spot.y, this.drawTowerShape(definition, state.level, state.branch))
      .setSize(68, 68)
      .setInteractive({ useHandCursor: true })
      .setDepth(7);
    object.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
      pointer.event.stopPropagation();
      this.selectSpot(spot.id);
    });
    this.spots.set(spot.id, object);
    const soldiers = state.soldiers.map((point) => {
      const shield = this.add
        .graphics()
        .fillStyle(0x69dce4, 0.95)
        .fillCircle(0, 0, 8)
        .fillStyle(0xe7fbff)
        .fillTriangle(-7, -3, 7, -3, 0, 10)
        .lineStyle(2, 0x173b49)
        .strokeCircle(0, 0, 9);
      return this.add.container(point.x, point.y, [shield]).setDepth(8);
    });
    this.towerSprites.set(spot.id, {
      object,
      soldiers,
      level: state.level,
      definition,
    });
  }

  private restoreSpot(spotId: string) {
    const spot = this.options.stage.towerSpots.find((item) => item.id === spotId);
    if (!spot) return;
    this.spots.get(spotId)?.destroy(true);
    const ring = this.add
      .graphics()
      .fillStyle(0x0a1623, 0.78)
      .fillCircle(0, 0, 29)
      .lineStyle(3, 0x8bd8c5, 0.72)
      .strokeCircle(0, 0, 27);
    const plus = this.add
      .text(0, -2, "+", {
        fontFamily: "system-ui",
        fontSize: "30px",
        color: "#a9e7d6",
        fontStyle: "bold",
      })
      .setOrigin(0.5);
    const container = this.add
      .container(spot.x, spot.y, [ring, plus])
      .setSize(62, 62)
      .setInteractive({ useHandCursor: true })
      .setDepth(4);
    container.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
      pointer.event.stopPropagation();
      this.selectSpot(spot.id);
    });
    this.spots.set(spot.id, container);
  }

  private selectSpot(spotId: string) {
    this.selectedSpot = spotId;
    for (const [id, spot] of this.spots)
      spot.setScale(id === this.selectedSpot ? 1.14 : 1);
    this.emitHUD(true);
  }

  // ------------------------------------------------------------------ effects

  private drawProjectile(spotId: string, targetX: number, targetY: number, radius: number) {
    const sprite = this.towerSprites.get(spotId);
    if (!sprite) return;
    this.pulse(targetX, targetY, sprite.definition.color, radius);
    const projectile = this.add
      .circle(
        sprite.object.x,
        sprite.object.y,
        sprite.definition.damageType === "siege" ? 7 : 4,
        sprite.definition.color,
      )
      .setDepth(12);
    this.tweens.add({
      targets: projectile,
      x: targetX,
      y: targetY,
      duration: Math.max(
        80,
        (Phaser.Math.Distance.Between(
          projectile.x,
          projectile.y,
          targetX,
          targetY,
        ) /
          sprite.definition.projectileSpeed) *
          1000,
      ),
      onComplete: () => projectile.destroy(),
    });
  }

  private drawMelee(spotId: string, targetX: number, targetY: number) {
    const sprite = this.towerSprites.get(spotId);
    if (!sprite) return;
    this.pulse(targetX, targetY, sprite.definition.color, 24);
    const soldier = sprite.soldiers[0];
    if (!soldier || !soldier.active) return;
    const homeX = soldier.x;
    const homeY = soldier.y;
    this.tweens.add({
      targets: soldier,
      x: Phaser.Math.Linear(homeX, targetX, 0.42),
      y: Phaser.Math.Linear(homeY, targetY, 0.42),
      duration: 120,
      yoyo: true,
      onComplete: () => soldier.active && soldier.setPosition(homeX, homeY),
    });
  }

  private drawHeroAttack(targetId: number) {
    const hero = this.hero;
    const target = this.enemySprites.get(targetId);
    if (!hero || !target) return;
    this.pulse(target.object.x, target.object.y, this.options.hero.color, 24);
    const angle = Phaser.Math.Angle.Between(
      hero.object.x,
      hero.object.y,
      target.object.x,
      target.object.y,
    );
    if (hero.motif === "bow" || hero.motif === "target") {
      const arrow = this.add
        .rectangle(
          hero.object.x,
          hero.object.y,
          22,
          4,
          this.options.hero.color,
          0.95,
        )
        .setRotation(angle)
        .setDepth(12);
      this.tweens.add({
        targets: arrow,
        x: target.object.x,
        y: target.object.y,
        alpha: 0.25,
        duration: 115,
        onComplete: () => arrow.destroy(),
      });
      return;
    }
    if (["shield", "lock", "ai-shield", "command"].includes(hero.motif)) {
      const strike = this.add
        .arc(
          target.object.x,
          target.object.y,
          22,
          Phaser.Math.RadToDeg(angle) - 65,
          Phaser.Math.RadToDeg(angle) + 65,
          false,
          this.options.hero.color,
          0.18,
        )
        .setStrokeStyle(5, this.options.hero.color, 0.9)
        .setDepth(12);
      this.tweens.add({
        targets: strike,
        scale: 1.35,
        alpha: 0,
        duration: 180,
        onComplete: () => strike.destroy(),
      });
      return;
    }
    const orb = this.add
      .circle(hero.object.x, hero.object.y, 7, this.options.hero.color, 0.95)
      .setStrokeStyle(2, 0xffffff, 0.75)
      .setDepth(12);
    this.tweens.add({
      targets: orb,
      x: target.object.x,
      y: target.object.y,
      scale: 0.45,
      duration: 155,
      onComplete: () => orb.destroy(),
    });
  }

  private drawMeteor(x: number, y: number) {
    const impact = this.add.circle(x, y, 8, 0xff774e, 0.95).setDepth(14);
    this.tweens.add({
      targets: impact,
      radius: 125,
      alpha: 0,
      duration: 460,
      onComplete: () => impact.destroy(),
    });
  }

  private drawReinforcement(x: number, y: number) {
    const ward = this.add
      .graphics()
      .fillStyle(0xffd36b, 0.18)
      .fillCircle(0, 0, 56)
      .lineStyle(3, 0xffd36b, 0.8)
      .strokeCircle(0, 0, 48);
    const blades = this.add
      .graphics()
      .fillStyle(0xffe7a1)
      .fillTriangle(-18, 12, -4, -20, 2, 16)
      .fillTriangle(8, 15, 17, -18, 24, 13);
    const object = this.add.container(x, y, [ward, blades]).setDepth(8);
    this.time.delayedCall(8200, () => object.destroy(true));
  }

  private pulse(x: number, y: number, color: number, radius: number) {
    const ring = this.add
      .circle(x, y, 8, color, 0.18)
      .setStrokeStyle(3, color, 0.8)
      .setDepth(13);
    this.tweens.add({
      targets: ring,
      radius,
      alpha: 0,
      duration: 360,
      onComplete: () => ring.destroy(),
    });
  }

  private burst(x: number, y: number, color: number) {
    for (let index = 0; index < 7; index += 1) {
      const shard = this.add
        .circle(x, y, 3 + (index % 3), color, 0.85)
        .setDepth(13);
      const angle = (index / 7) * Math.PI * 2;
      this.tweens.add({
        targets: shard,
        x: x + Math.cos(angle) * (28 + index * 3),
        y: y + Math.sin(angle) * (28 + index * 3),
        alpha: 0,
        duration: 330,
        onComplete: () => shard.destroy(),
      });
    }
  }

  // -------------------------------------------------------------------- world

  private drawWorld() {
    const colors = themeColors[this.options.stage.theme];
    const graphics = this.add.graphics();
    graphics.fillStyle(colors.ground).fillRect(0, 0, WIDTH, HEIGHT);
    graphics
      .fillStyle(colors.fog, 0.35)
      .fillCircle(80, 80, 180)
      .fillCircle(1190, 650, 260)
      .fillCircle(660, 40, 140);
    for (let index = 0; index < 42; index += 1) {
      const x = (index * 197 + this.options.stage.number * 61) % WIDTH;
      const y = (index * 113 + this.options.stage.number * 97) % HEIGHT;
      graphics
        .fillStyle(colors.accent, 0.18 + (index % 3) * 0.05)
        .fillCircle(x, y, 4 + (index % 8));
    }
    this.drawWorldDecorations(graphics, colors);
    const lanes = this.options.stage.paths?.length
      ? this.options.stage.paths
      : [this.options.stage.path];
    for (const [laneIndex, lane] of lanes.entries()) {
      const path = lane.map(
        (point) => new Phaser.Math.Vector2(point.x, point.y),
      );
      graphics.lineStyle(68, 0x111827, 0.3).strokePoints(path, false, false);
      graphics.lineStyle(56, colors.path, 1).strokePoints(path, false, false);
      graphics.lineStyle(3, 0xe8dbb4, 0.2).strokePoints(path, false, false);
      this.drawRouteMarkers(graphics, lane, colors.accent);
      const start = lane[0];
      const entryX = Phaser.Math.Clamp(start.x, 18, WIDTH - 18);
      const entryY = Phaser.Math.Clamp(start.y, 18, HEIGHT - 18);
      graphics
        .fillStyle(colors.fog, 0.82)
        .fillCircle(entryX, entryY, 19)
        .lineStyle(3, colors.accent, 0.85)
        .strokeCircle(entryX, entryY, 15)
        .fillStyle(colors.accent, 0.85)
        .fillTriangle(entryX - 5, entryY - 7, entryX - 5, entryY + 7, entryX + 7, entryY);
      this.drawObjective(graphics, lane.at(-1)!, colors, laneIndex);
    }
    this.add
      .text(
        20,
        675,
        `${this.options.stage.name} · ${this.options.stage.version}${lanes.length > 1 ? ` · ${lanes.length}개 진입로` : ""}`,
        {
          fontFamily: "system-ui",
          fontSize: "17px",
          color: "#d8e5ed",
          backgroundColor: "#09131dbb",
          padding: { x: 10, y: 6 },
        },
      )
      .setDepth(20);
  }

  private drawWorldDecorations(graphics: Phaser.GameObjects.Graphics, colors: ThemePalette) {
    const style = this.options.stage.mapStyle ?? `realm-${this.options.stage.theme}`;
    if (style.startsWith("office-")) {
      for (let index = 0; index < 12; index += 1) {
        const x = 40 + ((index * 173 + this.options.stage.number * 41) % 1160);
        const y = 35 + ((index * 229 + this.options.stage.number * 67) % 590);
        const width = 70 + (index % 3) * 22;
        const height = 44 + (index % 4) * 14;
        graphics
          .fillStyle(0x07131d, 0.34)
          .fillRoundedRect(x, y, width, height, 8)
          .lineStyle(2, colors.accent, 0.13)
          .strokeRoundedRect(x, y, width, height, 8);
        for (let window = 0; window < 3; window += 1)
          graphics
            .fillStyle(colors.accent, 0.18)
            .fillRect(x + 12 + window * 19, y + 13, 10, 6);
      }
      return;
    }
    if (style.startsWith("cyber-") || style.startsWith("ai-")) {
      graphics.lineStyle(1, colors.accent, style.startsWith("ai-") ? 0.13 : 0.09);
      for (let x = 0; x <= WIDTH; x += 64) graphics.lineBetween(x, 0, x, HEIGHT);
      for (let y = 0; y <= HEIGHT; y += 64) graphics.lineBetween(0, y, WIDTH, y);
      for (let index = 0; index < 24; index += 1) {
        const x = (index * 227 + this.options.stage.number * 83) % WIDTH;
        const y = (index * 151 + this.options.stage.number * 47) % HEIGHT;
        graphics
          .fillStyle(colors.fog, 0.62)
          .fillCircle(x, y, 11 + (index % 3) * 4)
          .lineStyle(2, colors.accent, 0.38)
          .strokeCircle(x, y, 8 + (index % 3) * 4)
          .fillStyle(colors.accent, 0.55)
          .fillCircle(x, y, 3);
      }
      return;
    }
    for (let index = 0; index < 18; index += 1) {
      const x = (index * 181 + this.options.stage.number * 53) % WIDTH;
      const y = (index * 137 + this.options.stage.number * 89) % HEIGHT;
      graphics
        .fillStyle(colors.fog, 0.38)
        .fillCircle(x, y, 18 + (index % 5) * 5)
        .lineStyle(2, colors.accent, 0.16)
        .strokeCircle(x, y, 13 + (index % 5) * 5);
    }
  }

  private drawRouteMarkers(
    graphics: Phaser.GameObjects.Graphics,
    lane: RealmStage["path"],
    color: number,
  ) {
    for (let index = 1; index < lane.length; index += 1) {
      const start = lane[index - 1];
      const end = lane[index];
      const length = Phaser.Math.Distance.Between(start.x, start.y, end.x, end.y);
      const angle = Phaser.Math.Angle.Between(start.x, start.y, end.x, end.y);
      for (let offset = 76; offset < length - 28; offset += 108) {
        const x = start.x + Math.cos(angle) * offset;
        const y = start.y + Math.sin(angle) * offset;
        const sideX = Math.cos(angle + Math.PI / 2) * 7;
        const sideY = Math.sin(angle + Math.PI / 2) * 7;
        graphics
          .fillStyle(color, 0.28)
          .fillTriangle(
            x + Math.cos(angle) * 9,
            y + Math.sin(angle) * 9,
            x - Math.cos(angle) * 7 + sideX,
            y - Math.sin(angle) * 7 + sideY,
            x - Math.cos(angle) * 7 - sideX,
            y - Math.sin(angle) * 7 - sideY,
          );
      }
    }
  }

  private drawObjective(
    graphics: Phaser.GameObjects.Graphics,
    gate: RealmStage["path"][number],
    colors: ThemePalette,
    laneIndex: number,
  ) {
    const x = Phaser.Math.Clamp(gate.x, 24, WIDTH - 24);
    const y = Phaser.Math.Clamp(gate.y, 24, HEIGHT - 24);
    const style = this.options.stage.mapStyle ?? "realm";
    graphics
      .fillStyle(colors.accent, 0.2)
      .fillCircle(x, y, 43 - laneIndex * 3)
      .lineStyle(5, 0x8fffe8, 0.78)
      .strokeCircle(x, y, 31 - laneIndex * 2);
    if (style.startsWith("office-")) {
      graphics
        .fillStyle(0x0b1b28, 0.94)
        .fillRoundedRect(x - 19, y - 24, 38, 48, 5)
        .fillStyle(colors.accent, 0.8)
        .fillRect(x - 11, y - 14, 8, 8)
        .fillRect(x + 3, y - 14, 8, 8)
        .fillRect(x - 11, y, 8, 8)
        .fillRect(x + 3, y, 8, 8);
    } else if (style.startsWith("cyber-")) {
      graphics
        .fillStyle(0x07121e, 0.95)
        .fillRoundedRect(x - 19, y - 16, 38, 34, 7)
        .lineStyle(4, colors.accent, 0.9)
        .strokeRoundedRect(x - 15, y - 12, 30, 26, 5)
        .fillStyle(colors.accent, 0.9)
        .fillCircle(x, y + 1, 5);
    } else if (style.startsWith("ai-")) {
      graphics
        .fillStyle(0x091526, 0.95)
        .fillTriangle(x, y - 24, x - 23, y + 14, x + 23, y + 14)
        .lineStyle(3, colors.accent, 0.9)
        .strokeCircle(x, y, 11)
        .fillStyle(0xffffff, 0.85)
        .fillCircle(x, y, 4);
    }
  }

  private createTowerSpots() {
    for (const spot of this.options.stage.towerSpots) this.restoreSpot(spot.id);
  }

  private createHero() {
    const state = this.kernel.heroView();
    const presentation = resolveHeroPresentation(
      this.options.hero.id,
      this.options.presentationGame ?? "realmguard",
    );
    const secondary = Phaser.Display.Color.HexStringToColor(
      presentation.secondary,
    ).color;
    const aura = this.add
      .graphics()
      .fillStyle(this.options.hero.color, 0.18)
      .fillCircle(0, 0, 30)
      .lineStyle(2, this.options.hero.color, 0.75)
      .strokeCircle(0, 0, 23);
    for (let ray = 0; ray < 6; ray += 1) {
      const angle = (ray / 6) * Math.PI * 2;
      aura.lineBetween(
        Math.cos(angle) * 24,
        Math.sin(angle) * 24,
        Math.cos(angle) * 29,
        Math.sin(angle) * 29,
      );
    }
    const body = this.drawHeroBody(presentation.motif, secondary);
    const health = this.add.graphics();
    const levelLabel = this.add
      .text(0, 27, `${this.options.hero.name} · Lv.1`, {
        fontFamily: "system-ui",
        fontSize: "15px",
        color: "#ffffff",
        backgroundColor: "#07101dcc",
        padding: { x: 5, y: 2 },
      })
      .setOrigin(0.5, 0);
    const object = this.add
      .container(state.x, state.y, [aura, body, health, levelLabel])
      .setDepth(9);
    this.tweens.add({
      targets: aura,
      angle: 360,
      duration: 9000,
      repeat: -1,
    });
    this.hero = {
      object,
      health,
      levelLabel,
      motif: presentation.motif,
      level: 1,
    };
    this.drawHeroHealth(this.hero, 1);
  }

  private drawHeroBody(motif: HeroPortraitMotif, secondary: number) {
    const color = this.options.hero.color;
    const body = this.add.graphics();
    body.fillStyle(0x020712, 0.42).fillEllipse(3, 15, 36, 13);
    const ranged = motif === "bow" || motif === "target";
    const armored = ["shield", "lock", "ai-shield", "command"].includes(motif);
    const mystic = ["staff", "research", "data", "network"].includes(motif);
    if (ranged) {
      body
        .fillStyle(color, 0.9)
        .fillTriangle(-15, 15, 0, -17, 15, 15)
        .fillStyle(secondary, 0.95)
        .fillCircle(0, -9, 9)
        .lineStyle(3, 0xf8e8c4, 0.95)
        .strokeCircle(13, 1, 13)
        .lineBetween(13, -12, 13, 14)
        .lineStyle(3, secondary, 0.9)
        .lineBetween(-15, 9, -4, -1);
    } else if (armored) {
      body
        .fillStyle(color, 0.95)
        .fillRoundedRect(-15, -8, 30, 27, 7)
        .fillStyle(secondary, 0.9)
        .fillCircle(0, -14, 10)
        .fillStyle(0x18202d, 0.92)
        .fillTriangle(-19, -6, -2, -11, -7, 18)
        .lineStyle(3, secondary, 0.95)
        .strokeTriangle(-19, -6, -2, -11, -7, 18)
        .lineBetween(-13, 1, -5, 1)
        .lineBetween(-9, -3, -9, 7);
    } else if (mystic) {
      body
        .fillStyle(color, 0.9)
        .fillTriangle(-17, 18, 0, -17, 17, 18)
        .fillStyle(0xe9f7ff, 0.92)
        .fillCircle(0, -11, 8)
        .lineStyle(4, secondary, 0.95)
        .lineBetween(14, -15, 14, 18)
        .fillStyle(secondary, 0.95)
        .fillCircle(14, -18, 7)
        .lineStyle(2, 0xffffff, 0.75)
        .strokeCircle(0, 4, 7);
    } else {
      body
        .fillStyle(color, 0.94)
        .fillRoundedRect(-14, -7, 28, 26, 6)
        .fillStyle(secondary, 0.95)
        .fillCircle(0, -13, 9)
        .fillStyle(0x0b1725, 0.88)
        .fillRect(-9, -16, 18, 6)
        .lineStyle(2, secondary, 0.9)
        .strokeRoundedRect(-10, -3, 20, 15, 4)
        .lineBetween(-5, 4, 5, 4);
    }
    return body;
  }

  private drawHeroHealth(hero: HeroSprite, ratio: number) {
    const clamped = Math.max(0, Math.min(1, ratio));
    hero.health.clear();
    hero.health
      .fillStyle(0x030811, 0.9)
      .fillRoundedRect(-24, -39, 48, 7, 3)
      .fillStyle(clamped > 0.35 ? 0x65e392 : 0xff6f72, 0.95)
      .fillRoundedRect(-22, -37, 44 * clamped, 3, 2);
  }

  private drawTowerShape(definition: TowerDefinition, level: number, branchId = "") {
    const base = this.add.graphics();
    const presentation = resolveTowerPresentation(definition, branchId, this.isBlockingTower(definition));
    // A specialised tower carries its branch badge in a brighter accent, so the
    // two level-three builds of the same tower are no longer identical.
    drawTowerBody(base, presentation, level, definition.color, 0xffffff);
    return [base];
  }

  // ---------------------------------------------------------------- reporting

  private emitHUD(force = false) {
    const now = this.kernel.time;
    if (!force && now - this.lastHUD < 180) return;
    this.lastHUD = now;
    const state = this.kernel.status();
    const selected = this.selectedSpot
      ? this.kernel.towerAt(this.selectedSpot)
      : undefined;
    this.options.onHUD({
      status: this.paused && !this.completed ? "paused" : state.status,
      gold: state.gold,
      lives: state.lives,
      wave: state.wave,
      totalWaves: state.totalWaves,
      kills: state.kills,
      heroLevel: state.heroLevel,
      heroHp: state.heroHp,
      heroMaxHp: state.heroMaxHp,
      heroAlive: state.heroAlive,
      heroRespawn: state.heroRespawn,
      nextWaveIn: state.nextWaveIn,
      selectedSpot: this.selectedSpot,
      selectedTower: selected
        ? {
            type: this.options.config.towers[selected.def]?.id ?? "",
            level: selected.level,
            branch: selected.branch || undefined,
            profile: selected.profile || undefined,
            targeting: selected.targeting,
          }
        : undefined,
      skillCooldowns: state.skillCooldowns,
      speed: this.battleSpeed,
    });
  }

  private async finish(victory: boolean) {
    if (this.completed) return;
    this.completed = true;
    const outcome = this.kernel.outcome();
    const local = calculateLocalResult(
      {
        victory,
        lives: outcome.lives,
        kills: outcome.kills,
        waves: outcome.waves_completed,
        gold: outcome.gold,
        duration_ms: outcome.duration_ms,
        difficulty: this.options.difficulty,
        mode: this.options.stage.mode,
      },
      this.options.config.balance,
    );
    const stats: BattleStats = {
      stage_id: this.options.stage.id,
      mode: this.options.stage.mode,
      difficulty: this.options.difficulty,
      duration_ms: outcome.duration_ms,
      lives: outcome.lives,
      gold: outcome.gold,
      earned_gold: outcome.earned_gold,
      spent_gold: outcome.spent_gold,
      sold_gold: outcome.sold_gold,
      kills: outcome.kills,
      waves: outcome.waves_completed,
      waves_completed: outcome.waves_completed,
      escaped: outcome.escaped,
      spawned: outcome.spawned,
      defeated_by_enemy: outcome.defeated_by_enemy,
      escaped_by_enemy: outcome.escaped_by_enemy,
      spawned_by_enemy: outcome.spawned_by_enemy,
      hero_id: this.options.hero.id,
      hero_level: outcome.hero_level,
      content_version: this.options.config.contentVersion,
      balance_version: this.options.config.balanceVersion,
      stage_version: this.options.stage.version,
      asset_version: this.options.config.assetVersion,
      ledger: this.recorder.truncated
        ? undefined
        : this.recorder.build(this.configDigest, outcome.ticks),
    };
    this.emitHUD(true);
    const result = { ...stats, victory, ...local };
    try {
      await this.options.onTelemetry("realmguard.battle.complete", {
        ...stats,
        ledger: undefined,
        victory,
        local_score: local.score,
        local_stars: local.stars,
      });
      this.options.onComplete(result);
    } catch (cause) {
      this.options.onCompleteError(
        result,
        cause instanceof Error
          ? cause
          : new Error("전투 검증 로그를 전송하지 못했습니다."),
      );
    }
  }
}

export function mountRealmGuard(
  parent: HTMLElement,
  options: MountOptions,
): RealmSceneController {
  const scene = new RealmGuardBattleScene(options);
  const game = new Phaser.Game({
    type: Phaser.AUTO,
    parent,
    width: WIDTH,
    height: HEIGHT,
    backgroundColor: "#101a27",
    scene,
    transparent: false,
    input: { mouse: { preventDefaultWheel: true }, touch: { capture: true } },
    render: { antialias: true, roundPixels: true },
    scale: {
      mode: Phaser.Scale.FIT,
      autoCenter: Phaser.Scale.CENTER_BOTH,
      width: WIDTH,
      height: HEIGHT,
    },
  });
  return {
    command: (command) => scene.command(command),
    destroy: () => game.destroy(true),
  };
}
