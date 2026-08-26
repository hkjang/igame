import type { TowerBranch, TowerDefinition } from "./types";

/**
 * How a tower is built, and what its chosen branch turned it into.
 *
 * A tower used to be drawn from its id alone, and the branch — the single
 * biggest decision a player makes on a tower — never reached the renderer at
 * all. Two level-three towers with opposite specialisations were pixel
 * identical on the field.
 */
export type TowerForm = "spire" | "garden" | "mortar" | "barracks" | "beacon";

/**
 * What a branch turned the tower into, chosen from what the branch actually
 * does rather than from its name, so an operator's own branch still reads.
 */
export type BranchEmblem =
  | "none"
  | "volley"
  | "reach"
  | "burst"
  | "pierce"
  | "chill"
  | "heavy";

export interface TowerPresentation {
  readonly id: string;
  readonly form: TowerForm;
  readonly emblem: BranchEmblem;
  readonly branchId: string;
  readonly known: boolean;
}

const KNOWN_FORMS: Record<string, TowerForm> = {
  sunspire: "spire",
  runebloom: "garden",
  stonepulse: "mortar",
  windward: "barracks",
};

/** A tower this client has never seen is built the way its damage behaves. */
const FORM_BY_DAMAGE: Record<string, TowerForm> = {
  physical: "spire",
  arcane: "garden",
  magic: "garden",
  siege: "mortar",
  frost: "beacon",
  true: "beacon",
};

/**
 * Reads the emblem from the branch's own numbers.
 *
 * A branch usually moves several stats at once, so each is scored against how
 * far a branch normally moves it and the standout wins. Picking by a fixed
 * priority instead gave both of a tower's branches the same badge whenever they
 * shared a mechanic — stonepulse's wide blast and its concentrated core are
 * both splash branches, and telling them apart is the whole point.
 */
export function emblemForBranch(branch: TowerBranch | undefined): BranchEmblem {
  if (!branch) return "none";
  const scores: Array<[BranchEmblem, number]> = [
    ["chill", (branch.slow ?? 0) / 0.5],
    ["burst", (branch.splash ?? 0) / 60],
    ["pierce", (branch.pierce ?? 0) / 2],
    ["volley", (1 - (branch.rateMultiplier ?? 1)) / 0.4],
    ["reach", ((branch.rangeMultiplier ?? 1) - 1) / 0.35],
    ["heavy", ((branch.damageMultiplier ?? 1) - 1) / 0.8],
  ];
  let best: BranchEmblem = "none";
  let bestScore = 0;
  for (const [emblem, score] of scores) {
    if (score > bestScore) {
      best = emblem;
      bestScore = score;
    }
  }
  return best;
}

export function resolveTowerPresentation(
  tower: Pick<TowerDefinition, "id" | "damageType" | "branches">,
  branchId = "",
  /** Whether the battle rules treat this tower as one that holds ground. */
  blocking = false,
): TowerPresentation {
  const id = tower.id.trim().toLowerCase();
  // A tower that holds ground is drawn as a barracks whatever it is called,
  // because that is the rule the player is reading off the field.
  const known = blocking ? "barracks" : KNOWN_FORMS[id];
  const branch = tower.branches.find((item) => item.id === branchId);
  return {
    id,
    form: known ?? FORM_BY_DAMAGE[tower.damageType] ?? "spire",
    emblem: emblemForBranch(branch),
    branchId: branch?.id ?? "",
    known: blocking || Boolean(KNOWN_FORMS[id]),
  };
}
