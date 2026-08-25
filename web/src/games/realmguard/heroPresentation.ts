export const HERO_PRESENTATION_GAMES = [
  "realmguard",
  "office-guardians",
  "cyber-fortress",
  "ai-nexus-defense",
] as const;

export type HeroPresentationGame = (typeof HERO_PRESENTATION_GAMES)[number];

export type HeroPortraitMotif =
  | "bow"
  | "shield"
  | "staff"
  | "blueprint"
  | "lock"
  | "wrench"
  | "command"
  | "target"
  | "forensics"
  | "research"
  | "ai-shield"
  | "code"
  | "data"
  | "network";

export type HeroPortraitBuild = "agile" | "balanced" | "armored";
export type HeroPortraitHeadgear =
  | "none"
  | "hood"
  | "crown"
  | "visor"
  | "halo"
  | "cap";

/**
 * Visual tokens consumed by code-native portraits. They deliberately contain
 * no React or canvas state, so the same hero resolves identically in menus,
 * battle HUDs and result screens.
 */
export interface HeroPresentation {
  readonly id: string;
  readonly game: HeroPresentationGame;
  readonly primary: string;
  readonly secondary: string;
  readonly background: string;
  readonly skin: string;
  readonly hair: string;
  readonly motif: HeroPortraitMotif;
  readonly build: HeroPortraitBuild;
  readonly headgear: HeroPortraitHeadgear;
  readonly seed: number;
  readonly known: boolean;
}

type HeroAppearance = Omit<
  HeroPresentation,
  "id" | "game" | "seed" | "known"
>;

const KNOWN_APPEARANCES = {
  aerin: {
    primary: "#FFD36B",
    secondary: "#FF7F6A",
    background: "#17243A",
    skin: "#F2BE91",
    hair: "#F4E4B4",
    motif: "bow",
    build: "agile",
    headgear: "hood",
  },
  brann: {
    primary: "#D98762",
    secondary: "#F3C56F",
    background: "#251B20",
    skin: "#B97555",
    hair: "#3A2522",
    motif: "shield",
    build: "armored",
    headgear: "crown",
  },
  nyra: {
    primary: "#7CDCF2",
    secondary: "#B694FF",
    background: "#111C38",
    skin: "#D9A77F",
    hair: "#E8F6FF",
    motif: "staff",
    build: "balanced",
    headgear: "halo",
  },
  architect: {
    primary: "#72E0A6",
    secondary: "#65D6FF",
    background: "#102A2A",
    skin: "#D99A72",
    hair: "#243541",
    motif: "blueprint",
    build: "balanced",
    headgear: "none",
  },
  security_master: {
    primary: "#65D6FF",
    secondary: "#FF7C91",
    background: "#101E31",
    skin: "#8F5E46",
    hair: "#151C27",
    motif: "lock",
    build: "armored",
    headgear: "visor",
  },
  operations_master: {
    primary: "#F49B67",
    secondary: "#72E0A6",
    background: "#2A1B1A",
    skin: "#F0C39B",
    hair: "#6A3C28",
    motif: "wrench",
    build: "balanced",
    headgear: "cap",
  },
  incident_commander: {
    primary: "#65D6FF",
    secondary: "#FFC866",
    background: "#0B2032",
    skin: "#C98761",
    hair: "#17202B",
    motif: "command",
    build: "armored",
    headgear: "cap",
  },
  threat_hunter: {
    primary: "#FF7C91",
    secondary: "#B694FF",
    background: "#281326",
    skin: "#E6AD82",
    hair: "#542B48",
    motif: "target",
    build: "agile",
    headgear: "hood",
  },
  forensic_lead: {
    primary: "#91A7FF",
    secondary: "#67E8DB",
    background: "#161C38",
    skin: "#8B5B43",
    hair: "#D8C9B2",
    motif: "forensics",
    build: "balanced",
    headgear: "visor",
  },
  research_agent: {
    primary: "#B694FF",
    secondary: "#67E8DB",
    background: "#20173B",
    skin: "#E0A77C",
    hair: "#49366A",
    motif: "research",
    build: "agile",
    headgear: "halo",
  },
  security_agent: {
    primary: "#67E8DB",
    secondary: "#FF7C91",
    background: "#102B30",
    skin: "#B87956",
    hair: "#15333B",
    motif: "ai-shield",
    build: "armored",
    headgear: "visor",
  },
  coding_agent: {
    primary: "#65D6FF",
    secondary: "#B694FF",
    background: "#111F39",
    skin: "#F0BE94",
    hair: "#263D67",
    motif: "code",
    build: "balanced",
    headgear: "hood",
  },
  data_agent: {
    primary: "#72E0A6",
    secondary: "#E7D86E",
    background: "#122B28",
    skin: "#85563F",
    hair: "#182B2B",
    motif: "data",
    build: "balanced",
    headgear: "none",
  },
  supervisor_agent: {
    primary: "#FFC866",
    secondary: "#91A7FF",
    background: "#2B2131",
    skin: "#D79B72",
    hair: "#EFE3C8",
    motif: "network",
    build: "armored",
    headgear: "crown",
  },
} as const satisfies Record<string, HeroAppearance>;

