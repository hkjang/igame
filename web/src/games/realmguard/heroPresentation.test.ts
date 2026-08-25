import { describe, expect, it } from "vitest";
import { resolveHeroPresentation } from "./heroPresentation";

describe("resolveHeroPresentation", () => {
  it("resolves the authored RealmGuard roster", () => {
    expect(resolveHeroPresentation("aerin")).toMatchObject({
      id: "aerin",
      game: "realmguard",
      motif: "bow",
      build: "agile",
      headgear: "hood",
      known: true,
    });
    expect(resolveHeroPresentation("brann").motif).toBe("shield");
    expect(resolveHeroPresentation("nyra").motif).toBe("staff");
  });

  it("recognizes every built-in Defense Series hero", () => {
    const heroes = {
      "office-guardians": ["architect", "security_master", "operations_master"],
      "cyber-fortress": ["incident_commander", "threat_hunter", "forensic_lead"],
      "ai-nexus-defense": ["research_agent", "security_agent", "coding_agent", "data_agent", "supervisor_agent"],
    } as const;

    for (const [game, ids] of Object.entries(heroes)) {
      for (const id of ids) {
        expect(resolveHeroPresentation(id, game as keyof typeof heroes)).toMatchObject({
          id,
          game,
          known: true,
        });
      }
    }
  });

  it("normalizes configured IDs before looking up authored art", () => {
    expect(resolveHeroPresentation("  Incident-Commander  ", "cyber-fortress"))
      .toMatchObject({ id: "incident_commander", motif: "command", known: true });
  });

  it("creates stable game-aware fallbacks without sharing mutable results", () => {
    const first = resolveHeroPresentation("future_sentinel", "cyber-fortress");
    const repeated = resolveHeroPresentation("future_sentinel", "cyber-fortress");
    const otherGame = resolveHeroPresentation("future_sentinel", "ai-nexus-defense");

    expect(first).toEqual(repeated);
    expect(first).not.toBe(repeated);
    expect(first).toMatchObject({
      id: "future_sentinel",
      game: "cyber-fortress",
      known: false,
    });
    expect(otherGame.seed).not.toBe(first.seed);
    expect(otherGame.game).toBe("ai-nexus-defense");
  });

  it("uses a deterministic safe identity for an empty remote ID", () => {
    const fallback = resolveHeroPresentation("   ");
    expect(fallback.id).toBe("unknown");
    expect(fallback.known).toBe(false);
    expect(fallback).toEqual(resolveHeroPresentation("unknown"));
  });
});
