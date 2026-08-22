import {
  createRealmGuardUUID,
  retryRealmGuardTelemetry,
} from "../realmguard/telemetry";

export const DEFENSE_OPTIONAL_TELEMETRY_LIMIT = 128;
const requiredEvents = new Set([
  "defense.battle.ready",
  "defense.wave.start",
  "defense.wave.complete",
  "defense.tower.build",
  "defense.tower.upgrade",
  "defense.tower.sell",
  "defense.education.apply",
  "defense.battle.complete",
]);
const optionalEvents = new Set([
  "game.pause",
  "game.resume",
  "defense.skill.cast",
  "defense.hero.move",
  "defense.education.prompt",
]);

export function isRequiredDefenseTelemetry(event: string) {
  return requiredEvents.has(event);
}

export function isAllowedDefenseTelemetry(event: string) {
  return requiredEvents.has(event) || optionalEvents.has(event);
}

export function adaptDefenseRuntimeTelemetry(
  event: string,
  data: Record<string, unknown>,
  contentVersion: string,
  policyVersion: string,
) {
  const normalized: Record<string, unknown> = { ...data };
  const names: Record<string, string> = {
    lives: "health",
    gold: "resource",
    earned_gold: "earned_resource",
    spent_gold: "spent_resource",
    sold_gold: "sold_resource",
  };
  for (const [from, to] of Object.entries(names))
    if (from in normalized) {
      normalized[to] = normalized[from];
      delete normalized[from];
    }
  if (
    event === "realmguard.battle.ready" ||
    event === "realmguard.battle.complete"
  ) {
    normalized.content_version = contentVersion;
    normalized.policy_version = policyVersion;
  }
  return {
    event: event.replace(/^realmguard\./, "defense."),
    data: normalized,
  };
}

export function defenseEducationTrigger(
  event: string,
  data: Record<string, unknown>,
  terminal = false,
) {
  if (event === "defense.battle.ready") return "battle_start";
  if (event === "defense.wave.start" && !terminal)
    return `wave_${Number(data.wave ?? 0)}`;
  return undefined;
}

/** Open and pause locally before starting the network ledger operation. */
export function openDefensePromptBeforeTelemetry<T>(
  trigger: string | undefined,
  openPrompt: (value: string) => boolean,
  queue: () => Promise<T>,
) {
  const opened = trigger ? openPrompt(trigger) : false;
  return { opened, pending: queue() };
}

export function shouldPauseDefensePrompt(
  promptOpen: boolean,
  eventPauseApplied: boolean,
  controllerReady: boolean,
  battleStatus: string,
) {
  return (
    promptOpen &&
    !eventPauseApplied &&
    controllerReady &&
    battleStatus !== "paused"
  );
}

export function defenseAttestationPayload(
  battleId: string,
  sequence: number,
  data: Record<string, unknown> = {},
) {
  return {
    ...data,
    battle_id: battleId,
    sequence,
    client_event_id: createRealmGuardUUID(),
    occurred_at: new Date().toISOString(),
  };
}

export {
  createRealmGuardUUID as createDefenseUUID,
  retryRealmGuardTelemetry as retryDefenseTelemetry,
};
