import type { HeroPresentationGame } from "./heroPresentation";

/**
 * Silhouette families the battlefield can draw.
 *
 * Every enemy in the product used to be the same filled circle in a different
 * colour, so a player could not tell an armoured wall from a healer without
 * reading the wave table. These are shapes chosen to be told apart at the size
 * an enemy actually appears on the field.
 */
export type EnemySilhouette =
  | "blob"
  | "bristle"
  | "runner"
  | "flyer"
  | "seer"
  | "crystal"
  | "treant"
  | "wraith"
  | "ram"
  | "core"
  | "sovereign"
  | "drake";

/** Trait affordances drawn on top of the silhouette. */
export type EnemyMark =
  | "armored"
  | "flying"
  | "healer"
  | "splitting"
  | "phasing"
  | "siege"
  | "regenerating"
  | "swift"
  | "stealth"
  | "berserk"
  | "boss";

export interface EnemyPresentation {
  readonly id: string;
  readonly game: HeroPresentationGame;
  readonly silhouette: EnemySilhouette;
  /** Body fill. Falls back to the content's own colour when one is supplied. */
  readonly primary: string;
  /** Plating, limbs and shadowed facets. */
  readonly secondary: string;
  /** Eyes, cores and trait marks; always the brightest of the three. */
  readonly accent: string;
  readonly eyes: 0 | 1 | 2 | 3;
  readonly marks: readonly EnemyMark[];
  readonly seed: number;
  readonly known: boolean;
}

type EnemyAppearance = Pick<
  EnemyPresentation,
  "silhouette" | "primary" | "secondary" | "accent" | "eyes"
>;

/**
 * The RealmGuard bestiary is authored: it is the product's own IP and each
 * creature has a name a player is meant to recognise.
 */
const KNOWN_APPEARANCES: Record<string, EnemyAppearance> = {
  mireling: { silhouette: "blob", primary: "#7FCF75", secondary: "#4E8F4A", accent: "#EAFFDF", eyes: 2 },
  thornback: { silhouette: "bristle", primary: "#4F8958", secondary: "#2C4F33", accent: "#D7F0A8", eyes: 2 },
  glintfox: { silhouette: "runner", primary: "#F5C76A", secondary: "#B07A2A", accent: "#FFF4CF", eyes: 2 },
  cloudray: { silhouette: "flyer", primary: "#8FE3FF", secondary: "#3E86A8", accent: "#F0FBFF", eyes: 2 },
  bloomseer: { silhouette: "seer", primary: "#EF8ED5", secondary: "#9B4E8C", accent: "#FFE9F8", eyes: 1 },
  shardling: { silhouette: "crystal", primary: "#B79CFF", secondary: "#6B54B0", accent: "#F2ECFF", eyes: 3 },
  ironroot: { silhouette: "treant", primary: "#8D8062", secondary: "#544B36", accent: "#D9CFA6", eyes: 2 },
  veilrunner: { silhouette: "wraith", primary: "#8B87D7", secondary: "#4A468C", accent: "#E4E2FF", eyes: 2 },
  rammer: { silhouette: "ram", primary: "#DA755D", secondary: "#8A3E2C", accent: "#FFD9C9", eyes: 2 },
  rimeheart: { silhouette: "core", primary: "#70C9E8", secondary: "#2F6E88", accent: "#EAFBFF", eyes: 1 },
  hollow_king: { silhouette: "sovereign", primary: "#9D5BE8", secondary: "#4E2777", accent: "#FFE29B", eyes: 2 },
  timewyrm: { silhouette: "drake", primary: "#FF866A", secondary: "#9C3A28", accent: "#FFE1A8", eyes: 2 },
};

/**
 * Silhouettes a trait implies. A remotely configured enemy this client has
 * never seen still has to *look* like what it does, so behaviour picks the
 * shape before the seed does.
 */
const TRAIT_SILHOUETTES: Partial<Record<string, EnemySilhouette>> = {
  boss: "sovereign",
  flying: "flyer",
  siege: "ram",
  healer: "seer",
  splitting: "crystal",
  phasing: "wraith",
  regenerating: "core",
  armored: "bristle",
  swift: "runner",
};

