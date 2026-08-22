import { describe, expect, it } from "vitest";
import { DEFAULT_REALMGUARD_CONFIG } from "../content";
import {
  effectiveDamage,
  movementMultiplier,
  towerEffectivenessMultiplier,
} from "./CombatMath";
import {
  applyResourceDelta,
  calculateResult,
  calculateStartingGold,
  visibleResource,
} from "./RewardSystem";
import { targetComparator } from "./TargetSystem";
import {
  canCompleteWave,
  canResolveCampaignVictory,
  expandWave,
} from "./WaveSystem";

describe("RealmGuard deterministic systems", () => {
  it("expands two lanes and parallel spawn groups deterministically", () => {
    const plan = expandWave([
      { enemy: "mireling", count: 2, interval: 1, pathIndex: 0 },
      {
        enemy: "glintfox",
        count: 2,
        interval: 0.5,
        delay: 0.25,
        pathIndex: 1,
        parallel: true,
      },
    ]);
    expect(plan.map((item) => [item.enemy, item.at, item.pathIndex])).toEqual([
      ["mireling", 0, 0],
      ["glintfox", 250, 1],
      ["glintfox", 750, 1],
      ["mireling", 1000, 0],
    ]);
  });

  it("never completes a wave after a defeat has already completed the battle", () => {
    expect(canCompleteWave(true, true, 0, 0)).toBe(false);
    expect(canCompleteWave(false, true, 0, 0)).toBe(true);
    expect(canResolveCampaignVictory(true, "campaign", 5, 5)).toBe(false);
    expect(canResolveCampaignVictory(false, "campaign", 5, 5)).toBe(true);
  });

  it("orders first, last and closest targets", () => {
    const origin = { x: 0, y: 0 };
    const a = { pathProgress: 1, hp: 20, x: 30, y: 0 };
    const b = { pathProgress: 3, hp: 50, x: 80, y: 0 };
    expect(targetComparator("first", origin, a, b)).toBeGreaterThan(0);
    expect(targetComparator("last", origin, a, b)).toBeLessThan(0);
    expect(targetComparator("closest", origin, a, b)).toBeLessThan(0);
  });

  it("applies advanced enemy modifiers", () => {
    expect(effectiveDamage(100, "arcane", 0, new Set(["magic_resist"]))).toBe(
      52,
    );
    expect(
      effectiveDamage(100, "magic", 0.5, new Set(["magic_resist", "armored"])),
    ).toBe(52);
    expect(
      effectiveDamage(100, "true", 0.5, new Set(["magic_resist", "armored"])),
    ).toBe(100);
    expect(movementMultiplier(new Set(["berserk"]), 0.3, false, 1, false)).toBe(
      1.5,
    );
    expect(
      movementMultiplier(new Set(["immune_stun"]), 1, true, 0.2, false),
    ).toBe(1);
  });

  it("applies a data-driven counter only to matching threat types", () => {
    const tower = {
      effectiveAgainst: ["web_attack"],
      effectiveMultiplier: 1.8,
    };
    expect(
      towerEffectivenessMultiplier(tower, { threatType: "web_attack" }),
    ).toBe(1.8);
    expect(towerEffectivenessMultiplier(tower, { threatType: "malware" })).toBe(
      1,
    );
  });

  it("mirrors server economy and scoring", () => {
    const balance = DEFAULT_REALMGUARD_CONFIG.balance;
    const stage = DEFAULT_REALMGUARD_CONFIG.stages[0];
    expect(calculateStartingGold(stage, balance, "casual")).toBe(
      Math.round(stage.startingGold * 1.18),
    );
    expect(
      calculateResult(
        {
          victory: false,
          lives: 20,
          waves: 7,
          gold: 100,
          difficulty: "normal",
          mode: "endless",
        },
        balance,
      ).score,
    ).toBe(33_000);
    const debt = applyResourceDelta(20, -50);
    expect(visibleResource(debt)).toBe(0);
    expect(visibleResource(applyResourceDelta(debt, 100))).toBe(70);
  });
});
