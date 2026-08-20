import { describe, expect, it } from 'vitest';
import { BUILTIN_GAMES, mergeGames } from './builtinGames';
import type { Game } from '../types';

describe('mergeGames', () => {
  it('uses bundled games only when no server catalog is available', () => {
    expect(mergeGames()).toHaveLength(BUILTIN_GAMES.length);
    expect(mergeGames([])).toEqual([]);
  });

  it('keeps server identity and status authoritative for bundled runners', () => {
    const registered: Game = {
      ...BUILTIN_GAMES[0],
      id: 'b7bc1b89-bdb7-45c4-aa3c-49efcece9b99',
      status: 'maintenance',
      game_type: 'embedded',
    };
    expect(mergeGames([registered])).toEqual([
      expect.objectContaining({ id: registered.id, status: 'maintenance', game_type: 'builtin' }),
    ]);
  });
});
