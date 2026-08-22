import { describe, expect, it } from "vitest";
import { AI_NEXUS_DEFENSE, CYBER_FORTRESS, OFFICE_GUARDIANS } from "./content";
import {
  aiDepletionDisposition,
  aiEscapedResourceCosts,
  aiResourcePercent,
  applyAIResourceCosts,
  applyAIResourceDeltas,
  buildAIResourceState,
  defenseTelemetryUsesAIResourceState,
  isAIResourceDepleted,
} from "./resource";

describe("Defense Series practice packs", () => {
  it("contains the agreed playable unit and stage counts", () => {
    expect([
      OFFICE_GUARDIANS.config.stages.length,
      OFFICE_GUARDIANS.config.towers.length,
      OFFICE_GUARDIANS.config.heroes.length,
    ]).toEqual([8, 6, 3]);
    expect(
      OFFICE_GUARDIANS.config.enemies.filter(
        (item) => !item.traits.includes("boss"),
      ),
    ).toHaveLength(10);
    expect(
      OFFICE_GUARDIANS.config.enemies.filter((item) =>
        item.traits.includes("boss"),
      ),
    ).toHaveLength(2);
    expect(CYBER_FORTRESS.config.stages).toHaveLength(10);
    expect(CYBER_FORTRESS.config.towers).toHaveLength(8);
    expect(
      CYBER_FORTRESS.config.enemies.filter(
        (item) => !item.traits.includes("boss"),
      ),
    ).toHaveLength(15);
    expect(
      CYBER_FORTRESS.config.enemies.filter((item) =>
        item.traits.includes("boss"),
      ),
    ).toHaveLength(3);
    expect(AI_NEXUS_DEFENSE.config.stages).toHaveLength(10);
    expect(AI_NEXUS_DEFENSE.config.towers).toHaveLength(10);
    expect(AI_NEXUS_DEFENSE.config.heroes).toHaveLength(5);
    expect(
      AI_NEXUS_DEFENSE.config.enemies.filter(
        (item) => !item.traits.includes("boss"),
      ),
    ).toHaveLength(15);
    expect(
      AI_NEXUS_DEFENSE.config.enemies.filter((item) =>
        item.traits.includes("boss"),
      ),
    ).toHaveLength(4);
    expect(AI_NEXUS_DEFENSE.modelProfiles?.map((item) => item.id)).toEqual([
      "small",
      "medium",
      "large",
      "reasoning",
      "vision",
    ]);
  });

  it("keeps client practice packs free of answer keys", () => {
    expect(CYBER_FORTRESS.education).toEqual([]);
    expect(CYBER_FORTRESS.events).toEqual([]);
    expect(AI_NEXUS_DEFENSE.education).toEqual([]);
  });

  it("serializes AI resource headroom as a conserved ledger", () => {
    const state = buildAIResourceState(AI_NEXUS_DEFENSE, {
      compute: 800,
      token: 750,
      trust: 88,
      latency: 70,
    });
    expect(state).toMatchObject({
      compute: { start: 1000, spent: 200, remaining: 800 },
      latency: { start: 100, spent: 30, remaining: 70 },
    });
    expect(
      isAIResourceDepleted({ compute: 1, token: 1, trust: 0, latency: 1 }),
    ).toBe(true);
    const exactZero = applyAIResourceCosts(
      AI_NEXUS_DEFENSE,
      { compute: 30, token: 40, trust: 100, latency: 100 },
      { compute: 30 },
    );
    expect(exactZero.compute).toBe(0);
    expect(isAIResourceDepleted(exactZero)).toBe(true);
    expect(aiDepletionDisposition(exactZero, true)).toBe("defer");
    expect(aiDepletionDisposition(exactZero, false)).toBe("defeat");
    expect(
      applyAIResourceCosts(
        AI_NEXUS_DEFENSE,
        { compute: 1000, token: 1000, trust: 100, latency: 100 },
        { compute: 30, trust: 8, latency: 4 },
      ),
    ).toEqual({ compute: 970, token: 1000, trust: 92, latency: 96 });
    expect(
      applyAIResourceDeltas(
        AI_NEXUS_DEFENSE,
        { compute: 970, token: 1000, trust: 92, latency: 96 },
        { trust: 20, latency: -5 },
      ),
    ).toEqual({ compute: 970, token: 1000, trust: 100, latency: 91 });
    expect(
      aiResourcePercent(
        {
          ...AI_NEXUS_DEFENSE,
          resourceRules: {
            ...AI_NEXUS_DEFENSE.resourceRules!,
            compute_start: 2000,
          },
        },
        "compute",
        500,
      ),
    ).toBe(25);
    const enemy = AI_NEXUS_DEFENSE.config.enemies[0];
    expect(
      aiEscapedResourceCosts(AI_NEXUS_DEFENSE, { [enemy.id]: 2 }, {}).costs,
    ).toMatchObject({
      trust:
        (AI_NEXUS_DEFENSE.resourceRules!.escaped_trust_cost +
          enemy.resourceEffect!.trust!) *
        2,
      latency:
        (AI_NEXUS_DEFENSE.resourceRules!.escaped_latency_cost +
          enemy.resourceEffect!.latency!) *
        2,
    });
    expect(defenseTelemetryUsesAIResourceState("defense.tower.build")).toBe(
      true,
    );
    expect(defenseTelemetryUsesAIResourceState("defense.tower.upgrade")).toBe(
      false,
    );
  });
});
