import { describe, expect, it } from 'vitest';
import { levelProgress } from './level';

/** Mirrors internal/api/content.go levelForXP so the two cannot drift apart. */
function serverLevelForXP(xp: number): number {
  let level = 1;
  let threshold = 100;
  while (xp >= threshold) {
    level += 1;
    threshold = threshold * 2 + 100;
  }
  return level;
}

describe('levelProgress', () => {
  it('agrees with the server curve at and around every threshold', () => {
    for (const xp of [0, 1, 99, 100, 101, 299, 300, 301, 699, 700, 1499, 1500, 3099, 3100, 50_000]) {
      expect(levelProgress(xp).level).toBe(serverLevelForXP(xp));
    }
  });

  it('measures progress inside the current level, not modulo 100', () => {
    // 250 XP is level 2, three quarters of the way from 100 to 300.
    expect(levelProgress(250)).toMatchObject({ level: 2, xpIntoLevel: 150, xpForLevel: 200, xpToNext: 50, percent: 75 });
    // 1400 XP is level 4 and nearly level 5; the old `xp % 100` drew this as 0%.
    expect(levelProgress(1400)).toMatchObject({ level: 4, xpIntoLevel: 700, xpForLevel: 800, xpToNext: 100, percent: 88 });
  });

  it('starts empty and never leaves the 0–100 range', () => {
    expect(levelProgress(0)).toMatchObject({ level: 1, xpIntoLevel: 0, xpToNext: 100, percent: 0 });
    for (const xp of [0, 99, 100, 12_345, 10_000_000]) {
      const { percent } = levelProgress(xp);
      expect(percent).toBeGreaterThanOrEqual(0);
      expect(percent).toBeLessThanOrEqual(100);
    }
  });

  it('treats missing or nonsense XP as none earned', () => {
    for (const xp of [undefined, Number.NaN, -50]) {
      expect(levelProgress(xp as number)).toMatchObject({ level: 1, xpIntoLevel: 0, percent: 0 });
    }
  });
});
