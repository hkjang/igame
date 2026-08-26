import { describe, expect, it } from "vitest";
import { TOWERS } from "./content";
import { emblemForBranch, resolveTowerPresentation } from "./towerPresentation";

describe("resolveTowerPresentation", () => {
  it("tells a tower's two branches apart", () => {
    for (const tower of TOWERS) {
      const emblems = tower.branches.map((branch) => emblemForBranch(branch));
      // The branch is the biggest decision a player makes on a tower. If both
      // of its builds wore the same badge the field would not show the choice.
      expect(new Set(emblems).size).toBe(tower.branches.length);
      expect(emblems).not.toContain("none");
    }
  });

  it("names each RealmGuard branch by what it actually does", () => {
    const emblem = (towerId: string, branchId: string) =>
      resolveTowerPresentation(TOWERS.find((tower) => tower.id === towerId)!, branchId).emblem;
    expect(emblem("sunspire", "dawn_volley")).toBe("volley");
    expect(emblem("sunspire", "eagle_oath")).toBe("reach");
    expect(emblem("runebloom", "star_lattice")).toBe("burst");
    expect(emblem("runebloom", "null_petal")).toBe("pierce");
    expect(emblem("stonepulse", "quake_drum")).toBe("burst");
    // Both stonepulse branches carry splash; the concentrated one is a damage
    // branch and has to read as one.
    expect(emblem("stonepulse", "ember_core")).toBe("heavy");
    expect(emblem("windward", "shield_line")).toBe("chill");
  });

  it("has no emblem before a branch is chosen", () => {
    const sunspire = TOWERS.find((tower) => tower.id === "sunspire")!;
    expect(resolveTowerPresentation(sunspire).emblem).toBe("none");
    expect(resolveTowerPresentation(sunspire, "not-a-branch").emblem).toBe("none");
    expect(emblemForBranch(undefined)).toBe("none");
  });

  it("builds an unknown tower the way its damage behaves", () => {
    const base = { id: "soc_desk", branches: [] };
    expect(resolveTowerPresentation({ ...base, damageType: "arcane" }).form).toBe("garden");
    expect(resolveTowerPresentation({ ...base, damageType: "siege" }).form).toBe("mortar");
    expect(resolveTowerPresentation({ ...base, damageType: "frost" }).form).toBe("beacon");
    expect(resolveTowerPresentation({ ...base, damageType: "physical" }).known).toBe(false);
  });

  it("draws anything that holds ground as a barracks", () => {
    // The rule the player reads off the field is "this one blocks", not "this
    // one is called windward".
    const holder = { id: "human_review", damageType: "physical" as const, branches: [] };
    expect(resolveTowerPresentation(holder, "", true).form).toBe("barracks");
    expect(resolveTowerPresentation(holder, "", false).form).toBe("spire");
  });
});
