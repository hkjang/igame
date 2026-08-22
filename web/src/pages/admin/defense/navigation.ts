export const DEFENSE_STUDIO_VIEWS = [
  "editor",
  "versions",
  "telemetry",
  "report",
] as const;

export type DefenseStudioView = (typeof DEFENSE_STUDIO_VIEWS)[number];

export function normalizeDefenseStudioView(
  value: string | null,
): DefenseStudioView {
  return DEFENSE_STUDIO_VIEWS.includes(value as DefenseStudioView)
    ? (value as DefenseStudioView)
    : "editor";
}

export function mergeCreatedDefenseVersion<T extends { id: string }>(
  current: { items: T[] } | undefined,
  created: T,
): { items: T[] } {
  return {
    items: [
      created,
      ...(current?.items ?? []).filter((item) => item.id !== created.id),
    ],
  };
}

export function defenseVersionsForSlug<T>(
  current: { slug: string; items: T[] } | undefined,
  slug: string,
): T[] {
  return current?.slug === slug ? current.items : [];
}
