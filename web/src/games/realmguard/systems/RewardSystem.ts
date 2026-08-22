import type { RealmBalance, RealmStage } from "../types";

export function calculateStartingGold(
  stage: Pick<RealmStage, "startingGold">,
  balance: RealmBalance,
  difficulty: keyof RealmBalance["difficulties"],
) {
  return Math.round(stage.startingGold * balance.difficulties[difficulty].gold);
}

/** Preserve penalty debt so later income first repays the authoritative spend. */
export function applyResourceDelta(current: number, delta: number) {
  return current + Math.round(delta);
}

export function visibleResource(current: number) {
  return Math.max(0, current);
}

export function calculateResult(
  input: {
    victory: boolean;
    lives: number;
    waves: number;
    gold: number;
    duration_ms?: number;
    difficulty: keyof RealmBalance["difficulties"];
    mode: "campaign" | "endless";
  },
  balance: RealmBalance,
) {
  const clearTimeBonus =
    input.mode === "campaign" && input.victory
      ? Math.max(
          0,
          balance.parTimeSeconds -
            (input.duration_ms ?? balance.parTimeSeconds * 1000) / 1000,
        ) * balance.clearTimeBonusPerSecond
      : 0;
  const endlessWaveBonus =
    input.mode === "endless" ? input.waves * balance.endlessWaveBonus : 0;
  const score = Math.max(
    0,
    Math.round(
      input.lives * 1000 +
        input.gold * 10 +
        clearTimeBonus +
        endlessWaveBonus +
        balance.difficultyBonus[input.difficulty],
    ),
  );
  const stars =
    !input.victory || input.mode === "endless"
      ? 0
      : input.lives >= 18
        ? 3
        : input.lives >= 10
          ? 2
          : 1;
  return { score, stars };
}
