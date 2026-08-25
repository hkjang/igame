import { Box, Card, CardActionArea, Chip, Stack, Typography } from "@mui/material";
import { alpha, type SxProps, type Theme } from "@mui/material/styles";
import type { HeroDefinition } from "./types";
import {
  resolveHeroPresentation,
  type HeroPortraitBuild,
  type HeroPortraitHeadgear,
  type HeroPortraitMotif,
  type HeroPresentation,
  type HeroPresentationGame,
} from "./heroPresentation";

export type HeroSelectCardHero = Pick<
  HeroDefinition,
  | "id"
  | "name"
  | "title"
  | "hp"
  | "damage"
  | "range"
  | "speed"
  | "skill1"
  | "skill2"
  | "ultimate"
  | "unlockStage"
>;

export interface HeroSelectCardProps {
  hero: HeroSelectCardHero;
  game?: HeroPresentationGame;
  selected?: boolean;
  unlocked?: boolean;
  level?: number;
  disabled?: boolean;
  unlockLabel?: string;
  onSelect?: (heroId: string) => void;
  testId?: string;
  sx?: SxProps<Theme>;
}

function shoulderPath(build: HeroPortraitBuild) {
  if (build === "agile") return "M37 112 C42 94 57 87 80 87 C103 87 118 94 123 112 L129 144 H31 Z";
  if (build === "armored") return "M22 144 L29 105 L51 89 L80 96 L109 89 L131 105 L138 144 Z";
  return "M29 144 L36 105 C45 92 59 87 80 87 C101 87 115 92 124 105 L131 144 Z";
}

function Headgear({ type, presentation }: { type: HeroPortraitHeadgear; presentation: HeroPresentation }) {
  if (type === "hood") {
    return <path d="M48 66 Q49 24 80 20 Q111 24 112 66 L101 51 Q94 35 80 34 Q66 35 59 51 Z" fill={presentation.primary} stroke={presentation.secondary} strokeWidth="3" />;
  }
  if (type === "crown") {
    return <path d="M54 39 L57 18 L70 31 L80 13 L91 31 L104 18 L107 43 Z" fill={presentation.secondary} stroke={presentation.primary} strokeWidth="3" strokeLinejoin="round" />;
  }
  if (type === "visor") {
    return <path d="M51 48 Q80 37 109 48 L104 63 Q80 70 56 63 Z" fill={presentation.secondary} opacity=".92" stroke={presentation.primary} strokeWidth="3" />;
  }
  if (type === "halo") {
    return <ellipse cx="80" cy="24" rx="31" ry="10" fill="none" stroke={presentation.secondary} strokeWidth="4" opacity=".9" />;
  }
  if (type === "cap") {
    return <><path d="M49 43 Q54 19 82 19 Q105 21 111 43 Z" fill={presentation.primary} /><path d="M78 39 Q108 35 124 47 Q103 47 83 52 Z" fill={presentation.secondary} /></>;
  }
  return <path d="M49 47 Q54 20 81 21 Q108 23 111 49 Q99 36 88 36 Q70 48 49 47 Z" fill={presentation.hair} />;
}

