export interface LevelProgress {
  level: number;
  /** XP earned since reaching the current level. */
  xpIntoLevel: number;
  /** Total XP this level spans. */
  xpForLevel: number;
  /** XP still needed for the next level. */
  xpToNext: number;
  /** Progress through the current level, 0–100. */
  percent: number;
}

/**
 * Mirrors the server's levelForXP curve, where each level costs roughly twice
 * the previous one: the thresholds are 100, 300, 700, 1500, 3100 …
 *
 * The portal used to draw the progress bar as `xp % 100`, which assumed a flat
 * 100 XP per level. That reads correctly only at level 1 and drifts further off
 * with every level — a player 87% of the way to level 5 saw an empty bar.
 */
export function levelProgress(xp: number | undefined): LevelProgress {
  const earned = Number.isFinite(xp) && (xp as number) > 0 ? Math.floor(xp as number) : 0;
  let level = 1;
  let reached = 0;
  let next = 100;
  while (earned >= next) {
    level += 1;
    reached = next;
    next = next * 2 + 100;
  }
  const xpForLevel = next - reached;
  const xpIntoLevel = earned - reached;
  return {
    level,
    xpIntoLevel,
    xpForLevel,
    xpToNext: next - earned,
    percent: Math.min(100, Math.max(0, Math.round((xpIntoLevel / xpForLevel) * 100))),
  };
}
