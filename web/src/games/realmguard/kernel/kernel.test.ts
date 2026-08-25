import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { DEFAULT_REALMGUARD_CONFIG } from "../content";
import type { RealmDifficulty } from "../types";
import { canonicalJSON, kernelDigest, projectKernelConfig } from "./config";
import { BattleKernel, replayBattle } from "./kernel";
import type { KernelCommand } from "./ledger";
import type { KernelConfig, KernelOutcome } from "./types";

interface Vector {
  name: string;
  stage_id: string;
  difficulty: RealmDifficulty;
  hero_id: string;
  account_hero_level: number;
  ticks: number;
  config_digest: string;
  config: KernelConfig;
  commands: KernelCommand[];
  expected: KernelOutcome;
}

/**
 * Shared with the Go verifier. `UPDATE_KERNEL_VECTORS=1 npx vitest run` rewrites
 * the file; both suites then assert the same battles reach the same outcome, so
 * a rules change that only lands on one side fails the build.
 */
const VECTOR_PATH = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../../internal/battle/realmguard/testdata/vectors.json",
);

function build(
  stageId: string,
  difficulty: RealmDifficulty,
  heroId: string,
): KernelConfig {
  const stage = DEFAULT_REALMGUARD_CONFIG.stages.find((item) => item.id === stageId);
  if (!stage) throw new Error(`unknown stage ${stageId}`);
  return projectKernelConfig(DEFAULT_REALMGUARD_CONFIG, stage, difficulty, heroId);
}

const scenarios: Array<Omit<Vector, "config" | "config_digest" | "expected">> = [
  {
    name: "undefended-leak",
    stage_id: "stage-1",
    difficulty: "normal",
    hero_id: "aerin",
    account_hero_level: 1,
    ticks: 2400,
    commands: [],
  },
  {
    name: "held-line",
    stage_id: "stage-1",
    difficulty: "normal",
    hero_id: "aerin",
    account_hero_level: 1,
    ticks: 9000,
    commands: [
      { tick: 2, op: "build", spot: "s1-spot-2", tower: "sunspire" },
      { tick: 4, op: "build", spot: "s1-spot-4", tower: "sunspire" },
      { tick: 6, op: "build", spot: "s1-spot-3", tower: "windward" },
      { tick: 10, op: "wave" },
      { tick: 400, op: "upgrade", spot: "s1-spot-2" },
      { tick: 900, op: "build", spot: "s1-spot-6", tower: "runebloom" },
      { tick: 1400, op: "upgrade", spot: "s1-spot-2", branch: "dawn_volley" },
      { tick: 1500, op: "target", spot: "s1-spot-4", mode: "strong" },
      { tick: 1800, op: "hero", x: 620, y: 420 },
      { tick: 2400, op: "skill", skill: "freeze" },
      { tick: 3000, op: "skill", skill: "meteor" },
      { tick: 3001, op: "meteor", x: 520, y: 430 },
      { tick: 4200, op: "sell", spot: "s1-spot-3" },
      { tick: 4400, op: "build", spot: "s1-spot-3", tower: "stonepulse" },
      { tick: 5200, op: "skill", skill: "reinforcement" },
      { tick: 5201, op: "reinforce", x: 900, y: 300 },
    ],
  },
  {
    name: "boss-stage",
    stage_id: "stage-5",
    difficulty: "casual",
    hero_id: "brann",
    account_hero_level: 4,
    ticks: 26000,
    commands: [
      { tick: 1, op: "build", spot: "s5-spot-4", tower: "runebloom" },
      { tick: 2, op: "build", spot: "s5-spot-6", tower: "sunspire" },
      { tick: 5, op: "wave" },
      { tick: 1200, op: "upgrade", spot: "s5-spot-4" },
      { tick: 2400, op: "upgrade", spot: "s5-spot-4", branch: "null_petal" },
      { tick: 3600, op: "build", spot: "s5-spot-8", tower: "stonepulse" },
      { tick: 5000, op: "upgrade", spot: "s5-spot-8" },
      { tick: 6000, op: "upgrade", spot: "s5-spot-8", branch: "quake_drum" },
      { tick: 7000, op: "hero", x: 800, y: 260 },
      { tick: 9000, op: "build", spot: "s5-spot-2", tower: "sunspire" },
      { tick: 12000, op: "upgrade", spot: "s5-spot-2" },
      { tick: 15000, op: "upgrade", spot: "s5-spot-2", branch: "eagle_oath" },
      { tick: 20000, op: "skill", skill: "meteor" },
      { tick: 20001, op: "meteor", x: 820, y: 300 },
      { tick: 20400, op: "skill", skill: "freeze" },
    ],
  },
  {
    name: "barracks-stall",
    stage_id: "stage-5",
    difficulty: "casual",
    hero_id: "brann",
    account_hero_level: 4,
    ticks: 14000,
    commands: [
      { tick: 1, op: "build", spot: "s5-spot-4", tower: "runebloom" },
      { tick: 3, op: "build", spot: "s5-spot-2", tower: "windward" },
      { tick: 5, op: "wave" },
      { tick: 7000, op: "hero", x: 800, y: 260 },
    ],
  },
  {
    name: "veteran-endless",
    stage_id: "endless-rift",
    difficulty: "veteran",
    hero_id: "nyra",
    account_hero_level: 7,
    ticks: 12000,
    commands: [
      { tick: 1, op: "build", spot: "endless-spot-1", tower: "sunspire" },
      { tick: 2, op: "build", spot: "endless-spot-4", tower: "runebloom" },
      { tick: 3, op: "build", spot: "endless-spot-7", tower: "stonepulse" },
      { tick: 4, op: "build", spot: "endless-spot-3", tower: "windward" },
      { tick: 8, op: "wave" },
      { tick: 1500, op: "upgrade", spot: "endless-spot-4" },
      { tick: 2500, op: "upgrade", spot: "endless-spot-4", branch: "star_lattice" },
      { tick: 3500, op: "target", spot: "endless-spot-7", mode: "last" },
      { tick: 4500, op: "skill", skill: "freeze" },
      { tick: 6000, op: "hero", x: 640, y: 360 },
      { tick: 8000, op: "economy", gold: 400, lives: 0 },
      { tick: 8200, op: "build", spot: "endless-spot-6", tower: "sunspire" },
      { tick: 9000, op: "skill", skill: "reinforcement" },
      { tick: 9001, op: "reinforce", x: 700, y: 500 },
    ],
  },
  {
    name: "surrendered",
    stage_id: "stage-3",
    difficulty: "normal",
    hero_id: "aerin",
    account_hero_level: 1,
    ticks: 3000,
    commands: [
      { tick: 1, op: "build", spot: "s3-spot-1", tower: "sunspire" },
      { tick: 600, op: "defeat" },
    ],
  },
];