const FALLBACK_PALETTES: Record<
  HeroPresentationGame,
  readonly (readonly [primary: string, secondary: string, background: string])[]
> = {
  realmguard: [
    ["#FFD36B", "#73DCFF", "#17233A"],
    ["#E8845C", "#C69CFF", "#2A1927"],
    ["#69DCE4", "#9CF56B", "#102A2D"],
    ["#B79CFF", "#FF8B5E", "#201837"],
  ],
  "office-guardians": [
    ["#72E0A6", "#65D6FF", "#102A2A"],
    ["#FFC866", "#67E8DB", "#2B2516"],
    ["#91A7FF", "#F49B67", "#1B2038"],
  ],
  "cyber-fortress": [
    ["#65D6FF", "#FF7C91", "#102139"],
    ["#67E8DB", "#B694FF", "#112B31"],
    ["#91A7FF", "#FFC866", "#191F3A"],
  ],
  "ai-nexus-defense": [
    ["#B694FF", "#67E8DB", "#20183A"],
    ["#65D6FF", "#E7D86E", "#11223A"],
    ["#FF7C91", "#91A7FF", "#2B1830"],
  ],
};

const FALLBACK_MOTIFS: Record<HeroPresentationGame, readonly HeroPortraitMotif[]> = {
  realmguard: ["bow", "shield", "staff"],
  "office-guardians": ["blueprint", "lock", "wrench", "command"],
  "cyber-fortress": ["command", "target", "forensics", "ai-shield"],
  "ai-nexus-defense": ["research", "ai-shield", "code", "data", "network"],
};

const SKIN_TONES = ["#F3C8A2", "#E2A97F", "#BE805E", "#8B5B43", "#684332"] as const;
const HAIR_COLORS = ["#17202B", "#3B2928", "#6A4430", "#D8C9B2", "#E8F3F6"] as const;
const BUILDS: readonly HeroPortraitBuild[] = ["agile", "balanced", "armored"];
const HEADGEAR: readonly HeroPortraitHeadgear[] = ["none", "hood", "visor", "halo", "cap", "crown"];

function normalizeHeroId(heroId: string) {
  return heroId.trim().toLowerCase().replace(/[\s-]+/g, "_") || "unknown";
}

/** A small FNV-1a hash with stable unsigned 32-bit output across runtimes. */
function stableHash(value: string) {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

/**
 * Resolve a known hero's art direction or create a deterministic procedural
 * fallback for remotely configured heroes that this client has never seen.
 */
export function resolveHeroPresentation(
  heroId: string,
  game: HeroPresentationGame = "realmguard",
): HeroPresentation {
  const id = normalizeHeroId(heroId);
  const known = KNOWN_APPEARANCES[id as keyof typeof KNOWN_APPEARANCES];
  const seed = stableHash(`${game}:${id}`);

  if (known) {
    return { id, game, ...known, seed, known: true };
  }

  const palettes = FALLBACK_PALETTES[game];
  const palette = palettes[seed % palettes.length];
  const motifs = FALLBACK_MOTIFS[game];

  return {
    id,
    game,
    primary: palette[0],
    secondary: palette[1],
    background: palette[2],
    skin: SKIN_TONES[(seed >>> 3) % SKIN_TONES.length],
    hair: HAIR_COLORS[(seed >>> 7) % HAIR_COLORS.length],
    motif: motifs[(seed >>> 11) % motifs.length],
    build: BUILDS[(seed >>> 15) % BUILDS.length],
    headgear: HEADGEAR[(seed >>> 19) % HEADGEAR.length],
    seed,
    known: false,
  };
}
