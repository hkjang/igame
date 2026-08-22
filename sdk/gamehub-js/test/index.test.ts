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

  it('normalizes the legacy all period to the backend all_time value', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ items: [] }));
    const client = new GameHubClient({ gameId: 'snake', fetch: fetcher });

    await client.getLeaderboard({ period: 'all' });

    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/rankings?game_id=snake&period=all_time',
      expect.objectContaining({ credentials: 'include' }),
    );
  });

  it('sends RealmGuard attestation identity at the telemetry envelope top level', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ session: { id: 'rg-session', session_token: 'rg-token' } }))
      .mockResolvedValueOnce(response({ accepted: true }));
    const client = new GameHubClient({ gameId: 'realmguard', fetch: fetcher });
    await client.start();
    await client.telemetry({
      event: 'realmguard.wave.complete', payload: { wave: 1 }, occurredAt: '2026-08-20T10:00:00.000Z',
      clientEventId: '22222222-2222-4222-8222-222222222222', sequence: 3,
    });
    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/v1/telemetry', expect.objectContaining({
      body: JSON.stringify({
        game_id: 'realmguard', session_id: 'rg-session', session_token: 'rg-token', event: 'realmguard.wave.complete',
        data: { wave: 1 }, occurred_at: '2026-08-20T10:00:00.000Z', client_event_id: '22222222-2222-4222-8222-222222222222', sequence: 3,
      }),
    }));
  });

  it('atomically completes a server-authoritative game with session credentials', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ session: { id: 'rg-session', session_token: 'rg-token' } }))
      .mockResolvedValueOnce(response({ result: { score: 1234, stars: 3 } }));
    const client = new GameHubClient({ gameId: 'realmguard', fetch: fetcher });

    await client.start();
    const completed = await client.completeAuthoritatively<{ result: { score: number } }>({
      path: '/api/v1/realmguard/results',
      payload: { stage_id: 'stage-1', remaining_lives: 20 },
    });

    expect(completed.result.score).toBe(1234);
    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/v1/realmguard/results', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ stage_id: 'stage-1', remaining_lives: 20, game_id: 'realmguard', session_id: 'rg-session', session_token: 'rg-token' }),
    }));
    expect(client.session).toBeUndefined();
  });

  it('submits an authoritative education action without closing the defense session', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ session: { id: 'def-session', session_token: 'def-token' } }))
      .mockResolvedValueOnce(response({ answer: { event_id: 'event-1', correct: true, score: 100 } }));
    const client = new GameHubClient({ gameId: 'cyber-fortress', fetch: fetcher });

    await client.start({ defense_content_version_id: 'version-1' });
    await client.requestAuthoritatively({
      path: '/api/v1/defense/cyber-fortress/education/events/event-1/answer',
      payload: { answer_id: 'safe' },
    });

    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/v1/defense/cyber-fortress/education/events/event-1/answer', expect.objectContaining({
      body: JSON.stringify({ answer_id: 'safe', game_id: 'cyber-fortress', session_id: 'def-session', session_token: 'def-token' }),
    }));
    expect(client.session?.id).toBe('def-session');
  });
});
