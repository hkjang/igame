import { describe, expect, it } from 'vitest';
import { gameRankingHref, REALMGUARD_RANKING_ANCHOR } from './gameLinks';

describe('game ranking navigation', () => {
  it('opens RealmGuard’s server-authoritative leaderboard inside the game', () => {
    expect(gameRankingHref('realmguard', 'realmguard-id')).toBe(`/games/realmguard#${REALMGUARD_RANKING_ANCHOR}`);
  });

  it('keeps other games on the shared leaderboard route', () => {
    expect(gameRankingHref('snake', 'snake id')).toBe('/rankings?game=snake%20id');
  });
});
