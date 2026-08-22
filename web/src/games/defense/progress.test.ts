import { describe, expect, it } from "vitest";
import { isDefenseHeroUnlocked, resolveDefenseProgress } from "./progress";
import type { DefenseProgress } from "./types";

describe("Defense result progress", () => {
  it("uses authoritative result progress immediately so the next stage unlocks", () => {
    const item = {
      stage_id: "stage-2",
      difficulty: "normal" as const,
      unlocked: false,
      completed: false,
      best_score: 0,
      best_learning_score: 0,
      attempts: 0,
      completions: 0,
      total_playtime_ms: 0,
    };
    const base: DefenseProgress = {
      summary: {
        completed_stages: 0,
        total_stars: 0,
        total_playtime_ms: 0,
        campaign_complete: false,
      },
      items: [item],
    };
    const result: DefenseProgress = {
      summary: { ...base.summary, completed_stages: 1, total_stars: 3 },
      items: [{ ...item, unlocked: true }],
    };
    expect(resolveDefenseProgress(base, result)?.items[0].unlocked).toBe(true);
  });

  it("unlocks a hero only on its configured campaign stage", () => {
    const hero = { unlockStage: 3 } as Parameters<
      typeof isDefenseHeroUnlocked
    >[0];
    expect(isDefenseHeroUnlocked(hero, { number: 2 })).toBe(false);
    expect(isDefenseHeroUnlocked(hero, { number: 3 })).toBe(true);
  });
});
