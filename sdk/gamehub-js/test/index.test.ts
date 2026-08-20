import { describe, expect, it, vi } from 'vitest';
import { GameHubClient, GameHubError } from '../src/index';

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('GameHubClient', () => {
  it('initializes without sending telemetry before a session exists', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ user: { id: 'user-1', username: 'player' } }));
    const client = new GameHubClient({ gameId: 'snake', fetch: fetcher });

    await expect(client.init()).resolves.toMatchObject({ gameId: 'snake', user: { id: 'user-1' } });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledWith('/api/v1/me', expect.any(Object));
  });

  it('starts a session and submits the score with its id', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ session: { id: 'session-1', session_token: 'token-1' }, user: { id: 'user-1' } }))
      .mockResolvedValueOnce(response({ data: { accepted: true } }));
    const client = new GameHubClient({ gameId: 'snake', fetch: fetcher });

    await client.start();
    await client.submitScore({ score: 42, metadata: { apples: 4 } });

    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/v1/scores', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        session_id: 'session-1', session_token: 'token-1', game_id: 'snake', score: 42, metadata: { apples: 4 }, proof: undefined,
      }),
    }));
  });

  it('requires a session before score submission', async () => {
    const client = new GameHubClient({ gameId: '2048', fetch: vi.fn() });
    await expect(client.submitScore(10)).rejects.toBeInstanceOf(GameHubError);
  });

  it('exposes structured API errors', async () => {
    const client = new GameHubClient({
      gameId: 'memory',
      fetch: vi.fn().mockResolvedValue(response({ error: { code: 'blocked', message: 'Play is blocked' } }, 403)),
    });
    await expect(client.start()).rejects.toMatchObject({ status: 403, code: 'blocked' });
  });

  it('uses the backend group query parameter for team leaderboards', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ items: [] }));
    const client = new GameHubClient({ gameId: 'snake', fetch: fetcher });

    await client.getLeaderboard({ period: 'weekly', scope: 'team', limit: 20 });

    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/rankings?game_id=snake&period=weekly&limit=20&group=team',
      expect.objectContaining({ credentials: 'include' }),
    );
  });
});
