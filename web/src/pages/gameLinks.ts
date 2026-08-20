export const REALMGUARD_RANKING_ANCHOR = 'realmguard-rankings';

export function gameRankingHref(slug: string, gameId: string): string {
  if (slug === 'realmguard') return `/games/${encodeURIComponent(slug)}#${REALMGUARD_RANKING_ANCHOR}`;
  return `/rankings?game=${encodeURIComponent(gameId)}`;
}