function Motif({ type, color }: { type: HeroPortraitMotif; color: string }) {
  const common = { fill: "none", stroke: color, strokeWidth: 4, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };
  if (type === "bow") return <><path d="M7 5 Q29 22 7 43" {...common} /><path d="M7 5 L7 43 M7 24 L38 24 M31 18 L38 24 L31 30" {...common} /></>;
  if (type === "shield") return <><path d="M8 7 L38 7 L35 31 Q28 42 23 45 Q18 42 11 31 Z" {...common} /><path d="M23 13 V37 M13 24 H33" {...common} /></>;
  if (type === "staff") return <><path d="M23 12 V45" {...common} /><circle cx="23" cy="10" r="8" {...common} /><path d="M7 27 Q23 18 39 27" {...common} /></>;
  if (type === "blueprint") return <><rect x="6" y="8" width="35" height="32" rx="3" {...common} /><path d="M13 31 L21 20 L29 26 L36 15 M13 15 H20" {...common} /></>;
  if (type === "lock") return <><rect x="8" y="20" width="32" height="25" rx="5" {...common} /><path d="M15 20 V14 Q15 4 24 4 Q33 4 33 14 V20 M24 29 V36" {...common} /></>;
  if (type === "wrench") return <path d="M34 6 Q23 5 20 15 L7 31 Q3 36 8 41 Q13 46 18 41 L31 25 Q42 22 41 11 L33 18 L27 17 L26 11 Z" {...common} />;
  if (type === "command") return <><path d="M7 11 L23 24 L7 37 M24 11 L40 24 L24 37" {...common} /><path d="M9 45 H39" {...common} /></>;
  if (type === "target") return <><circle cx="24" cy="24" r="15" {...common} /><circle cx="24" cy="24" r="5" {...common} /><path d="M24 2 V10 M24 38 V46 M2 24 H10 M38 24 H46" {...common} /></>;
  if (type === "forensics") return <><circle cx="20" cy="20" r="13" {...common} /><path d="M29 30 L42 43 M14 15 Q20 10 26 15 M14 22 Q20 17 26 22 M17 28 Q20 25 23 28" {...common} /></>;
  if (type === "research") return <><path d="M7 10 Q16 6 23 12 V42 Q16 35 7 39 Z M39 10 Q30 6 23 12 V42 Q30 35 39 39 Z" {...common} /><path d="M13 18 H19 M27 18 H34" {...common} /></>;
  if (type === "ai-shield") return <><path d="M7 8 L24 3 L41 8 V25 Q38 38 24 45 Q10 38 7 25 Z" {...common} /><path d="M15 22 H33 M18 16 V29 M30 16 V29" {...common} /></>;
  if (type === "code") return <><path d="M17 9 L5 24 L17 39 M31 9 L43 24 L31 39 M28 5 L20 43" {...common} /></>;
  if (type === "data") return <><ellipse cx="24" cy="10" rx="17" ry="7" {...common} /><path d="M7 10 V36 Q7 43 24 43 Q41 43 41 36 V10 M7 23 Q7 30 24 30 Q41 30 41 23" {...common} /></>;
  return <><path d="M24 7 V20 M9 38 L19 29 M39 38 L29 29" {...common} /><circle cx="24" cy="25" r="7" {...common} /><circle cx="24" cy="5" r="4" {...common} /><circle cx="7" cy="40" r="4" {...common} /><circle cx="41" cy="40" r="4" {...common} /></>;
}

function seededCoordinate(seed: number, index: number, extent: number) {
  let value = (seed + Math.imul(index + 1, 0x6d2b79f5)) >>> 0;
  value = Math.imul(value ^ (value >>> 15), value | 1);
  value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
  return ((value ^ (value >>> 14)) >>> 0) % extent;
}

export function HeroPortrait({ hero, game = "realmguard" }: { hero: Pick<HeroDefinition, "id" | "name">; game?: HeroPresentationGame }) {
  const presentation = resolveHeroPresentation(hero.id, game);
  const marks = Array.from({ length: 8 }, (_, index) => ({
    x: 8 + seededCoordinate(presentation.seed, index * 2, 144),
    y: 8 + seededCoordinate(presentation.seed, index * 2 + 1, 128),
    radius: 1 + (index % 3),
  }));

  return <svg viewBox="0 0 160 160" role="img" aria-label={`${hero.name} 초상화`} preserveAspectRatio="xMidYMid slice">
    <rect width="160" height="160" rx="18" fill={presentation.background} />
    <circle cx="80" cy="66" r="61" fill={presentation.primary} opacity=".12" />
    <circle cx="80" cy="66" r="46" fill="none" stroke={presentation.secondary} strokeWidth="2" opacity=".26" />
    {marks.map((mark, index) => <circle key={index} cx={mark.x} cy={mark.y} r={mark.radius} fill={index % 2 ? presentation.primary : presentation.secondary} opacity=".34" />)}
    <g transform="translate(9 18) scale(.72)" opacity=".8"><Motif type={presentation.motif} color={presentation.secondary} /></g>
    <path d={shoulderPath(presentation.build)} fill={presentation.primary} stroke={presentation.secondary} strokeWidth="3" />
    {presentation.build === "armored" && <path d="M45 99 L63 91 L80 103 L97 91 L115 99 L105 140 H55 Z" fill={presentation.background} opacity=".55" stroke={presentation.secondary} strokeWidth="2" />}
    <path d="M70 78 H90 V101 Q80 110 70 101 Z" fill={presentation.skin} />
    <ellipse cx="80" cy="57" rx="29" ry="34" fill={presentation.skin} stroke={presentation.secondary} strokeWidth="2" />
    <path d="M51 50 Q55 21 80 20 Q106 22 110 50 Q97 39 88 36 Q70 50 51 50 Z" fill={presentation.hair} />
    <Headgear type={presentation.headgear} presentation={presentation} />
    {presentation.headgear !== "visor" && <><path d="M65 57 L73 57" stroke={presentation.background} strokeWidth="4" strokeLinecap="round" /><path d="M87 57 L95 57" stroke={presentation.background} strokeWidth="4" strokeLinecap="round" /></>}
    <path d="M73 72 Q80 77 87 72" fill="none" stroke={presentation.background} strokeWidth="2.5" strokeLinecap="round" />
    <g transform="translate(106 104)"><circle cx="24" cy="24" r="25" fill={presentation.background} stroke={presentation.secondary} strokeWidth="3" /><g transform="translate(8 8) scale(.68)"><Motif type={presentation.motif} color={presentation.secondary} /></g></g>
  </svg>;
}

