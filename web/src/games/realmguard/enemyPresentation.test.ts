import { describe, expect, it } from "vitest";
import { ENEMIES } from "./content";
import { resolveEnemyPresentation } from "./enemyPresentation";

describe("resolveEnemyPresentation", () => {
  it("gives every RealmGuard creature its own silhouette", () => {
    const silhouettes = ENEMIES.map(
      (enemy) => resolveEnemyPresentation(enemy.id, enemy.traits).silhouette,
    );
    // The roster used to be twelve identical circles; the point of the art is
    // that no two of them are the same shape.
    expect(new Set(silhouettes).size).toBe(ENEMIES.length);
    expect(resolveEnemyPresentation("hollow_king", ["boss"]).known).toBe(true);
  });

  it("carries the pinned content's traits through as marks", () => {
    const rammer = resolveEnemyPresentation("rammer", ["siege", "armored"]);
    expect(rammer.marks).toEqual(["armored", "siege"]);
    // Marks are what the battle rules act on, so an unlisted trait is dropped
    // rather than invented.
    expect(resolveEnemyPresentation("rammer", ["summoned"]).marks).toEqual([]);
  });

  it("shapes an unknown enemy by what it does, not by its name", () => {
    expect(resolveEnemyPresentation("phishing", ["flying"], "cyber-fortress").silhouette).toBe("flyer");
    expect(resolveEnemyPresentation("ransomware", ["siege"], "cyber-fortress").silhouette).toBe("ram");
    expect(resolveEnemyPresentation("hallucination", ["healer"], "ai-nexus-defense").silhouette).toBe("seer");
    expect(resolveEnemyPresentation("outage_overlord", ["boss", "swift"], "office-guardians").silhouette).toBe("sovereign");
  });

  it("resolves the same enemy identically every time", () => {
    const first = resolveEnemyPresentation("token_monster", [], "ai-nexus-defense");
    const second = resolveEnemyPresentation("token_monster", [], "ai-nexus-defense");
    expect(second).toEqual(first);
    expect(first.known).toBe(false);
    // A different game is a different roster, so the same id may look different.
    expect(resolveEnemyPresentation("token_monster", [], "cyber-fortress").seed).not.toBe(first.seed);
  });

  it("keeps a traitless unknown enemy inside the game's own palette", () => {
    for (const game of ["office-guardians", "cyber-fortress", "ai-nexus-defense"] as const) {
      const presentation = resolveEnemyPresentation("mystery_threat", [], game);
      expect(presentation.primary).toMatch(/^#[0-9A-F]{6}$/i);
      expect(presentation.accent).toMatch(/^#[0-9A-F]{6}$/i);
      expect([1, 2, 3]).toContain(presentation.eyes);
    }
  });

  it("normalizes ids the way the content pipeline writes them", () => {
    expect(resolveEnemyPresentation("  Mireling  ").id).toBe("mireling");
    expect(resolveEnemyPresentation("Mireling").silhouette).toBe("blob");
  });
});