/** Order matters: the first trait present decides the shape. */
const TRAIT_PRIORITY = [
  "boss",
  "flying",
  "siege",
  "healer",
  "splitting",
  "phasing",
  "regenerating",
  "armored",
  "swift",
] as const;

const MARKS: readonly EnemyMark[] = [
  "armored",
  "flying",
  "healer",
  "splitting",
  "phasing",
  "siege",
  "regenerating",
  "swift",
  "stealth",
  "berserk",
  "boss",
];

const FALLBACK_SILHOUETTES: EnemySilhouette[] = [
  "blob",
  "bristle",
  "runner",
  "crystal",
  "wraith",
  "core",
];

/** One palette per game so a roster reads as a set rather than a jumble. */
const FALLBACK_PALETTES: Record<HeroPresentationGame, string[][]> = {
  realmguard: [
    ["#7FCF75", "#3F6E3C", "#EAFFDF"],
    ["#B79CFF", "#5B4694", "#F2ECFF"],
    ["#70C9E8", "#2F6E88", "#EAFBFF"],
  ],
  "office-guardians": [
    ["#7FB2FF", "#2F5698", "#EAF2FF"],
    ["#FFC46B", "#A2701F", "#FFF3DC"],
    ["#8ADFC0", "#2F7A5F", "#E6FFF5"],
    ["#FF9BA6", "#963F4C", "#FFE8EB"],
  ],
  "cyber-fortress": [
    ["#FF7B7B", "#8E2B2B", "#FFE3E3"],
    ["#FFB25C", "#96591A", "#FFEFD9"],
    ["#8FD9FF", "#2C6C93", "#E7F8FF"],
    ["#C79BFF", "#5E3B9B", "#F3EAFF"],
  ],
  "ai-nexus-defense": [
    ["#A98CFF", "#513A9E", "#EFE9FF"],
    ["#6FE3D0", "#227F71", "#E2FFFA"],
    ["#FFD16B", "#9A731B", "#FFF5DA"],
    ["#FF8FB8", "#93365C", "#FFE7F0"],
  ],
};

function normalizeEnemyId(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9_-]/g, "");
}

/** Same 32-bit FNV-1a the hero presentation uses, so both stay reproducible. */
function stableHash(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash >>> 0;
}

function marksFor(traits: readonly string[]): EnemyMark[] {
  const present = new Set(traits);
  return MARKS.filter((mark) => present.has(mark));
}

function silhouetteFor(traits: readonly string[], seed: number): EnemySilhouette {
  const present = new Set(traits);
  for (const trait of TRAIT_PRIORITY) {
    if (present.has(trait)) return TRAIT_SILHOUETTES[trait] ?? "blob";
  }
  return FALLBACK_SILHOUETTES[seed % FALLBACK_SILHOUETTES.length];
}

/**
 * Resolve an enemy's art direction.
 *
 * `traits` come from the pinned content, so the marks a player reads on the
 * field are the same ones the battle rules act on.
 */
export function resolveEnemyPresentation(
  enemyId: string,
  traits: readonly string[] = [],
  game: HeroPresentationGame = "realmguard",
): EnemyPresentation {
  const id = normalizeEnemyId(enemyId);
  const seed = stableHash(`${game}:${id}`);
  const marks = marksFor(traits);
  const known = KNOWN_APPEARANCES[id];

  if (known && game === "realmguard") {
    return { id, game, ...known, marks, seed, known: true };
  }

  const palettes = FALLBACK_PALETTES[game];
  const palette = palettes[seed % palettes.length];
  return {
    id,
    game,
    silhouette: silhouetteFor(traits, seed >>> 5),
    primary: palette[0],
    secondary: palette[1],
    accent: palette[2],
    eyes: [2, 2, 1, 3][(seed >>> 9) % 4] as 1 | 2 | 3,
    marks,
    seed,
    known: false,
  };
}