export function HeroSelectCard({
  hero,
  game = "realmguard",
  selected = false,
  unlocked = true,
  level = 1,
  disabled = false,
  unlockLabel,
  onSelect,
  testId,
  sx,
}: HeroSelectCardProps) {
  const presentation = resolveHeroPresentation(hero.id, game);
  const displayLevel = Math.max(1, Math.floor(Number.isFinite(level) ? level : 1));
  const status = unlocked
    ? `Lv.${displayLevel}`
    : unlockLabel ?? (hero.unlockStage ? `Stage ${hero.unlockStage} 해금` : "잠김");
  const unavailable = disabled || !unlocked || !onSelect;
  const stats = [
    ["체력", hero.hp],
    ["공격", hero.damage],
    ["사거리", hero.range],
    ["기동", hero.speed],
  ] as const;

  return <Card
    data-testid={testId}
    data-selected={selected}
    data-unlocked={unlocked}
    variant="outlined"
    sx={[
      {
        overflow: "hidden",
        borderWidth: selected ? 2 : 1,
        borderColor: selected ? presentation.primary : "divider",
        boxShadow: selected ? `0 0 0 1px ${alpha(presentation.primary, .28)}, 0 12px 28px ${alpha(presentation.primary, .18)}` : "none",
        transition: "border-color 160ms ease, box-shadow 160ms ease, transform 160ms ease",
        "&:hover": unavailable ? undefined : { transform: "translateY(-2px)" },
      },
      ...(Array.isArray(sx) ? sx : [sx]),
    ]}
  >
    <CardActionArea
      disabled={unavailable}
      aria-pressed={selected}
      aria-label={`${hero.name}, ${hero.title}, ${status}${selected ? ", 선택됨" : ""}`}
      onClick={() => onSelect?.(hero.id)}
      sx={{ p: 0, textAlign: "left", opacity: unlocked ? 1 : .58 }}
    >
      <Box sx={{ display: "grid", gridTemplateColumns: "104px minmax(0, 1fr)", minHeight: 146 }}>
        <Box sx={{ p: 1, bgcolor: alpha(presentation.background, .72), display: "flex", alignItems: "stretch", filter: unlocked ? "none" : "grayscale(.72)" }}>
          <HeroPortrait hero={hero} game={game} />
        </Box>
        <Stack spacing={.7} sx={{ minWidth: 0, px: 1.2, py: 1 }}>
          <Stack direction="row" spacing={1} alignItems="flex-start">
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography fontWeight={900} noWrap>{hero.name}</Typography>
              <Typography variant="caption" color="text.secondary" noWrap display="block">{hero.title}</Typography>
            </Box>
            <Chip
              size="small"
              label={status}
              color={unlocked ? (selected ? "primary" : "default") : "warning"}
              variant={selected && unlocked ? "filled" : "outlined"}
              sx={{ flexShrink: 0, fontWeight: 800, maxWidth: 112 }}
            />
          </Stack>
          <Box component="dl" aria-label={`${hero.name} 능력치`} sx={{ display: "grid", gridTemplateColumns: "repeat(4, minmax(0, 1fr))", gap: .35, m: 0 }}>
            {stats.map(([label, value]) => <Box key={label} sx={{ minWidth: 0, textAlign: "center", p: .35, borderRadius: 1, bgcolor: alpha(presentation.primary, .09) }}>
              <Typography component="dt" variant="caption" color="text.secondary" sx={{ fontSize: ".64rem", lineHeight: 1.2 }}>{label}</Typography>
              <Typography component="dd" variant="caption" sx={{ m: 0, fontWeight: 900, lineHeight: 1.35 }}>{value}</Typography>
            </Box>)}
          </Box>
          <Stack direction="row" spacing={.45} useFlexGap flexWrap="wrap" aria-label={`${hero.name} 기술`}>
            <Chip size="small" label={hero.skill1} variant="outlined" sx={{ height: 22, maxWidth: "48%", "& .MuiChip-label": { px: .7, overflow: "hidden", textOverflow: "ellipsis" } }} />
            <Chip size="small" label={hero.skill2} variant="outlined" sx={{ height: 22, maxWidth: "48%", "& .MuiChip-label": { px: .7, overflow: "hidden", textOverflow: "ellipsis" } }} />
            <Chip size="small" label={`궁극 · ${hero.ultimate}`} sx={{ height: 22, maxWidth: "100%", color: presentation.secondary, bgcolor: alpha(presentation.secondary, .1), "& .MuiChip-label": { px: .7, overflow: "hidden", textOverflow: "ellipsis" } }} />
          </Stack>
        </Stack>
      </Box>
    </CardActionArea>
  </Card>;
}

export default HeroSelectCard;
