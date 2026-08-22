import type { DefenseProgress } from "./types";
import type { HeroDefinition, RealmStage } from "../realmguard/types";

export function resolveDefenseProgress(
  base: DefenseProgress | undefined,
  resultProgress: DefenseProgress | undefined,
) {
  return resultProgress ?? base;
}

export function isDefenseHeroUnlocked(
  hero: HeroDefinition,
  stage: Pick<RealmStage, "number"> | undefined,
) {
  return Boolean(stage && stage.number >= (hero.unlockStage ?? 1));
}
