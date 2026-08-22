import { describe, expect, it } from "vitest";
import {
  DEFENSE_OPTIONAL_TELEMETRY_LIMIT,
  adaptDefenseRuntimeTelemetry,
  defenseEducationTrigger,
  defenseAttestationPayload,
  isAllowedDefenseTelemetry,
  isRequiredDefenseTelemetry,
  openDefensePromptBeforeTelemetry,
  shouldPauseDefensePrompt,
} from "./telemetry";

describe("Defense Series telemetry ledger", () => {
  it("recognizes only server attestation events as required", () => {
    expect(isRequiredDefenseTelemetry("defense.battle.ready")).toBe(true);
    expect(isRequiredDefenseTelemetry("defense.wave.complete")).toBe(true);
    expect(isRequiredDefenseTelemetry("defense.education.apply")).toBe(true);
    expect(isRequiredDefenseTelemetry("defense.hero.move")).toBe(false);
    expect(isRequiredDefenseTelemetry("other.defense.battle.ready")).toBe(
      false,
    );
    expect(isAllowedDefenseTelemetry("defense.hero.move")).toBe(true);
    expect(isAllowedDefenseTelemetry("defense.hero.skill")).toBe(false);
    expect(DEFENSE_OPTIONAL_TELEMETRY_LIMIT).toBe(128);
  });

  it("creates a one-based ordered attestation envelope", () => {
    expect(
      defenseAttestationPayload("11111111-1111-4111-8111-111111111111", 1, {
        wave: 1,
      }),
    ).toMatchObject({
      battle_id: "11111111-1111-4111-8111-111111111111",
      sequence: 1,
      wave: 1,
      client_event_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
      occurred_at: expect.any(String),
    });
  });

  it("maps Realm runtime snapshots to the common Defense ledger contract", () => {
    expect(
      adaptDefenseRuntimeTelemetry(
        "realmguard.battle.ready",
        { stage_id: "stage-1", lives: 20, gold: 250 },
        "0.3.0",
        "policy-1",
      ),
    ).toEqual({
      event: "defense.battle.ready",
      data: {
        stage_id: "stage-1",
        health: 20,
        resource: 250,
        content_version: "0.3.0",
        policy_version: "policy-1",
      },
    });
    expect(
      adaptDefenseRuntimeTelemetry(
        "realmguard.battle.complete",
        {
          earned_gold: 40,
          spent_gold: 30,
          sold_gold: 5,
          content_version: "stale-client-version",
          policy_version: "stale-client-policy",
        },
        "0.3.0",
        "policy-1",
      ),
    ).toMatchObject({
      event: "defense.battle.complete",
      data: {
        earned_resource: 40,
        spent_resource: 30,
        sold_resource: 5,
        content_version: "0.3.0",
        policy_version: "policy-1",
      },
    });
  });

  it("opens a battle or wave prompt synchronously before queuing telemetry", async () => {
    const calls: string[] = [];
    const trigger = defenseEducationTrigger("defense.wave.start", { wave: 3 });
    const scheduled = openDefensePromptBeforeTelemetry(
      trigger,
      (value) => {
        calls.push(`open:${value}`);
        return true;
      },
      async () => {
        calls.push("queue");
      },
    );
    expect(scheduled.opened).toBe(true);
    expect(calls).toEqual(["open:wave_3", "queue"]);
    await scheduled.pending;
    expect(defenseEducationTrigger("defense.battle.ready", {})).toBe(
      "battle_start",
    );
    expect(defenseEducationTrigger("defense.wave.complete", {})).toBe(
      undefined,
    );
    expect(
      defenseEducationTrigger("defense.wave.start", { wave: 3 }, true),
    ).toBeUndefined();
    expect(shouldPauseDefensePrompt(true, false, false, "ready")).toBe(false);
    expect(shouldPauseDefensePrompt(true, false, true, "ready")).toBe(true);
    expect(shouldPauseDefensePrompt(true, true, true, "ready")).toBe(false);
  });
});
