import { mountRealmGuard } from "../../realmguard/RealmGuardScene";
import type {
  BattleHUD,
  HeroDefinition,
  RealmCommand,
  RealmDifficulty,
  RealmGuardConfig,
  RealmResult,
  RealmSceneController,
  RealmStage,
} from "../../realmguard/types";
import type { DefenseSlug } from "../types";
import { adaptDefenseRuntimeTelemetry } from "../telemetry";

export interface DefenseCoreOptions {
  slug: DefenseSlug;
  config: RealmGuardConfig;
  stage: RealmStage;
  difficulty: RealmDifficulty;
  hero: HeroDefinition;
  accountHeroLevel?: number;
  policyVersion: string;
  onHUD: (hud: BattleHUD) => void;
  onTelemetry: (
    event: string,
    data?: Record<string, unknown>,
  ) => void | Promise<void>;
  onComplete: (result: RealmResult) => void;
  onCompleteError: (result: RealmResult, error: Error) => void;
}

/**
 * Shared Defense Series runtime adapter. RealmGuard remains a first-party
 * content pack while all new packs use this stable engine boundary.
 */
export function mountDefenseCore(
  parent: HTMLElement,
  options: DefenseCoreOptions,
): RealmSceneController {
  return mountRealmGuard(parent, {
    config: options.config,
    stage: options.stage,
    difficulty: options.difficulty,
    hero: options.hero,
    presentationGame: options.slug,
    accountHeroLevel: options.accountHeroLevel ?? 1,
    onHUD: options.onHUD,
    onTelemetry: (event, data) => {
      const adapted = adaptDefenseRuntimeTelemetry(
        event,
        data ?? {},
        options.config.contentVersion,
        options.policyVersion,
      );
      return options.onTelemetry(adapted.event, adapted.data);
    },
    onComplete: options.onComplete,
    onCompleteError: options.onCompleteError,
  });
}

export type {
  BattleHUD,
  RealmCommand as DefenseCommand,
  RealmDifficulty,
  RealmGuardConfig as DefenseRuntimeConfig,
  RealmResult as DefenseBattleResult,
  RealmSceneController as DefenseCoreController,
  RealmStage as DefenseRuntimeStage,
};
