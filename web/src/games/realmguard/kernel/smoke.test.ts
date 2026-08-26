import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { normalizeRealmGuardConfig } from "../api";
import { calculateLocalResult } from "../content";
import type { RealmDifficulty, RealmGuardConfig } from "../types";
import { kernelDigest, projectKernelConfig, withLoadout } from "./config";
import { BattleKernel } from "./kernel";
import { KERNEL_RULES_VERSION } from "./ledger";
import type { KernelCommand } from "./ledger";

/**
 * The release smoke posts a real battle, so it needs a ledger the server will
 * replay to a known outcome plus the telemetry milestones that battle would
 * have streamed. Hand-written numbers can no longer satisfy a replay, so the
 * client's own kernel generates both from the committed snapshot of the
 * canonical published content.
 */
const CONFIG_PATH = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../../internal/api/testdata/realmguard_published_config.json",
);
const FIXTURE_PATH = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../../scripts/testdata/realmguard-smoke.json",
);

const STAGE_ID = "stage-1";
const DIFFICULTY: RealmDifficulty = "veteran";
const HERO_ID = "aerin";
/**
 * The loadout a fresh account actually has on stage 1: meteor is the only skill
 * unlocked before stage 4. The smoke posts the battle a new player plays, which
 * is the one the server has to be able to reproduce.
 */
const SKILL_IDS = ["meteor"];

/**
 * An undefended battle is the shortest honest defeat the only stage a fresh
 * account can play will produce. The tower is built, upgraded and sold inside
 * the first 150ms so the smoke exercises the whole spend ledger without
 * changing the fight.
 */
const COMMANDS: KernelCommand[] = [
  { tick: 1, op: "build", spot: "s1-spot-4", tower: "sunspire" },
  { tick: 2, op: "upgrade", spot: "s1-spot-4" },
  { tick: 3, op: "sell", spot: "s1-spot-4" },
];

const REQUIRED_EVENTS = new Set([
  "realmguard.wave.start",
  "realmguard.wave.complete",
  "realmguard.tower.build",
  "realmguard.tower.upgrade",
  "realmguard.tower.sell",
]);

function publishedConfig(): RealmGuardConfig {
  return normalizeRealmGuardConfig(
    JSON.parse(readFileSync(CONFIG_PATH, "utf8")) as unknown,
  );
}

function buildFixture() {
  const config = publishedConfig();
  const stage = config.stages.find((item) => item.id === STAGE_ID)!;
  const projection = projectKernelConfig(withLoadout(config, SKILL_IDS), stage, DIFFICULTY, HERO_ID);
  const kernel = new BattleKernel(projection, 1);
  const telemetry: Array<{ event: string; data: Record<string, unknown> }> = [
    {
      event: "realmguard.battle.ready",
      data: { stage_id: stage.id, difficulty: DIFFICULTY, hero_id: HERO_ID },
    },
  ];
  const collect = () => {
    for (const event of kernel.drainEvents())
      if (event.k === "telemetry" && REQUIRED_EVENTS.has(event.event))
        telemetry.push({ event: event.event, data: event.data ?? {} });
  };
  let cursor = 0;
  for (let tick = 0; tick < 20_000 && !kernel.finished; tick += 1) {
    while (cursor < COMMANDS.length && COMMANDS[cursor].tick <= tick) {
      kernel.apply(COMMANDS[cursor]);
      cursor += 1;
    }
    collect();
    if (kernel.finished) break;
    kernel.tick();
  }
  collect();
  const outcome = kernel.outcome();
  const local = calculateLocalResult(
    {
      victory: outcome.victory,
      lives: outcome.lives,
      kills: outcome.kills,
      waves: outcome.waves_completed,
      gold: outcome.gold,
      duration_ms: outcome.duration_ms,
      difficulty: DIFFICULTY,
      mode: stage.mode,
    },
    config.balance,
  );
  telemetry.push({
    event: "realmguard.battle.complete",
    data: {
      stage_id: stage.id,
      mode: stage.mode,
      difficulty: DIFFICULTY,
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
      hero_id: HERO_ID,
      hero_level: outcome.hero_level,
      content_version: config.contentVersion,
      stage_version: stage.version,
      balance_version: config.balanceVersion,
      asset_version: config.assetVersion,
      victory: outcome.victory,
      local_score: local.score,
      local_stars: local.stars,
    },
  });
  return {
    stage_id: stage.id,
    mode: stage.mode,
    difficulty: DIFFICULTY,
    hero_id: HERO_ID,
    account_hero_level: 1,
    content_version: config.contentVersion,
    stage_version: stage.version,
    balance_version: config.balanceVersion,
    asset_version: config.assetVersion,
    ledger: {
      rules_version: KERNEL_RULES_VERSION,
      config_digest: kernelDigest(projection),
      skill_ids: SKILL_IDS,
      ticks: outcome.ticks,
      commands: COMMANDS,
    },
    outcome,
    telemetry,
  };
}

describe("release smoke fixture", () => {
  const fixture = buildFixture();

  it("stays in step with the committed smoke battle", () => {
    const serialized = `${JSON.stringify(fixture, null, 2)}\n`;
    if (process.env.UPDATE_KERNEL_VECTORS === "1") {
      mkdirSync(dirname(FIXTURE_PATH), { recursive: true });
      writeFileSync(FIXTURE_PATH, serialized);
    }
    expect(readFileSync(FIXTURE_PATH, "utf8")).toBe(serialized);
  });

  it("ends in an honest defeat with a complete spend ledger", () => {
    expect(fixture.outcome.victory).toBe(false);
    expect(fixture.outcome.lives).toBe(0);
    expect(fixture.outcome.escaped).toBeGreaterThan(0);
    expect(fixture.outcome.spent_gold).toBeGreaterThan(0);
    expect(fixture.outcome.sold_gold).toBeGreaterThan(0);
    expect(fixture.outcome.waves_completed).toBeGreaterThan(0);
  });

  it("streams the milestones the server attestation requires", () => {
    const events = fixture.telemetry.map((item) => item.event);
    expect(events[0]).toBe("realmguard.battle.ready");
    expect(events.at(-1)).toBe("realmguard.battle.complete");
    expect(events.filter((event) => event === "realmguard.wave.start")).toHaveLength(
      fixture.outcome.waves_completed + 1,
    );
    expect(events.filter((event) => event === "realmguard.wave.complete")).toHaveLength(
      fixture.outcome.waves_completed,
    );
    expect(events).toContain("realmguard.tower.build");
    expect(events).toContain("realmguard.tower.upgrade");
    expect(events).toContain("realmguard.tower.sell");
    // The optional-event budget is 128; the smoke must stay inside it.
    expect(events.length).toBeLessThan(120);
  });
});