function materialize(): Vector[] {
  return scenarios.map((scenario) => {
    const config = build(scenario.stage_id, scenario.difficulty, scenario.hero_id);
    return {
      ...scenario,
      config,
      config_digest: kernelDigest(config),
      expected: replayBattle(
        config,
        scenario.commands,
        scenario.ticks,
        scenario.account_hero_level,
      ),
    };
  });
}

describe("BattleKernel", () => {
  const vectors = materialize();

  it("keeps the replay vectors in step with the Go verifier", () => {
    const serialized = `${JSON.stringify(vectors, null, 2)}\n`;
    if (process.env.UPDATE_KERNEL_VECTORS === "1") {
      mkdirSync(dirname(VECTOR_PATH), { recursive: true });
      writeFileSync(VECTOR_PATH, serialized);
    }
    expect(readFileSync(VECTOR_PATH, "utf8")).toBe(serialized);
  });

  it("replays a ledger to the same outcome every time", () => {
    for (const vector of vectors) {
      const again = replayBattle(
        vector.config,
        vector.commands,
        vector.ticks,
        vector.account_hero_level,
      );
      expect(again).toEqual(vector.expected);
    }
  });

  it("matches a stepped battle with the same input", () => {
    const vector = vectors.find((item) => item.name === "held-line")!;
    const kernel = new BattleKernel(vector.config, vector.account_hero_level);
    let cursor = 0;
    for (let tick = 0; tick < vector.ticks; tick += 1) {
      while (cursor < vector.commands.length && vector.commands[cursor].tick <= tick) {
        kernel.apply(vector.commands[cursor]);
        cursor += 1;
      }
      kernel.drainEvents();
      if (kernel.finished) break;
      kernel.tick();
    }
    expect(kernel.outcome()).toEqual(vector.expected);
  });

  it("plays a defended battle differently from an abandoned one", () => {
    const idle = vectors.find((item) => item.name === "undefended-leak")!;
    const held = vectors.find((item) => item.name === "held-line")!;
    expect(idle.expected.escaped).toBeGreaterThan(0);
    expect(held.expected.escaped).toBe(0);
    expect(held.expected.kills).toBeGreaterThan(idle.expected.kills * 4);
    expect(held.expected.lives).toBeGreaterThan(0);
    expect(held.expected.spent_gold).toBeGreaterThan(0);
    expect(held.expected.sold_gold).toBeGreaterThan(0);
  });

  it("stops the battle as soon as the player forfeits", () => {
    const vector = vectors.find((item) => item.name === "surrendered")!;
    expect(vector.expected.victory).toBe(false);
    expect(vector.expected.ticks).toBe(600);
  });

  it("ignores actions the player cannot afford", () => {
    const config = build("stage-1", "normal", "aerin");
    const affordable = replayBattle(
      config,
      [{ tick: 0, op: "build", spot: "s1-spot-1", tower: "sunspire" }],
      10,
    );
    const overspent = replayBattle(
      config,
      [
        { tick: 0, op: "build", spot: "s1-spot-1", tower: "sunspire" },
        { tick: 0, op: "build", spot: "s1-spot-2", tower: "stonepulse" },
        { tick: 0, op: "build", spot: "s1-spot-3", tower: "stonepulse" },
        { tick: 0, op: "build", spot: "s1-spot-4", tower: "stonepulse" },
        { tick: 0, op: "build", spot: "s1-spot-5", tower: "stonepulse" },
      ],
      10,
    );
    expect(affordable.spent_gold).toBe(75);
    expect(overspent.spent_gold).toBe(75 + 125);
  });

  it("digests only the numbers a battle depends on", () => {
    const config = build("stage-1", "normal", "aerin");
    const digest = kernelDigest(config);
    expect(digest).toMatch(/^[0-9a-f]{16}$/);
    expect(kernelDigest(structuredClone(config))).toBe(digest);
    const retuned = structuredClone(config);
    retuned.towers[0].damage += 1;
    expect(kernelDigest(retuned)).not.toBe(digest);
  });

  it("renders canonical numbers independently of the JSON encoder", () => {
    expect(canonicalJSON({ b: 1, a: 0.5 })).toBe('{"a":0.5,"b":1}');
    expect(canonicalJSON([-0, 0.1 + 0.2, 1 / 3])).toBe("[0,0.3,0.333333]");
  });
});
